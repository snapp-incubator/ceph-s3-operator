package consts

import v1 "k8s.io/api/core/v1"

type CustomError string

func (e CustomError) Error() string { return string(e) }

const (
	LabelTeam = "snappcloud.io/team"

	ResourceNameS3MaxObjects v1.ResourceName = "s3/objects"
	ResourceNameS3MaxSize    v1.ResourceName = "s3/size"
	ResourceNameS3MaxBuckets v1.ResourceName = "s3/buckets"

	QuotaTypeUser = "user"

	DataKeyAccessKey = "accessKey"
	DataKeySecretKey = "secretKey"

	CephKeyTypeS3 = "s3"

	ErrExceededClusterQuota        = CustomError("exceeded cluster quota")
	ErrExceededNamespaceQuota      = CustomError("exceeded namespace quota")
	ErrClusterQuotaNotDefined      = CustomError("cluter quota is not defined")
	S3UserClassImmutableErrMessage = "s3UserClass is immutable"
	ContactCloudTeamErrMessage     = "please contact the cloud team"

	FinalizerPrefix             = "s3.snappcloud.io/"
	S3UserClaimCleanupFinalizer = FinalizerPrefix + "cleanup-s3userclaim"
)
