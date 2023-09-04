# End-to-END Test Workflow

The e2e tests are performed via [Kuttle](https://kuttl.dev/). Use the bellow command to run the tests:

```bash
kubectl-kuttl test
```

## Test Workflow

Here is the test workflow:

### Installation Steps

0. Prerequisites
    - external-crds
    - Ceph cluster
    - Operator
1. Create S3UserClaims  
2. Create two S3Bucket: One with retain and one with delete DeletionPolicy mode.
3. Update S3Bucket S3UserRef
4. Delete S3UserClaim
5. Delete S3Buckets
6. Delete S3UserClaim

### Assertions

0. No assertion
1. CRs and the user:
    - S3UserClaim CR
    - S3User CR
    - TODO: Created user on ceph-rgw
2. Items:
    - S3Bucket CR
    - S3Bucket on ceph-rgw via aws-cli
    - aws-cli with user credentials **can** create or delete objects on the bucket
3. Must be **denined** by the bucket validation webhook.
4. Must be **denied** by the user validation Webhook.
5. Dependent on the deletionPolicy mode:
    - delete: bucket must **be** deleted.
    - retain: bucket must **not be** deleted.
6. S3UserClaim and S3User CRs are deleted.
