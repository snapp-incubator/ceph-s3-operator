package s3bucket

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/opdev/subreconciler"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/snapp-incubator/ceph-s3-operator/pkg/consts"
)

// Cleanup cleans up the provisioned resources for the s3Bucket object
func (r *Reconciler) Cleanup(ctx context.Context) (ctrl.Result, error) {
	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.removeOrRetainBucket,
		r.removeCleanupFinalizer,
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) removeOrRetainBucket(ctx context.Context) (*ctrl.Result, error) {
	// Clean only if deletionPolicy is on Delete mode
	if r.s3Bucket.Spec.S3DeletionPolicy == consts.DeletionPolicyRetain {
		return subreconciler.ContinueReconciling()
	}
	err := r.s3Agent.DeleteBucket(r.s3BucketName)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				r.logger.Error(err, "No such bucket")
				return subreconciler.ContinueReconciling()
			}
		}
		r.logger.Error(err, "failed to remove the bucket")
		// update bucket status with failure reason; e.g. Bucket is not empty
		r.updateBucketStatus(ctx, true, err.Error(), "unknown")
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) removeCleanupFinalizer(ctx context.Context) (*ctrl.Result, error) {
	if r.s3Bucket == nil {
		return subreconciler.ContinueReconciling()
	}

	if objUpdated := controllerutil.RemoveFinalizer(r.s3Bucket, consts.S3BucketCleanupFinalizer); objUpdated {
		if err := r.Update(ctx, r.s3Bucket); err != nil {
			r.logger.Error(err, "failed to remove finalizer from s3Bucket")
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}
