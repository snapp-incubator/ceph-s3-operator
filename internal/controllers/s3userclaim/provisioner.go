package s3userclaim

import (
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/opdev/subreconciler"
	openshiftquota "github.com/openshift/api/quota/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/client-go/tools/reference"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		r.syncSubusersList,
		// retrieve the ceph user to have keys of subuser at hand
		r.retrieveCephUser,
		r.ensureAdminSecret,
		r.ensureReadonlySecret,
		r.ensureOtherSubusersSecret,
		r.ensureS3User,
		r.updateS3UserClaimStatus,
		r.updateNamespaceQuotaStatusInclusive,
		r.updateClusterQuotaStatusInclusive,
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
		MaxBuckets:  pointer.Int(int(r.s3UserClaim.Spec.Quota.MaxBuckets)),
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
	// retrieve desiredSubusers as string list
	r.desiredSubusersStringList = retrieveSubusersString(r.s3UserClaim.Spec.Subusers)

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

// syncSubusersList creates the new subusers and deletes the ones which have been removed
// from the spec.
// At first, it creates a map from subusers to a tag which can be either "create" or "remove"
// demonstrating the action that we want to be happened on the subuser:
// 1. Subusers which are in the spec list and not in the current ceph users list will be created.
// 2. Subusers which are not in the spec list but are in the current ceph users list will be removed with their secrets.
// 3. Subusers which are common in the both lists will be deleted from the map; hence, no action happens on them.
func (r *Reconciler) syncSubusersList(ctx context.Context) (*ctrl.Result, error) {

	subuserFullIdAccess := r.generateSubuserAccess(r.desiredSubusersStringList,
		r.cephUser.Subusers)

	// Iterate over the subusers map and create or remove subusers according to their tags.
	for subuserFullId, tag := range subuserFullIdAccess {
		desiredSubuser := admin.SubuserSpec{
			Name:    subuserFullId,
			Access:  admin.SubuserAccessNone,
			KeyType: pointer.String(consts.CephKeyTypeS3),
		}
		if tag == consts.SubuserTagCreate {
			if err := r.generateSubuser(ctx, r.cephUserFullId, desiredSubuser); err != nil {
				return subreconciler.Requeue()
			}
		} else {
			if err := r.removeSubuserAndSecret(ctx, r.cephUserFullId, desiredSubuser); err != nil {
				return subreconciler.Requeue()
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
	for _, subuser := range r.desiredSubusersStringList {
		cephSubuserFullId := generateSubuserFullId(r.cephUserFullId, subuser)
		SubuserSecretName := generateSubuserSecretName(r.s3UserClaim.Name, subuser)
		assembledSecret, err := r.assembleCephUserSecret(cephSubuserFullId, SubuserSecretName)
		if err != nil {
			r.logger.Error(err, "failed to assemble other subusers secret")
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
		Subusers:   r.s3UserClaim.Spec.Subusers,
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
func (r *Reconciler) updateNamespaceQuotaStatusInclusive(ctx context.Context) (*ctrl.Result, error) {
	return r.updateNamespaceQuotaStatus(ctx, true)
}

func (r *Reconciler) updateNamespaceQuotaStatusExclusive(ctx context.Context) (*ctrl.Result, error) {
	return r.updateNamespaceQuotaStatus(ctx, false)
}

func (r *Reconciler) updateNamespaceQuotaStatus(ctx context.Context, addCurrentQuota bool) (*ctrl.Result, error) {
	var err error
	// sum up all quotas in the namespace
	r.namespaceUsedQuota, err = s3v1alpha1.CalculateNamespaceUsedQuota(ctx, r.uncachedReader, r.s3UserClaim, addCurrentQuota)
	if err != nil {
		r.logger.Error(err, "failed to calculate namespace used quota")
		return subreconciler.Requeue()
	}
	// update the resource quota status used field
	resourceQuotaList := &corev1.ResourceQuotaList{}
	err = r.Client.List(ctx, resourceQuotaList, client.InNamespace(r.s3UserClaimNamespace))
	if err != nil {
		r.logger.Error(err, "failed to list resource quotas")
		return subreconciler.Requeue()
	}
	for _, quota := range resourceQuotaList.Items {
		status := quota.Status.DeepCopy()
		if status.Used == nil {
			status.Used = corev1.ResourceList{}
		}
		assignUsedQuotaToResourceStatus(status, r.namespaceUsedQuota)

		if !apiequality.Semantic.DeepEqual(quota.Status, *status) {
			quota.Status = *status
			if err := r.Status().Update(ctx, &quota); err != nil {
				if strings.Contains(err.Error(), genericregistry.OptimisticLockErrorMsg) {
					r.logger.Info("re-queuing item due to optimistic locking on resource", "error", err.Error())
				} else {
					r.logger.Error(err, "failed to update namespace quota status")
				}
				return subreconciler.Requeue()
			}
		}
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) updateClusterQuotaStatusInclusive(ctx context.Context) (*ctrl.Result, error) {
	return r.updateClusterQuotaStatus(ctx, true)
}

func (r *Reconciler) updateClusterQuotaStatusExclusive(ctx context.Context) (*ctrl.Result, error) {
	return r.updateClusterQuotaStatus(ctx, false)
}

func (r *Reconciler) updateClusterQuotaStatus(ctx context.Context, addCurrentQuota bool) (*ctrl.Result, error) {
	// sum up all quotas in the cluster related to the team label
	totalClusterUsedQuota, team, err := s3v1alpha1.CalculateClusterUsedQuota(ctx, r.Client, r.s3UserClaim, addCurrentQuota)
	if err != nil {
		r.logger.Error(err, "failed to calculate cluster used quota")
		return subreconciler.Requeue()
	}
	// update the cluster resource quota status
	clusterQuota := &openshiftquota.ClusterResourceQuota{}
	if err := r.Get(ctx, types.NamespacedName{Name: team}, clusterQuota); err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Error(err, consts.ErrClusterQuotaNotDefined.Error(), "team", team)
			return subreconciler.Requeue()
		}
		r.logger.Error(err, "failed to get clusterQuota")
		return subreconciler.Requeue()
	}
	status := clusterQuota.Status.DeepCopy()
	if status.Total.Used == nil {
		status.Total.Used = corev1.ResourceList{}
	}
	// update total field of the status
	assignUsedQuotaToResourceStatus(&status.Total, totalClusterUsedQuota)

	// update namespace field of the status
	status.Namespaces = r.assignNamespaceQuotatoResourceStatus(status.Namespaces)

	if !apiequality.Semantic.DeepEqual(clusterQuota.Status, *status) {
		clusterQuota.Status = *status
		if err := r.Status().Update(ctx, clusterQuota); err != nil {
			if strings.Contains(err.Error(), genericregistry.OptimisticLockErrorMsg) {
				r.logger.Info("re-queuing item due to optimistic locking on resource", "error", err.Error())
			} else {
				r.logger.Error(err, "failed to update cluster resource quota status")
			}
			return subreconciler.Requeue()
		}
	}

	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) assignNamespaceQuotatoResourceStatus(statusNamespaces openshiftquota.ResourceQuotasStatusByNamespace) openshiftquota.ResourceQuotasStatusByNamespace {
	if r.namespaceUsedQuota == nil {
		r.logger.Info("Warning: unable to find the namespace used quota while updating the cluster resource quota",
			"namespace", r.s3UserClaimNamespace)
		r.namespaceUsedQuota = &s3v1alpha1.UserQuota{}
	}
	// update the namespace status in cluster resource quota if it's there
	for i, namespaceQuota := range statusNamespaces {
		if namespaceQuota.Namespace == r.s3UserClaimNamespace {
			assignUsedQuotaToResourceStatus(&statusNamespaces[i].Status, r.namespaceUsedQuota)
			return statusNamespaces
		}
	}
	// create a new item for the current namespace if it's not already there
	namepaceQuotaStatus := corev1.ResourceQuotaStatus{}
	assignUsedQuotaToResourceStatus(&namepaceQuotaStatus, r.namespaceUsedQuota)
	namespaceQuota := openshiftquota.ResourceQuotaStatusByNamespace{Namespace: r.s3UserClaimNamespace,
		Status: namepaceQuotaStatus}
	statusNamespaces = append(statusNamespaces, namespaceQuota)
	return statusNamespaces
}

func assignUsedQuotaToResourceStatus(resourceQuotaStatus *corev1.ResourceQuotaStatus, usedQuota *s3v1alpha1.UserQuota) {
	if resourceQuotaStatus.Used == nil {
		resourceQuotaStatus.Used = corev1.ResourceList{}
	}
	resourceQuotaStatus.Used[consts.ResourceNameS3MaxObjects] = usedQuota.MaxObjects
	resourceQuotaStatus.Used[consts.ResourceNameS3MaxSize] = usedQuota.MaxSize
	resourceQuotaStatus.Used[consts.ResourceNameS3MaxBuckets] = *resource.NewQuantity(usedQuota.MaxBuckets, resource.DecimalSI)
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

func (r *Reconciler) generateSubuserAccess(desiredSubusers []string,
	currentSubusers []admin.SubuserSpec) map[string]string {
	// Create a map to move all spec and ceph subusers to it
	subuserFullIdAccess := make(map[string]string)

	// Tag specSubusers with "create"
	for _, subuser := range desiredSubusers {
		cephSubuserFullId := generateSubuserFullId(r.cephUserFullId, subuser)
		subuserFullIdAccess[cephSubuserFullId] = consts.SubuserTagCreate
	}

	// Add read-only subuser to subusers to prevent removing it
	subuserFullIdAccess[r.readonlyCephUserFullId] = consts.SubuserTagCreate

	// Tag cephSubusers as remove if they are not already in the map and remove them otherwise
	// since they are already available on ceph and not needed to created.
	for _, currentSubuser := range r.cephUser.Subusers {
		_, exists := subuserFullIdAccess[currentSubuser.Name]
		if exists {
			delete(subuserFullIdAccess, currentSubuser.Name)
		} else {
			subuserFullIdAccess[currentSubuser.Name] = consts.SubuserTagRemove
		}
	}
	return subuserFullIdAccess
}

func (r *Reconciler) generateSubuser(ctx context.Context, cephUserFullId string,
	desiredSubuser admin.SubuserSpec) error {
	r.logger.Info(fmt.Sprintf("Create subuser: %s", desiredSubuser.Name))
	if err := r.rgwClient.CreateSubuser(ctx, admin.User{ID: cephUserFullId}, desiredSubuser); err != nil {
		r.logger.Error(err, "failed to create subuser")
		return err
	}
	return nil
}

func (r *Reconciler) removeSubuserAndSecret(ctx context.Context, cephUserFullId string,
	subuserToRemove admin.SubuserSpec) error {
	r.logger.Info(fmt.Sprintf("Remove subuser: %s", subuserToRemove.Name))
	if err := r.rgwClient.RemoveSubuser(ctx, admin.User{ID: cephUserFullId},
		subuserToRemove); err != nil {
		r.logger.Error(err, "failed to remove subuser")
		return err
	}
	subuser, err := extractSubuserName(subuserToRemove.Name)
	if err != nil {
		r.logger.Error(err, "failed to remove s3SubuserSecret")
		return err
	}
	// Delete subuser secret
	subuserSecretName := generateSubuserSecretName(r.s3UserClaim.Name, subuser)
	subuserSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.s3UserClaim.Namespace,
			Name:      subuserSecretName,
		},
	}
	switch err := r.Delete(ctx, subuserSecret); {
	case apierrors.IsNotFound(err):
		return nil
	case err != nil:
		r.logger.Error(err, "failed to remove s3SubuserSecret")
		return err
	default:
		return nil
	}
}

func retrieveSubusersString(desiredSubusers []s3v1alpha1.Subuser) []string {
	subusersStringList := make([]string, len(desiredSubusers))
	for i, desiredSubuser := range desiredSubusers {
		subusersStringList[i] = string(desiredSubuser)
	}
	return subusersStringList
}
