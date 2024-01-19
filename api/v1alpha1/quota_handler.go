package v1alpha1

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/snapp-incubator/ceph-s3-operator/pkg/consts"
)

func CalculateNamespaceUsedQuota(ctx context.Context, uncachedReader client.Reader,
	suc *S3UserClaim, namespace string, addCurrentQuota bool) (*UserQuota, error) {
	totalUsedQuota := UserQuota{}
	if suc == nil {
		return &totalUsedQuota, fmt.Errorf("s3userclaim pointer is nil")
	}
	// List all s3UserClaims in the namespace
	s3UserClaimList := &S3UserClaimList{}
	if err := uncachedReader.List(ctx, s3UserClaimList, client.InNamespace(namespace)); err != nil {
		return &totalUsedQuota, fmt.Errorf("failed to list s3 user claims, %w", err)
	}

	// Sum all resource requests
	for _, claim := range s3UserClaimList.Items {
		if claim.Name != suc.Name {
			totalUsedQuota.MaxObjects.Add(claim.Spec.Quota.MaxObjects)
			totalUsedQuota.MaxSize.Add(claim.Spec.Quota.MaxSize)
			totalUsedQuota.MaxBuckets += claim.Spec.Quota.MaxBuckets
		}
	}
	// Don't add the current user quota if the function is called by the cleaner
	if addCurrentQuota {
		totalUsedQuota.MaxObjects.Add(suc.Spec.Quota.MaxObjects)
		totalUsedQuota.MaxSize.Add(suc.Spec.Quota.MaxSize)
		totalUsedQuota.MaxBuckets += suc.Spec.Quota.MaxBuckets
	}
	return &totalUsedQuota, nil
}

func CalculateClusterUsedQuota(ctx context.Context, runtimeClient client.Client,
	suc *S3UserClaim, addCurrentQuota bool) (*UserQuota, string, error) {
	totalClusterUsedQuota := UserQuota{}
	// Find team's clusterResourceQuota
	team, err := findTeam(ctx, runtimeClient, suc)
	if err != nil {
		return &totalClusterUsedQuota, "", fmt.Errorf("failed to find team, %w", err)
	}

	// Sum all resource requests in team's namespaces
	namespaces, err := findTeamNamespaces(ctx, runtimeClient, team)
	if err != nil {
		return &totalClusterUsedQuota, team, fmt.Errorf("failed to find team namespaces, %w", err)
	}
	for _, ns := range namespaces {
		s3UserClaimList := &S3UserClaimList{}
		if err := uncachedReader.List(ctx, s3UserClaimList, client.InNamespace(ns)); err != nil {
			return &totalClusterUsedQuota, team, fmt.Errorf("failed to list s3UserClaims, %w", err)
		}

		for _, claim := range s3UserClaimList.Items {
			if claim.Name != suc.Name || claim.Namespace != suc.Namespace {
				totalClusterUsedQuota.MaxObjects.Add(claim.Spec.Quota.MaxObjects)
				totalClusterUsedQuota.MaxSize.Add(claim.Spec.Quota.MaxSize)
				totalClusterUsedQuota.MaxBuckets += claim.Spec.Quota.MaxBuckets
			}
		}
	}
	// Don't add the current user quota if the function is called by the cleaner
	if addCurrentQuota {
		totalClusterUsedQuota.MaxObjects.Add(suc.Spec.Quota.MaxObjects)
		totalClusterUsedQuota.MaxSize.Add(suc.Spec.Quota.MaxSize)
		totalClusterUsedQuota.MaxBuckets += suc.Spec.Quota.MaxBuckets
	}
	return &totalClusterUsedQuota, team, nil
}

func findTeam(ctx context.Context, runtimeClient client.Client, suc *S3UserClaim) (string, error) {
	ns := &v1.Namespace{}
	if err := runtimeClient.Get(ctx, types.NamespacedName{Name: suc.ObjectMeta.Namespace}, ns); err != nil {
		return "", fmt.Errorf("failed to get namespace, %w", err)
	}

	team, ok := ns.ObjectMeta.Labels[consts.LabelTeam]
	if !ok {
		return "", fmt.Errorf("namespace %s doesn't have team label", ns.ObjectMeta.Name)
	}

	return team, nil
}

func findTeamNamespaces(ctx context.Context, runtimeClient client.Client, team string) ([]string, error) {
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
