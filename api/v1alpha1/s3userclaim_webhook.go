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
	"fmt"
	"time"

	openshiftquota "github.com/openshift/api/quota/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/snapp-incubator/s3-operator/pkg/consts"
)

const (
	internalErrorMessage = "internal error"
)

var (
	s3userclaimlog = logf.Log.WithName("s3userclaim-resource")
	runtimeClient  client.Client

	ValidationTimeout time.Duration
)

func (suc *S3UserClaim) SetupWebhookWithManager(mgr ctrl.Manager) error {
	// Create a new client *without* cache to reliably get objects from API Server.
	// This is necessary as reading from a stale cache could result in missing a newly created object, which
	// could then lead to an aggregated quota value lower than the real value.
	// https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md#q-my-cache-might-be-stale-if-i-read-from-a-cache-how-should-i-deal-with-that
	var err error
	runtimeClient, err = client.New(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	})
	if err != nil {
		return fmt.Errorf("failed to create runtime client, %w", err)
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(suc).
		Complete()
}

//+kubebuilder:webhook:path=/validate-s3-snappcloud-io-v1alpha1-s3userclaim,mutating=false,failurePolicy=fail,sideEffects=None,groups=s3.snappcloud.io,resources=s3userclaims,verbs=create;update,versions=v1alpha1,name=vs3userclaim.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &S3UserClaim{}

func (suc *S3UserClaim) ValidateCreate() error {
	s3userclaimlog.Info("validate create", "name", suc.Name)

	// Validate Quota
	errorList := validateQuota(suc)
	if len(errorList) == 0 {
		return nil
	}

	return apierrors.NewInvalid(suc.GroupVersionKind().GroupKind(), suc.Name, errorList)
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

	// Validate Quota
	errorList := validateQuota(suc)
	allErrs = append(allErrs, errorList...)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(suc.GroupVersionKind().GroupKind(), suc.Name, allErrs)
}

func (suc *S3UserClaim) ValidateDelete() error {
	return nil
}

func validateQuota(suc *S3UserClaim) field.ErrorList {
	ctx, cancel := context.WithTimeout(context.Background(), ValidationTimeout)
	defer cancel()

	errorList := field.ErrorList{}

	quotaFieldPath := field.NewPath("spec").Child("quota")

	// TODO(therealak12): refactor the code as there are similarities between two quota validator functions

	exceeded, err := validateAgainstNamespaceQuota(ctx, suc)
	if err != nil {
		s3userclaimlog.Error(err, "failed to validate against namespace quota")
		errorList = append(errorList, field.InternalError(quotaFieldPath, fmt.Errorf(consts.ContactCloudTeamErrMessage)))
	} else if exceeded {
		errorList = append(errorList, field.Forbidden(quotaFieldPath, consts.ExceededNamespaceQuotaErrMessage))
	}

	exceeded, err = validateAgainstClusterQuota(ctx, suc)
	if err != nil {
		s3userclaimlog.Error(err, "failed to validate against cluster quota")
		errorList = append(errorList, field.InternalError(quotaFieldPath, fmt.Errorf(consts.ContactCloudTeamErrMessage)))
	} else if exceeded {
		errorList = append(errorList, field.Forbidden(quotaFieldPath, consts.ExceededClusterQuotaErrMessage))
	}

	return errorList
}

