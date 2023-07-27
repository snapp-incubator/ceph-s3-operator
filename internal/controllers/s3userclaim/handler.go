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
	"strings"

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
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
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
	clusterResourceQuota   *openshiftquota.ClusterResourceQuota
	s3UserClaim            *s3v1alpha1.S3UserClaim
	cephUser               admin.User
	cephTenant             string
	cephUserId             string
	cephUserFullId         string
	cephDisplayName        string
	s3UserName             string
	readonlyCephUserId     string
	readonlyCephUserFullId string

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
		r.ensureReadonlySubuser,
		// retrieve the ceph user to have keys of subuser at hand
		r.retrieveCephUser,
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
	// Only alphanumeric characters and underscore are allowed for tenant name
	k8sNameSpecialChars := regexp.MustCompile(`[.-]`)
	namespace := k8sNameSpecialChars.ReplaceAllString(r.s3UserClaim.Namespace, "_")
	clusterName := k8sNameSpecialChars.ReplaceAllString(r.clusterName, "_")
	r.cephTenant = fmt.Sprintf("%s__%s", clusterName, namespace)
	r.cephUserId = r.s3UserClaim.Name
	// Ceph-SDK functions that involve retrieving the user such as GetQuota, GetUser and even SetUser,
	// required tenant name in UID field.
	r.cephUserFullId = fmt.Sprintf("%s$%s", r.cephTenant, r.cephUserId)
	r.cephDisplayName = fmt.Sprintf("%s in %s.%s", r.s3UserClaim.Name, r.s3UserClaim.Namespace, r.clusterName)

	r.readonlyCephUserId = "readonly"
	r.readonlyCephUserFullId = fmt.Sprintf("%s:%s", r.cephUserFullId, r.readonlyCephUserId)

	r.s3UserName = fmt.Sprintf("%s.%s", r.s3UserClaim.Namespace, r.s3UserClaim.Name)

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureCephUser(ctx context.Context) (*ctrl.Result, error) {
	desiredUser := admin.User{
		ID:          r.cephUserFullId,
		DisplayName: r.cephDisplayName,
	}

	switch exitingUser, err := r.rgwClient.GetUser(ctx, desiredUser); {
	case err == nil:
		r.cephUser = exitingUser
	case errors.Is(err, admin.ErrNoSuchUser):
		user, err := r.rgwClient.CreateUser(ctx, desiredUser)
		if err != nil {
			r.logger.Error(err, "failed to create ceph user", "userId", desiredUser.ID)
			return subreconciler.Requeue()
		}
		r.cephUser = user
	default:
		r.logger.Error(err, "failed to get ceph user")
		return subreconciler.Requeue()
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureCephUserQuota(ctx context.Context) (*ctrl.Result, error) {
	desiredQuota := admin.QuotaSpec{
		UID:        r.cephUserFullId,
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

		r.cephUser.UserQuota = desiredQuota
		return subreconciler.ContinueReconciling()
	default:
		r.logger.Error(err, "failed to get user quota")
		return subreconciler.Requeue()
	}
}

func (r *Reconciler) ensureReadonlySubuser(ctx context.Context) (*ctrl.Result, error) {
	desiredSubuser := admin.SubuserSpec{
		Name:    r.readonlyCephUserId,
		Access:  admin.SubuserAccessRead,
		KeyType: pointer.String(consts.CephKeyTypeS3),
	}

	for _, subuser := range r.cephUser.Subusers {
		if subuser.Name == r.readonlyCephUserFullId {
			return subreconciler.ContinueReconciling()
		}
	}

	if err := r.rgwClient.CreateSubuser(ctx, admin.User{ID: r.cephUserFullId}, desiredSubuser); err != nil {
		r.logger.Error(err, "failed to create subuser")
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) retrieveCephUser(ctx context.Context) (*ctrl.Result, error) {
	retrievedUser, err := r.rgwClient.GetUser(ctx, admin.User{ID: r.cephUserFullId})
	if err != nil {
		r.logger.Error(err, "failed to retrieve ceph user")
		return subreconciler.Requeue()
	}

	r.cephUser = retrievedUser
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureAdminSecret(ctx context.Context) (*ctrl.Result, error) {
	assembledSecret, err := r.assembleCephUserSecret(r.cephUserFullId, r.s3UserClaim.Spec.AdminSecret)
	if err != nil {
		r.logger.Error(err, "failed to assemble admin secret")
		return subreconciler.Requeue()
	}
	return r.ensureSecret(ctx, assembledSecret)
}

func (r *Reconciler) ensureReadonlySecret(ctx context.Context) (*ctrl.Result, error) {
	assembledSecret, err := r.assembleCephUserSecret(r.readonlyCephUserFullId, r.s3UserClaim.Spec.ReadonlySecret)
	if err != nil {
		r.logger.Error(err, "failed to assemble readonly secret")
		return subreconciler.Requeue()
	}
	return r.ensureSecret(ctx, assembledSecret)
}

// ensureSecret ensures the passed secret exists and is controlled by r.s3UserClaim
func (r *Reconciler) ensureSecret(ctx context.Context, secret *v1.Secret) (*ctrl.Result, error) {
	existingSecret := &v1.Secret{}
	switch err := r.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, existingSecret); {
	case apierrors.IsNotFound(err):
		if err := r.Create(ctx, secret); err != nil {
			r.logger.Error(err, "failed to create secret", "name", secret.Name)
			return subreconciler.Requeue()
		}
	case err != nil:
		r.logger.Error(err, "failed to get secret", "name", secret.Name)
		return subreconciler.Requeue()
	default:
		if !apiequality.Semantic.DeepEqual(existingSecret.Data, secret.Data) ||
			!metav1.IsControlledBy(existingSecret, r.s3UserClaim) {
			existingSecret.Data = secret.Data
			if err := ctrl.SetControllerReference(r.s3UserClaim, existingSecret, r.scheme); err != nil {
				return nil, err
			}
			if err := r.Update(ctx, existingSecret); err != nil {
				r.logger.Error(err, "failed to update secret", "name", secret.Name)
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
			if strings.Contains(err.Error(), genericregistry.OptimisticLockErrorMsg) {
				r.logger.Info("re-queuing item due to optimistic locking on resource", "error", err.Error())
			} else {
				r.logger.Error(err, "failed to update s3 user claim")
			}
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

// assembleCephUserSecret tries to find a key for the given userName and assembles a secret
// with accessKey and secretKey of the found key
func (r *Reconciler) assembleCephUserSecret(userName, secretName string) (*v1.Secret, error) {
	var existingKey *admin.UserKeySpec
	for _, key := range r.cephUser.Keys {
		if key.User == userName {
			existingKey = &key
			break
		}
	}

	if existingKey == nil {
		return nil, fmt.Errorf("no key found for user %s", userName)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.s3UserClaim.Namespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			consts.DataKeyAccessKey: []byte(existingKey.AccessKey),
			consts.DataKeySecretKey: []byte(existingKey.SecretKey),
		},
	}

	if err := ctrl.SetControllerReference(r.s3UserClaim, secret, r.scheme); err != nil {
		return nil, err
	}

	return secret, nil
}
