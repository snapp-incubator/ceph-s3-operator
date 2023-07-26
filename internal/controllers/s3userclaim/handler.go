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
	"regexp"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/go-logr/logr"
	"github.com/opdev/subreconciler"
	openshiftquota "github.com/openshift/api/quota/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/config"
	"github.com/snapp-incubator/s3-operator/internal/rgwclient"
	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

type Reconciler struct {
	client.Client
	scheme    *runtime.Scheme
	logger    logr.Logger
	rgwClient rgwclient.RgwClient

	// reconcile specific variables
	clusterResourceQuota *openshiftquota.ClusterResourceQuota
	s3UserClaim          *s3v1alpha1.S3UserClaim
	cephUser             *admin.User
	cephTenant           string
	cephUserId           string
	fullCephUserId       string
	cephDisplayName      string
	s3UserName           string

	// configurations
	clusterName  string
	rgwAccessKey string
	rgwSecretKey string
	rgwEndpoint  string
	s3UserClass  string
}

func NewReconciler(mgr manager.Manager, cfg *config.Config, rgwClient rgwclient.RgwClient) *Reconciler {
	return &Reconciler{
		Client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		rgwClient: rgwClient,

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
	r.s3UserClaim = &s3v1alpha1.S3UserClaim{}

	// Fetch the object
	switch err := r.Get(ctx, req.NamespacedName, r.s3UserClaim); {
	case apierrors.IsNotFound(err):
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
		r.ensureCephUserQuota,
		r.ensureAdminSecret,
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
	// Only alphanumeric characters and underscore are allowed for tenant name
	k8sNameSpecialChars := regexp.MustCompile(`[.-]`)
	namespace := k8sNameSpecialChars.ReplaceAllString(r.s3UserClaim.Namespace, "_")
	clusterName := k8sNameSpecialChars.ReplaceAllString(r.clusterName, "_")
	r.cephTenant = fmt.Sprintf("%s__%s", clusterName, namespace)
	r.cephUserId = r.s3UserClaim.Name
	// Ceph-SDK functions that involve retrieving the user such as GetQuota, GetUser and even SetUser,
	// required tenant name in UID field.
	r.fullCephUserId = fmt.Sprintf("%s$%s", r.cephTenant, r.cephUserId)
	r.cephDisplayName = fmt.Sprintf("%s in %s.%s", r.s3UserClaim.Name, r.s3UserClaim.Namespace, r.clusterName)

	r.s3UserName = fmt.Sprintf("%s.%s", r.s3UserClaim.Namespace, r.s3UserClaim.Name)

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureCephUser(ctx context.Context) (*ctrl.Result, error) {
	desiredUser := admin.User{
		ID:          r.fullCephUserId,
		DisplayName: r.cephDisplayName,
	}

	switch exitingUser, err := r.rgwClient.GetUser(ctx, &desiredUser); {
	case err == nil:
		r.cephUser = exitingUser
	case errors.Is(err, admin.ErrNoSuchUser):
		user, err := r.rgwClient.CreateUser(ctx, &desiredUser)
		if err != nil {
			r.logger.Error(err, "failed to create ceph user", "userId", desiredUser.ID)
			return subreconciler.Requeue()
		}
		r.cephUser = user
	default:
		r.logger.Error(err, "failed to get ceph user")
		return subreconciler.Requeue()
	}

	// TODO: What should be done here? Requeueing won't help. Emit an event or increasing a prometheus counter :-?
	if len(r.cephUser.Keys) == 0 {
		err := fmt.Errorf("ceph user doesn't have any keys")
		r.logger.Error(err, "")
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureCephUserQuota(ctx context.Context) (*ctrl.Result, error) {
	desiredQuota := &admin.QuotaSpec{
		UID:        r.fullCephUserId,
		QuotaType:  consts.QuotaTypeUser,
		Enabled:    pointer.Bool(true),
		MaxSize:    pointer.Int64(r.s3UserClaim.Spec.Quota.MaxSize.Value()),
		MaxObjects: pointer.Int64(r.s3UserClaim.Spec.Quota.MaxObjects.Value()),
	}

	switch existingQuota, err := r.rgwClient.GetQuota(ctx, desiredQuota); {
	case err == nil:
		// We need to compare field by field. DeepEqual won't work here as the retrieved quota doesn't have all
		// the fields set to their real value (e.g. UID will be empty although the real user has UID)
		if *existingQuota.Enabled != *desiredQuota.Enabled ||
			*existingQuota.MaxSize != *desiredQuota.MaxSize ||
			*existingQuota.MaxObjects != *desiredQuota.MaxObjects {
			if err := r.rgwClient.SetQuota(ctx, desiredQuota); err != nil {
				r.logger.Error(err, "failed to set user quota", "userId", desiredQuota.UID)
				return subreconciler.Requeue()
			}
		}

		r.cephUser.UserQuota = *desiredQuota
		return subreconciler.ContinueReconciling()
	default:
		r.logger.Error(err, "failed to get user quota")
		return subreconciler.Requeue()
	}
}

func (r *Reconciler) ensureAdminSecret(ctx context.Context) (*ctrl.Result, error) {
	adminSecret := &v1.Secret{}

	switch err := r.Get(
		ctx,
		types.NamespacedName{Namespace: r.s3UserClaim.Namespace, Name: r.s3UserClaim.Spec.AdminSecret},
		adminSecret,
	); {
	case apierrors.IsNotFound(err):
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
		if !apiequality.Semantic.DeepEqual(adminSecret.Data, secret.Data) {
			adminSecret.Data = secret.Data
			if err := r.Update(ctx, adminSecret); err != nil {
				r.logger.Error(err, "failed to update admin secret")
				return subreconciler.Requeue()
			}
		}
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureS3User(ctx context.Context) (*ctrl.Result, error) {
	existingS3User := &s3v1alpha1.S3User{}

	switch err := r.Get(ctx, types.NamespacedName{Name: r.s3UserName}, existingS3User); {
	case apierrors.IsNotFound(err):
		s3user, err := r.assembleS3User()
		if err != nil {
			r.logger.Error(err, "failed to assemble s3 user")
			return subreconciler.Requeue()
		}
		if err := r.Create(ctx, s3user); err != nil {
			r.logger.Error(err, "failed to create s3 user")
			return subreconciler.Requeue()
		}
		return subreconciler.ContinueReconciling()
	case err != nil:
		r.logger.Error(err, "failed to get s3 user")
		return subreconciler.Requeue()
	default:
		desiredS3user, err := r.assembleS3User()
		if err != nil {
			r.logger.Error(err, "failed to assemble s3 user")
			return subreconciler.Requeue()
		}
		if !apiequality.Semantic.DeepEqual(desiredS3user.Spec, existingS3User.Spec) {
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
	status := s3v1alpha1.S3UserClaimStatus{
		Quota:      r.s3UserClaim.Spec.Quota,
		S3UserName: r.s3UserName,
	}

	if !apiequality.Semantic.DeepEqual(r.s3UserClaim.Status, status) {
		r.s3UserClaim.Status = status
		if err := r.Status().Update(ctx, r.s3UserClaim); err != nil {
			r.logger.Error(err, "failed to update s3 user claim")
			return subreconciler.Requeue()
		}
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) assembleS3User() (*s3v1alpha1.S3User, error) {
	claimRef, err := reference.GetReference(r.scheme, r.s3UserClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to create claim reference, %w", err)
	}

	s3user := &s3v1alpha1.S3User{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.s3UserName,
		},
		Spec: s3v1alpha1.S3UserSpec{
			S3UserClass: r.s3UserClass,
			Quota: &s3v1alpha1.UserQuota{
				MaxSize:    r.s3UserClaim.Spec.Quota.MaxSize,
				MaxObjects: r.s3UserClaim.Spec.Quota.MaxObjects,
			},
			ClaimRef: claimRef,
		},
	}

	return s3user, nil
}

func (r *Reconciler) assembleAdminSecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.s3UserClaim.Namespace,
			Name:      r.s3UserClaim.Spec.AdminSecret,
		},
		Data: map[string][]byte{
			consts.DataKeyAccessKey: []byte(r.cephUser.Keys[0].AccessKey),
			consts.DataKeySecretKey: []byte(r.cephUser.Keys[0].SecretKey),
		},
	}
}
