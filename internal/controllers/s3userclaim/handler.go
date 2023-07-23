/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package s3userclaim

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
)

type Reconciler struct {
	client.Client
	scheme *runtime.Scheme

	logger logr.Logger

	// reconcile specific variables
	s3UserClaim *s3v1alpha1.S3UserClaim
	cephUser    *admin.User
	tenant      string
	userId      string
	s3UserName  string

	// configurations
	displayName  string
	clusterName  string
	rgwAccessKey string
	rgwSecretKey string
	rgwEndpoint  string
	s3UserClass  string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config) *Reconciler {
	return &Reconciler{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),

		s3UserClass:  cfg.S3UserClass,
		clusterName:  cfg.ClusterName,
		rgwAccessKey: cfg.Rgw.AccessKey,
		rgwSecretKey: cfg.Rgw.SecretKey,
		rgwEndpoint:  cfg.Rgw.Endpoint,
	}
}

//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.snappcloud.io,resources=s3userclaims/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger = log.FromContext(ctx)

	// Handle object fetch error
	switch err := r.Get(ctx, req.NamespacedName, r.s3UserClaim); {
	case k8serrors.IsNotFound(err):
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	case err != nil:
		r.logger.Error(err, "failed to fetch object")
		return subreconciler.Evaluate(subreconciler.Requeue())
	}

	// Ignore object with deletionTimestamp set
	if r.s3UserClaim.ObjectMeta.DeletionTimestamp != nil {
		return subreconciler.Evaluate(subreconciler.DoNotRequeue())
	}

	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.initVars,
		r.ensureCephUser,
		r.ensureAdminSecret,
		r.ensureReadonlySecret,
		r.ensureS3User,
		r.updateS3UserClaimStatus,
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) initVars(context.Context) (*ctrl.Result, error) {
	r.userId = r.s3UserClaim.Name
	r.tenant = fmt.Sprintf("%s.%s", r.clusterName, r.s3UserClaim.ObjectMeta.Namespace)
	r.displayName = fmt.Sprintf("%s in %s.%s", r.s3UserClaim.Name, r.s3UserClaim.Namespace, r.clusterName)

	r.s3UserName = fmt.Sprintf("%s.%s", r.s3UserClaim.ObjectMeta.Namespace, r.s3UserClaim.Name)

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureCephUser(ctx context.Context) (*ctrl.Result, error) {
	rgwClient, err := admin.New(r.rgwEndpoint, r.rgwAccessKey, r.rgwSecretKey, nil)
	if err != nil {
		r.logger.Error(err, "failed to create rgw connection")
		return subreconciler.Requeue()
	}

	desiredUser := admin.User{
		Tenant:      r.tenant,
		ID:          r.userId,
		DisplayName: r.displayName,
	}

	switch _, err = rgwClient.GetUser(ctx, desiredUser); {
	case err == nil:
		user, err := rgwClient.ModifyUser(ctx, desiredUser)
		if err != nil {
			r.logger.Error(err, "failed to update ceph user")
			return subreconciler.Requeue()
		}
		r.cephUser = &user
	case errors.Is(err, admin.ErrNoSuchUser):
		user, err := r.createCephUser(ctx, rgwClient)
		if err != nil {
			r.logger.Error(err, "failed to create ceph user")
			return subreconciler.Requeue()
		}
		r.cephUser = user
	default:
		r.logger.Error(err, "failed to get ceph user")
		return subreconciler.Requeue()
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureAdminSecret(ctx context.Context) (*ctrl.Result, error) {
	adminSecret := &v1.Secret{}

	switch err := r.Get(
		ctx,
		types.NamespacedName{Namespace: r.s3UserClaim.Namespace, Name: r.s3UserClaim.Spec.AdminSecret},
		adminSecret,
	); {
	case k8serrors.IsNotFound(err):
		secret := r.assembleAdminSecret()
		if err := r.Create(ctx, secret); err != nil {
			r.logger.Error(err, "failed to create admin secret")
			return subreconciler.Requeue()
		}
	case err != nil:
		r.logger.Error(err, "failed to get admin secret")
		return subreconciler.Requeue()
	default:
		secret := r.assembleAdminSecret()
		if !reflect.DeepEqual(adminSecret.Data, secret.Data) {
			adminSecret.Data = secret.Data
			if err := r.Update(ctx, adminSecret); err != nil {
				r.logger.Error(err, "failed to update admin secret")
				return subreconciler.Requeue()
			}
		}
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureReadonlySecret(ctx context.Context) (*ctrl.Result, error) {
	readonlySecret := &v1.Secret{}

	switch err := r.Get(
		ctx,
		types.NamespacedName{Namespace: r.s3UserClaim.Namespace, Name: r.s3UserClaim.Spec.ReadonlySecret},
		readonlySecret,
	); {
	case k8serrors.IsNotFound(err):
		secret := r.assembleReadonlySecret()
		if err := r.Create(ctx, secret); err != nil {
			r.logger.Error(err, "failed to create admin secret")
			return subreconciler.Requeue()
		}
	case err != nil:
		r.logger.Error(err, "failed to get admin secret")
		return subreconciler.Requeue()
	default:
		secret := r.assembleReadonlySecret()
		if !reflect.DeepEqual(readonlySecret.Data, secret.Data) {
			readonlySecret.Data = secret.Data
			if err := r.Update(ctx, readonlySecret); err != nil {
				r.logger.Error(err, "failed to update readonly secret")
				return subreconciler.Requeue()
			}
		}
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureS3User(ctx context.Context) (*ctrl.Result, error) {
	existingS3User := &s3v1alpha1.S3User{}

	switch err := r.Get(ctx, types.NamespacedName{}, existingS3User); {
	case k8serrors.IsNotFound(err):
		s3user := r.assembleS3User()
		if err := r.Create(ctx, s3user); err != nil {
			r.logger.Error(err, "failed to create s3 user")
			return subreconciler.Requeue()
		}
		return subreconciler.ContinueReconciling()
	case err != nil:
		r.logger.Error(err, "failed to get s3 user")
		return subreconciler.Requeue()
	default:
		desiredS3user := r.assembleS3User()
		if !reflect.DeepEqual(desiredS3user.Spec, existingS3User.Spec) ||
			!reflect.DeepEqual(desiredS3user.Status, existingS3User.Status) {
			existingS3User.Spec = *desiredS3user.Spec.DeepCopy()
			if err := r.Update(ctx, existingS3User); err != nil {
				r.logger.Error(err, "failed to update s3 user")
				return subreconciler.Requeue()
			}
		}
		return subreconciler.ContinueReconciling()
	}
}

func (r *Reconciler) updateS3UserClaimStatus(ctx context.Context) (*ctrl.Result, error) {
	if err := r.Update(ctx, r.s3UserClaim); err != nil {
		r.logger.Error(err, "failed to update s3 user claim")
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) createCephUser(ctx context.Context, rgwClient *admin.API) (*admin.User, error) {
	user, err := rgwClient.CreateUser(ctx, admin.User{
		Tenant:      r.tenant,
		ID:          r.userId,
		DisplayName: r.displayName,
	})
	return &user, err
}

func (r *Reconciler) assembleS3User() *s3v1alpha1.S3User {
	return &s3v1alpha1.S3User{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.s3UserName,
		},
		Spec:   s3v1alpha1.S3UserSpec{},
		Status: s3v1alpha1.S3UserStatus{},
	}
}

func (r *Reconciler) assembleAdminSecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.s3UserClaim.Namespace,
			Name:      r.s3UserClaim.Spec.AdminSecret,
		},
		Data: map[string][]byte{
			"accessKey": []byte(r.cephUser.Keys[0].AccessKey),
			"secretKey": []byte(r.cephUser.Keys[0].SecretKey),
		},
	}
}

func (r *Reconciler) assembleReadonlySecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.s3UserClaim.Namespace,
			Name:      r.s3UserClaim.Spec.ReadonlySecret,
		},
		Data: map[string][]byte{
			"accessKey": []byte(r.cephUser.Keys[0].AccessKey),
			"secretKey": []byte(r.cephUser.Keys[0].SecretKey),
		},
	}
}
