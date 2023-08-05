package s3userclaim

// This package contains the controller for provisioning and cleaning Ceph users.
//
// controller.go just sets up the controller with the manager and ensures proper tracking of related resources.
//
// handler.go is the entrypoint for the reconciliation logic. It decides to provision the required resources or to
// clean up the already provisioned resources. The key for this decision is the S3UserClaim being reconciled.
//
// If the S3UserClaim has deletionTimestamp set or if it doesn't exist at all, the controller will try to clean up.
// Otherwise, the controller will provision the required resources.

// Overall provisioning flow:
//
// 1. Create the user in Ceph
// 2. Set quota for the Ceph user
// 3. Create subuser with read access in Ceph
// 4. Create two secrets containing S3 keys for the admin and readonly users
// 5. Create an S3User object
// 6. Update the status of the S3UserClaim
// 7. Add a cleanup finalizer to the S3UserClaim

// Overall cleanup flow:
//
// 1. remove the Ceph User
// 2. remove the S3User
// 3. remove the cleanup finalizer from S3UserClaim if the object still exists
