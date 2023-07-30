package common

import "fmt"

func GetCephUserTenant(clusterName, namespace string) string {
	return fmt.Sprintf("%s__%s", clusterName, namespace)
}

func GetCephUserId(s3UserClaimName string) string {
	return s3UserClaimName
}

func GetCephUserFullId(clusterName, namespace, s3UserClaimName string) string {
	return fmt.Sprintf(
		"%s$%s",
		GetCephUserTenant(clusterName, namespace),
		GetCephUserId(s3UserClaimName),
	)
}
