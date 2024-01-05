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

package v1alpha1

import (
	"context"
	goerrors "errors"
	"fmt"
	"time"

	openshiftquota "github.com/openshift/api/quota/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/snapp-cab/ceph-s3-operator/pkg/consts"
)

const (
	internalErrorMessage = "internal error"
)

var (
	s3userclaimlog = logf.Log.WithName("s3userclaim-resource")
	runtimeClient  client.Client
	uncachedReader client.Reader

	ValidationTimeout time.Duration
)

func (suc *S3UserClaim) SetupWebhookWithManager(mgr ctrl.Manager) error {
	runtimeClient = mgr.GetClient()
	// uncachedReader will be used in cases that having the most up-to-date state of objects is necessary for the proper functioning.
	// One such instance is when retrieving all S3UserClaim objects to find the aggregated quota requests.
	// In this case, the absence a recently created S3UserClaim could lead to an aggregated value lower than the real value.
	// https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md#q-my-cache-might-be-stale-if-i-read-from-a-cache-how-should-i-deal-with-that
	uncachedReader = mgr.GetAPIReader()

	return ctrl.NewWebhookManagedBy(mgr).
		For(suc).
		Complete()
}

//+kubebuilder:webhook:path=/validate-s3-snappcloud-io-v1alpha1-s3userclaim,mutating=false,failurePolicy=fail,sideEffects=None,groups=s3.snappcloud.io,resources=s3userclaims,verbs=create;update;delete,versions=v1alpha1,name=vs3userclaim.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &S3UserClaim{}

func (suc *S3UserClaim) ValidateCreate() error {
	s3userclaimlog.Info("validate create", "name", suc.Name)
	allErrs := field.ErrorList{}

	allErrs = validateQuota(suc, allErrs)

	secretNames := []string{suc.Spec.AdminSecret, suc.Spec.ReadonlySecret}
	allErrs = validateSecrets(secretNames, suc.Namespace, allErrs)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(suc.GroupVersionKind().GroupKind(), suc.Name, allErrs)
}

func (suc *S3UserClaim) ValidateUpdate(old runtime.Object) error {
	s3userclaimlog.Info("validate update", "name", suc.Name)
	allErrs := field.ErrorList{}

	// Err if s3UserClass is changed
	oldS3UserClaim, ok := old.(*S3UserClaim)
	if !ok {
		s3userclaimlog.Info("invalid object passed as old s3UserClaim", "type", old.GetObjectKind())
		return fmt.Errorf(internalErrorMessage)
	}
	if suc.Spec.S3UserClass != oldS3UserClaim.Spec.S3UserClass {
		allErrs = append(
			allErrs,
			field.Forbidden(field.NewPath("spec").Child("s3UserClass"), consts.S3UserClassImmutableErrMessage),
		)
	}

	allErrs = validateQuota(suc, allErrs)

	// validate against updated secret names
	var secretNames []string
	if oldS3UserClaim.Spec.AdminSecret != suc.Spec.AdminSecret {
		secretNames = append(secretNames, suc.Spec.AdminSecret)
	}
	if oldS3UserClaim.Spec.ReadonlySecret != suc.Spec.ReadonlySecret {
		secretNames = append(secretNames, suc.Spec.ReadonlySecret)
	}

	allErrs = validateSecrets(secretNames, suc.Namespace, allErrs)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(suc.GroupVersionKind().GroupKind(), suc.Name, allErrs)
}

func (suc *S3UserClaim) ValidateDelete() error {
	s3userclaimlog.Info("validate delete", "name", suc.Name)

	ctx, cancel := context.WithTimeout(context.Background(), ValidationTimeout)
	defer cancel()

	// Err if there are existing buckets in the namespace
	s3BucketList := &S3BucketList{}
	err := runtimeClient.List(ctx, s3BucketList, client.InNamespace(suc.Namespace))
	if err != nil {
		s3userclaimlog.Error(err, "failed to list buckets")
		return err
	}

	for _, bucket := range s3BucketList.Items {
		if bucket.Spec.S3UserRef == suc.Name {
			return apierrors.NewBadRequest("There are existing buckets associated with this userclaim. " +
				"Please first delete them and try again.")
		}
	}
	return nil
}

