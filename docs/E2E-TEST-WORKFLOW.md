# End-to-END Test Workflow

The e2e tests are performed via [Kuttle](https://kuttl.dev/). Use the bellow command to run the tests:

```bash
kubectl-kuttl test
```

## Test Workflow

Here is the test workflow:

| Step | Action                                                       | Assertion                                                    |
| ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| 0    | Install external-crds                                        | No assertion                                                 |
| 0    | Install Ceph cluster                                         | No assertion                                                 |
| 0    | Run the operator locally                                     | No assertion                                                 |
| 1    | Create S3UserClaims                                          | S3UserClaim CR<br />S3User CR<br />Created user on ceph-rgw  |
| 2    | Create two S3Bucket: One with retain and one with delete DeletionPolicy mode. | S3Bucket CR<br />S3Bucket on ceph-rgw via aws-cli<br />aws-cli with user credentials **can** create or delete objects on the bucket |
| 3    | Webhook validation: Update S3Bucket S3UserRef                | Must be **denied** by the bucket validation update webhook.  |
| 3    | Webhook validation: Create S3bucket with wrong S3UserRef     | Must be **denied** by the bucket validation create webhook.  |
| 3    | Webhook validation: Delete S3UserClaim                       | Must be **denied** by the user validation delete Webhook.    |
| 4    | Delete S3Buckets                                             | DeletionPolicy on delete: bucket must **be** deleted.<br />DeletionPolicy on retain: bucket must **not be** deleted. |
| 5    | Delete S3UserClaim                                           | S3UserClaim and S3User CRs are deleted.                      |