func validateAgainstNamespaceQuota(ctx context.Context, suc *S3UserClaim) (bool, error) {
	// List all s3UserClaims in the namespace
	s3UserClaimList := &S3UserClaimList{}
	if err := runtimeClient.List(ctx, s3UserClaimList, client.InNamespace(suc.Namespace)); err != nil {
		return false, fmt.Errorf("failed to list s3 user claims, %w", err)
	}

	// Sum all resource requests
	totalMaxObjects := resource.Quantity{}
	totalMaxSize := resource.Quantity{}
	for _, claim := range s3UserClaimList.Items {
		if claim.Name != suc.Name {
			totalMaxObjects.Add(claim.Spec.Quota.MaxObjects)
			totalMaxSize.Add(claim.Spec.Quota.MaxSize)
		}
	}
	totalMaxObjects.Add(suc.Spec.Quota.MaxObjects)
	totalMaxSize.Add(suc.Spec.Quota.MaxSize)

	// List all quotas in the namespace and validate against them
	resourceQuotaList := &v1.ResourceQuotaList{}
	err := runtimeClient.List(ctx, resourceQuotaList, client.InNamespace(suc.Namespace))
	if err != nil {
		return false, fmt.Errorf("failed to list resource quotas, %w", err)
	}
	for _, quota := range resourceQuotaList.Items {
		if maxObjects, ok := quota.Spec.Hard[consts.ResourceNameS3MaxObjects]; ok {
			if totalMaxObjects.Cmp(maxObjects) > 0 {
				return true, nil
			}
		}
		if maxSize, ok := quota.Spec.Hard[consts.ResourceNameS3MaxSize]; ok {
			if totalMaxSize.Cmp(maxSize) > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

func validateAgainstClusterQuota(ctx context.Context, suc *S3UserClaim) (bool, error) {
	// Find team's clusterResourceQuota
	team, err := findTeam(ctx, suc)
	if err != nil {
		return false, fmt.Errorf("faield to find team, %w", err)
	}
	clusterQuota := &openshiftquota.ClusterResourceQuota{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: team}, clusterQuota); err != nil {
		return false, fmt.Errorf("failed to get clusterQuota, %w", err)
	}

	// Sum all resource requests in team's namespaces
	totalMaxObjects := resource.Quantity{}
	totalMaxSize := resource.Quantity{}
	namespaces, err := findTeamNamespaces(ctx, team)
	if err != nil {
		return false, fmt.Errorf("failed to find team namespaces, %w", err)
	}
	for _, ns := range namespaces {
		s3UserClaimList := &S3UserClaimList{}
		if err := runtimeClient.List(ctx, s3UserClaimList, client.InNamespace(ns)); err != nil {
			return false, fmt.Errorf("failed to list s3UserClaims, %w", err)
		}

		for _, claim := range s3UserClaimList.Items {
			if claim.Name != suc.Name || claim.Namespace != suc.Namespace {
				totalMaxObjects.Add(claim.Spec.Quota.MaxObjects)
				totalMaxSize.Add(claim.Spec.Quota.MaxSize)
			}
		}
	}
	totalMaxObjects.Add(suc.Spec.Quota.MaxObjects)
	totalMaxSize.Add(suc.Spec.Quota.MaxSize)

	// Validate against clusterResourceQuota
	if maxObjects, ok := clusterQuota.Spec.Quota.Hard[consts.ResourceNameS3MaxObjects]; ok {
		if totalMaxObjects.Cmp(maxObjects) > 0 {
			return true, nil
		}
	}
	if maxSize, ok := clusterQuota.Spec.Quota.Hard[consts.ResourceNameS3MaxSize]; ok {
		if totalMaxSize.Cmp(maxSize) > 0 {
			return true, nil
		}
	}

	return false, nil
}

func findTeam(ctx context.Context, suc *S3UserClaim) (string, error) {
	ns := &v1.Namespace{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: suc.ObjectMeta.Namespace}, ns); err != nil {
		return "", fmt.Errorf("failed to get namespace, %w", err)
	}

	labels := ns.ObjectMeta.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	team, ok := ns.ObjectMeta.Labels[consts.LabelTeam]
	if !ok {
		return "", fmt.Errorf("namespace %s doesn't have team label", ns.ObjectMeta.Name)
	}

	return team, nil
}

func findTeamNamespaces(ctx context.Context, team string) ([]string, error) {
	var namespaces []string

	namespaceList := &v1.NamespaceList{}
	if err := runtimeClient.List(ctx, namespaceList); err != nil {
		return namespaces, fmt.Errorf("failed to list namespaces, %w", err)
	}

	for _, ns := range namespaceList.Items {
		labels := ns.ObjectMeta.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if nsTeam := labels[consts.LabelTeam]; nsTeam == team {
			namespaces = append(namespaces, ns.ObjectMeta.Name)
		}
	}

	return namespaces, nil
}
