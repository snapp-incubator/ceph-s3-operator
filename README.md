# S3 Operator

## Introduction

At Snapp! we are using Ceph object storage to have S3 for users, this operator is here
to make working with S3 easier and more fun.

## Objects

Following object is defined for each namespace:

```yaml
apiVersion: s3.snappcloud.io/v1alpha
Kind: S3UserClaim
metadata:
  name: myuser
  namespace: dispatching-test
Spec:
  s3ClassName (optional) have default value
  readOnlySecret (optional)
  adminSecret  (required)
Status:
  quota: (max_buckets, max_size, max_objects)
```

and this object is cluster scoped:

```yaml
apiVersion: s3.snappcloud.io/v1alpha
Kind: S3User
metadata:
  name: myuser
Spec:
  s3ClassName
  claimPolicy: Delete / Retain
  claimRef:
	apiVersion: v1
	kind: PersistentVolumeClaim
	name: redis-data-rediscentral-0
	namespace: baly-ode-001
	resourceVersion: "267741823"
	uid: ff1eddc9-fb16-4762-ba43-f193ed23b92d  
  Quota:
    (max_buckets, max_size, max_objects)
  Status:
```
