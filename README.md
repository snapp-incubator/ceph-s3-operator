# S3 Operator

## Introduction

At Snapp! we are using Ceph object storage to have S3 for users, this operator is here
to make working with S3 easier and more fun.

## Objects

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
