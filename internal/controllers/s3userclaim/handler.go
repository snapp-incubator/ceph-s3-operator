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
	user        *admin.User
	tenant      string
	userId      string
	s3UserName  string

	// configurations
	displayName  string
	clusterName  string
	rgwAccessKey string
	rgwSecretKey string
	rgwEndpoint  string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config) *Reconciler {
	return &Reconciler{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
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
		r.ensureS3User,
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
		r.user = &user
	case errors.Is(err, admin.ErrNoSuchUser):
		user, err := r.createCephUser(ctx, rgwClient)
		if err != nil {
			r.logger.Error(err, "failed to create ceph user")
			return subreconciler.Requeue()
		}
		r.user = user
	default:
		r.logger.Error(err, "failed to get ceph user")
		return subreconciler.Requeue()
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
