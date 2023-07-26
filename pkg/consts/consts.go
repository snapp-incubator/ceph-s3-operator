package consts

import v1 "k8s.io/api/core/v1"

const (
	LabelTeam = "snappcloud.io/team"

	ResourceNameS3MaxObjects v1.ResourceName = "s3/objects"
	ResourceNameS3MaxSize    v1.ResourceName = "s3/size"

	QuotaTypeUser = "user"

	DataKeyAccessKey = "accessKey"
	DataKeySecretKey = "secretKey"
)
