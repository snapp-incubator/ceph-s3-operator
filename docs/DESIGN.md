# S3 Operator Design

## Decisions:

- Use two CRDs or one CRD with finalizer access denied for users:
    - We add a cleanup finalizer to the S3UserClaim objects. This way we're called upon deleting the objects for
      cleanup.
      However, a user may manually remove the finalizer. In this case, we rely on our second CRD, called S3User. When we
      receive a reconcile request with the S3UserClaim missing, we try the cleanup process for that S3UserClaim and its
      related resources.

- S3 user name:
    - Uniqueness is implicitly handled by Kubernetes.
    - The user's name remains valid if the team of a namespace changes.
    - The namespace of a team may change so including the team name in the user name is not a good decision.
    - `<cluster_prefix>_\<namespace\>$<s3user_object_name>` could be a good option.

## Flow:

- A user creates/updates an S3UserClaim:

  ```yaml
  spec:
    s3ClassName: #(optional)
    readSecretName:  #(required)
    adminSecretName: #(required)
    quota: #(optional, with default value)
      size:
      objects:
  status:
    s3UserName:
    quota:
      size:
      objects:
  ```

- If the aggregated quota of S3UserClaim objects for a team is below the quota for the team AND the aggregated quota of
  objects in the namespace is below the quota of that namespace:
  Create/Update the user in Ceph
  Create/update an S3User:

  ```yaml
  spec:
    s3ClassName:
    readSecretName:
    adminSecretName:
    claimRef:
    quota:
      maxSize:
      maxObjects:
  ```

- If the S3UserClaim is deleted, the S3UserClaim will handle the cleanup. It will try to find the S3UserClaim with its
  name. If the claim doesn’t exist or exists and has the deletionTimestamp field set, the controller will go through the
  cleanup process.

## Quota Enforcement

Without an admission webhook, the quota should be checked by the controller. An issue arises here. Check the following
scenario:

- The max objects quota of an S3UserClaim is increased from 2 to 3.
- The controller adds 3 to the sum of S3User quotas (except the S3User of the current S3UserClaim) and compares the
  result
- with the hard quota
- The requested quota is accepted
- The controller updates S3User object
- The controller crashes (before updating the user in Ceph)
- A user changes the object quota of the S3UserClaim 3 to 4
- The controller starts running
- The controller repeats step 2 and this time rejects the requested quota

The final state of the cluster in the above scenario is an S3User object which is not synchronized with the Ceph cluster
status (and won't until S3UserClaim's requested quota is decreased)

To prevent similar scenarios, we should validate updates (and creation) of S3UserClaim objects and reject them if their
requested quota exceeds the allowable quota. This way, we'll have an idempotent flow. If the controller crashes in any
state, upon the next start it will move the state toward the desired state.

## Supporting Namespace Change

The PV, PVC system of Kubernetes supports changing a PVC's namespace. We're not going to support this feature the same
way as PV, PVC subsystem does.

The are multiple reasons for this. First, we prefer explicit tenant names to hashed names (unlike CSI-provisioned PV
names). Also, supporting namespace change requires handling too many if/else blocks and a complex operator.

Instead, we can use the bucket-linking solution offered by Ceph. In the future, we'll have a CRD named UserMigration.
The CRD spec would look like this:

```yaml
currentUser:

targetUser:

Buckets: [ ] #list of buckets to link from currentUser to targetUser
```

Creating an object of type UserMigration we'll make the controller:

- Create the target user
- Link the specified buckets to the target user
- Delete the current user

This CRD shouldn't be editable. It should behave like a one-time-run job.

## Supporting ReclaimPolicy

Based on what was explained in the previous section, keeping S3User objects after their corresponding S3UserClaim is
deleted may not offer much practicality. We do not currently plan to support it.

## Mocking Ceph RGW API or setting up a small Ceph cluster

It is a controversial topic whether to mock a dependency or not. For this specific use case, we've preferred setting up
a real Ceph cluster instead of mocking. The reasons include:

Maintaining a mock is not a simple task.
Using a mock may result in passing tests that should fail.
Having a real cluster can also help the development process and provides an easy and fast feedback loop for developers
