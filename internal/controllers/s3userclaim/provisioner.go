package s3userclaim

import (
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/opdev/subreconciler"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/client-go/tools/reference"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

// Provision provisions the required resources for the s3UserClaim object
func (r *Reconciler) Provision(ctx context.Context) (ctrl.Result, error) {
	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.ensureCephUser,
		r.ensureCephUserQuota,
		r.ensureReadonlySubuser,
		r.ensureOtherSubusers,
		// retrieve the ceph user to have keys of subuser at hand
		r.retrieveCephUser,
		r.ensureAdminSecret,
		r.ensureReadonlySecret,
		r.ensureOtherSubusersSecret,
		r.ensureS3User,
		r.updateS3UserClaimStatus,
		r.addCleanupFinalizer,
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) ensureCephUser(ctx context.Context) (*ctrl.Result, error) {
	desiredUser := admin.User{
		ID:          r.cephUserFullId,
		DisplayName: r.cephDisplayName,
		MaxBuckets:  pointer.Int(r.s3UserClaim.Spec.Quota.MaxBuckets),
	}
	logger := r.logger.WithValues("userId", desiredUser.ID)

	switch existingUser, err := r.rgwClient.GetUser(ctx, desiredUser); {
	case err == nil:
		if existingUser.MaxBuckets != desiredUser.MaxBuckets {
			existingUser, err = r.rgwClient.ModifyUser(ctx, desiredUser)
			if err != nil {
				logger.Error(err, "failed to update ceph user", "userId", desiredUser.ID)
				return subreconciler.Requeue()
			}
		}
		r.cephUser = existingUser
	case goerrors.Is(err, admin.ErrNoSuchUser):
		user, err := r.rgwClient.CreateUser(ctx, desiredUser)
		if err != nil {
			logger.Error(err, "failed to create ceph user", "userId", desiredUser.ID)
			return subreconciler.Requeue()
		}
		r.cephUser = user
	default:
		logger.Error(err, "failed to get ceph user", "userId", desiredUser.ID)
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

	switch existingQuota, err := r.rgwClient.GetUserQuota(ctx, desiredQuota); {
	case err == nil:
		// We need to compare field by field. DeepEqual won't work here as the retrieved quota doesn't have all
		// the fields set to their real value (e.g. UID will be empty although the real user has UID)
		if *existingQuota.Enabled != *desiredQuota.Enabled ||
			*existingQuota.MaxSize != *desiredQuota.MaxSize ||
			*existingQuota.MaxObjects != *desiredQuota.MaxObjects {
			if err := r.rgwClient.SetUserQuota(ctx, desiredQuota); err != nil {
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

func (r *Reconciler) ensureOtherSubusers(ctx context.Context) (*ctrl.Result, error) {
	specSubUsers := r.s3UserClaim.Spec.SubUsers
	// Create a hashmap to move all spec and ceph subUsers to it
	subUserFullIdSet := make(map[string]string)

	// Tag specSubUsers with "create"
	for _, subUser := range specSubUsers {
		cephSubUserFullId := cephSubUserFullIdMaker(r.cephUserFullId, subUser)
		subUserFullIdSet[cephSubUserFullId] = consts.SubUserTagCreate
	}

	// Add read-only subUser to subUsers to prevent if from removing
	subUserFullIdSet[r.readonlyCephUserFullId] = consts.SubUserTagCreate

	// Tag cephSubUsers as remove if they are not already in the hashmap and remove them otherwise
	// since they are already available on ceph and not needed to created.
	for _, cephsubUser := range r.cephUser.Subusers {
		_, exists := subUserFullIdSet[cephsubUser.Name]
		if exists {
			delete(subUserFullIdSet, cephsubUser.Name)
		} else {
			subUserFullIdSet[cephsubUser.Name] = consts.SubUserTagRemove
		}
	}

	// Iterate over the subUsers hashmap and create or remove subUsers according to their tags.
	for subUserFullId, tag := range subUserFullIdSet {
		desiredSubuser := admin.SubuserSpec{
			Name:    subUserFullId,
			Access:  admin.SubuserAccessNone,
			KeyType: pointer.String(consts.CephKeyTypeS3),
		}
		if tag == consts.SubUserTagCreate {
			// Create the subuser
			r.logger.Info(fmt.Sprintf("Create subUser: %s", subUserFullId))
			if err := r.rgwClient.CreateSubuser(ctx, admin.User{ID: r.cephUserFullId}, desiredSubuser); err != nil {
				r.logger.Error(err, "failed to create subUser")
				return subreconciler.Requeue()
			}
		} else {
			// Delete the subuser
			err := r.rgwClient.RemoveSubuser(ctx, admin.User{ID: r.cephUserFullId}, desiredSubuser)
			r.logger.Info(fmt.Sprintf("Remove subUser: %s", subUserFullId))
			if err != nil {
				r.logger.Error(err, "failed to remove subUser")
				return subreconciler.Requeue()
			}
			// Extrace subUser name from the subUserFullId
			subUser, err := subUserNameExtractor(subUserFullId)
			if err != nil {
				r.logger.Error(err, "failed to remove s3SubUserSecret")
				return subreconciler.Requeue()
			}
			subUserSecretName := subUserSecretNameMaker(r.s3UserClaim.Name, subUser)
			subUserSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: r.s3UserClaim.Namespace,
					Name:      subUserSecretName,
				},
			}
			// Delete subUser secret
			switch err := r.Delete(ctx, subUserSecret); {
			case apierrors.IsNotFound(err):
				return subreconciler.ContinueReconciling()
			case err != nil:
				r.logger.Error(err, "failed to remove s3SubUserSecret")
				return subreconciler.Requeue()
			default:
				return subreconciler.ContinueReconciling()
			}
		}
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

func (r *Reconciler) ensureOtherSubusersSecret(ctx context.Context) (*ctrl.Result, error) {
	for _, subUser := range r.s3UserClaim.Spec.SubUsers {
		cephSubUserFullId := cephSubUserFullIdMaker(r.cephUserFullId, subUser)
		SubUserSecretName := subUserSecretNameMaker(r.s3UserClaim.Name, subUser)
		assembledSecret, err := r.assembleCephUserSecret(cephSubUserFullId, SubUserSecretName)
		if err != nil {
			r.logger.Error(err, "failed to assemble other subUsers secret")
			return subreconciler.Requeue()
		}
		result, err := r.ensureSecret(ctx, assembledSecret)
		if result != nil || err != nil {
			return result, err
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
		SubUsers:   r.s3UserClaim.Spec.SubUsers,
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

func (r *Reconciler) addCleanupFinalizer(ctx context.Context) (*ctrl.Result, error) {
	if objUpdated := controllerutil.AddFinalizer(r.s3UserClaim, consts.S3UserClaimCleanupFinalizer); objUpdated {
		if err := r.Update(ctx, r.s3UserClaim); err != nil {
			r.logger.Error(err, "failed to update s3UserClaim")
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}

// ensureSecret ensures the passed secret exists and is controlled by r.s3UserClaim
func (r *Reconciler) ensureSecret(ctx context.Context, secret *corev1.Secret) (*ctrl.Result, error) {
	existingSecret := &corev1.Secret{}
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
				MaxBuckets: r.s3UserClaim.Spec.Quota.MaxBuckets,
			},
			ClaimRef: claimRef,
		},
	}

	return s3user, nil
}

// assembleCephUserSecret tries to find a key for the given userName and assembles a secret
// with accessKey and secretKey of the found key
func (r *Reconciler) assembleCephUserSecret(userName, secretName string) (*corev1.Secret, error) {
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

	secret := &corev1.Secret{
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