func validateQuota(suc *S3UserClaim, allErrs field.ErrorList) field.ErrorList {
	ctx, cancel := context.WithTimeout(context.Background(), ValidationTimeout)
	defer cancel()

	quotaFieldPath := field.NewPath("spec").Child("quota")

	// TODO(therealak12): refactor the code as there are similarities between two quota validator functions

	switch err := validateAgainstNamespaceQuota(ctx, suc); {
	case err == consts.ErrExceededNamespaceQuota:
		allErrs = append(allErrs, field.Forbidden(quotaFieldPath, err.Error()))
	case err != nil:
		s3userclaimlog.Error(err, "failed to validate against cluster quota")
		allErrs = append(allErrs, field.InternalError(quotaFieldPath, fmt.Errorf(consts.ContactCloudTeamErrMessage)))
	}

	switch err := validateAgainstClusterQuota(ctx, suc); {
	case err == consts.ErrExceededClusterQuota:
		allErrs = append(allErrs, field.Forbidden(quotaFieldPath, err.Error()))
	case goerrors.Is(err, consts.ErrClusterQuotaNotDefined):
		allErrs = append(allErrs, field.Forbidden(quotaFieldPath, err.Error()))
	case err != nil:
		s3userclaimlog.Error(err, "failed to validate against cluster quota")
		allErrs = append(allErrs, field.InternalError(quotaFieldPath, fmt.Errorf(consts.ContactCloudTeamErrMessage)))
	}
	return allErrs
}

func validateAgainstNamespaceQuota(ctx context.Context, suc *S3UserClaim) error {
	totalUsedQuota, err := CalculateNamespaceUsedQuota(ctx, uncachedReader, suc, suc.Namespace, true)
	if err != nil {
		return fmt.Errorf("failed to calculate namespace used quota , %w", err)
	}
	// List all quotas in the namespace and validate against them
	resourceQuotaList := &v1.ResourceQuotaList{}
	err = runtimeClient.List(ctx, resourceQuotaList, client.InNamespace(suc.Namespace))
	if err != nil {
		return fmt.Errorf("failed to list resource quotas, %w", err)
	}
	for _, quota := range resourceQuotaList.Items {
		if maxObjects, ok := quota.Spec.Hard[consts.ResourceNameS3MaxObjects]; ok {
			if totalUsedQuota.MaxObjects.Cmp(maxObjects) > 0 {
				return consts.ErrExceededNamespaceQuota
			}
		}
		if maxSize, ok := quota.Spec.Hard[consts.ResourceNameS3MaxSize]; ok {
			if totalUsedQuota.MaxSize.Cmp(maxSize) > 0 {
				return consts.ErrExceededNamespaceQuota
			}
		}
		if maxBuckets, ok := quota.Spec.Hard[consts.ResourceNameS3MaxBuckets]; ok {
			if totalUsedQuota.MaxBuckets > maxBuckets.Value() {
				return consts.ErrExceededNamespaceQuota
			}
		}
	}

	return nil
}

func validateAgainstClusterQuota(ctx context.Context, suc *S3UserClaim) error {
	totalClusterUsedQuota, team, err := CalculateClusterUsedQuota(ctx, runtimeClient, suc, true)
	if err != nil {
		return fmt.Errorf("failed to calculate cluster resource used quota , %w", err)
	}

	clusterQuota := &openshiftquota.ClusterResourceQuota{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: team}, clusterQuota); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w, team=%s", consts.ErrClusterQuotaNotDefined, team)
		}
		return fmt.Errorf("failed to get clusterQuota, %w", err)
	}
	// Validate against clusterResourceQuota
	if maxObjects, ok := clusterQuota.Spec.Quota.Hard[consts.ResourceNameS3MaxObjects]; ok {
		if totalClusterUsedQuota.MaxObjects.Cmp(maxObjects) > 0 {
			return consts.ErrExceededClusterQuota
		}
	} else {
		return fmt.Errorf("%w, team=%s", consts.ErrClusterQuotaNotDefined, team)
	}
	if maxSize, ok := clusterQuota.Spec.Quota.Hard[consts.ResourceNameS3MaxSize]; ok {
		if totalClusterUsedQuota.MaxSize.Cmp(maxSize) > 0 {
			return consts.ErrExceededClusterQuota
		}
	} else {
		return fmt.Errorf("%w, team=%s", consts.ErrClusterQuotaNotDefined, team)
	}
	if maxBuckets, ok := clusterQuota.Spec.Quota.Hard[consts.ResourceNameS3MaxBuckets]; ok {
		if totalClusterUsedQuota.MaxBuckets > maxBuckets.Value() {
			return consts.ErrExceededClusterQuota
		}
	} else {
		return fmt.Errorf("%w, team=%s", consts.ErrClusterQuotaNotDefined, team)
	}

	return nil
}

func validateSecrets(secretNames []string, namespace string, allErrs field.ErrorList) field.ErrorList {
	ctx, cancel := context.WithTimeout(context.Background(), ValidationTimeout)
	defer cancel()
	for _, secretName := range secretNames {
		existingSecret := &v1.Secret{}
		switch err := runtimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, existingSecret); {
		case apierrors.IsNotFound(err):
			continue
		case err != nil:
			allErrs = append(allErrs,
				field.InternalError(field.NewPath("spec"), fmt.Errorf("error with getting secret '%s': %w", secretName, err)))
		default:
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec"), fmt.Sprintf("secret %s exists", secretName)))
		}
	}
	return allErrs

}
