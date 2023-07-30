package consts

import v1 "k8s.io/api/core/v1"

const (
	LabelTeam = "snappcloud.io/team"

	ResourceNameS3MaxObjects v1.ResourceName = "s3/objects"
	ResourceNameS3MaxSize    v1.ResourceName = "s3/size"

	QuotaTypeUser = "user"

	DataKeyAccessKey = "accessKey"
	DataKeySecretKey = "secretKey"

	CephKeyTypeS3 = "s3"

	ExceededClusterQuotaErrMessage   = "exceeded cluster quota"
	ExceededNamespaceQuotaErrMessage = "exceeded namespace quota"
	S3UserClassImmutableErrMessage   = "s3UserClass is immutable"
	ContactCloudTeamErrMessage       = "please contact the cloud team"

	FinalizerPrefix             = "s3.snappcloud.io/"
	S3UserClaimCleanupFinalizer = FinalizerPrefix + "cleanup-s3userclaim"
)
