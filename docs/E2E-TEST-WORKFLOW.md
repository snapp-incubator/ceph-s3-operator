# End-to-END Test Workflow

The e2e tests are performed via [Kuttle](https://kuttl.dev/). Use the bellow command to run the tests:

```bash
make e2e-test
```

## Test Workflow

Here is the test workflow:

| Step | Action                                                       | Assertion                                                    |
| ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| 0    | Install Cert Manager                                         | check if cert manager pods are ready                         |
| 1    | Install Ceph cluster                                         | Check if ceph cluster pod is ready                           |
| 1    | Install resource quota CRD                                   | No assertion                                                 |
| 1    | Install the operator                                         | Check if operator pod is ready                               |
| 2    | Create S3UserClaims                                          | S3UserClaim CR<br />S3User CR<br />Created user on ceph-rgw  |
| 3    | Create S3Buckets: One with retain and one with delete DeletionPolicy mode. | S3Bucket CR<br />S3Bucket on ceph-rgw via aws-cli<br />aws-cli with user credentials **can** create or delete objects on the bucket |
| 4    | Webhook validation: Update S3Bucket S3UserRef                | Must be **denied** by the bucket validation update webhook.  |
| 4    | Webhook validation: Create S3bucket with wrong S3UserRef     | Must be **denied** by the bucket validation create webhook.  |
| 4    | Webhook validation: Delete S3UserClaim                       | Must be **denied** by the user validation delete Webhook.    |
| 5    | Add subUser                                                  | Check if subUser is added to the s3Userclaim status.<br />Check if subUsers secrets are created. |
| 6    | Add Bucket Access                                            | Check if subUsers have correct access to the bucket.         |
| 7    | Delete subUser                                               | Check if deleted subUser doesn't have access to the bucket.  |
| 8    | Delete S3Buckets                                             | DeletionPolicy on delete: bucket must **be** deleted.<br />DeletionPolicy on retain: bucket must **not be** deleted. |
| 9    | Delete S3UserClaim for Sample user                           | S3UserClaim and S3User CRs are deleted.                      |
| 10   | Delete S3bucket and S3UserClaim for Extra user               | S3bucket, S3UserClaim and S3User CRs are deleted.            |
