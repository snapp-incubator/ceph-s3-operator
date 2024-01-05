package s3bucket

import (
	"context"

	"github.com/opdev/subreconciler"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	s3v1alpha1 "github.com/snapp-cab/ceph-s3-operator/api/v1alpha1"
	"github.com/snapp-cab/ceph-s3-operator/pkg/consts"
)

// Provision provisions the required resources for the s3UserClaim object
func (r *Reconciler) Provision(ctx context.Context) (ctrl.Result, error) {
	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.ensureBucket,
		r.ensureBucketPolicy,
		r.updateBucketStatusSuccess,
		r.addCleanupFinalizer,
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) ensureBucket(ctx context.Context) (*ctrl.Result, error) {
	err := r.s3Agent.CreateBucket(r.s3Bucket.GetName())
	if err != nil {
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) ensureBucketPolicy(ctx context.Context) (*ctrl.Result, error) {
	var err error
	r.bucketPolicy, err = r.s3Agent.SetBucketPolicy(r.subuserAccessMap,
		r.cephTenant, r.s3UserRef, r.s3BucketName)
	if err != nil {
		r.logger.Error(err, "failed to set the bucket policy")
		r.updateBucketStatus(ctx, true, err.Error(), r.bucketPolicy)
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) updateBucketStatusSuccess(ctx context.Context) (*ctrl.Result, error) {
	return r.updateBucketStatus(ctx, true, "", r.bucketPolicy)
}
func (r *Reconciler) updateBucketStatus(ctx context.Context,
	created bool, reason string, policy string) (*ctrl.Result, error) {
	status := s3v1alpha1.S3BucketStatus{
		Created: created,
		Reason:  reason,
		Policy:  policy,
	}

	if !apiequality.Semantic.DeepEqual(r.s3Bucket.Status, status) {
		r.s3Bucket.Status = status
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Status().Update(ctx, r.s3Bucket)
		}); err != nil {
			r.logger.Error(err, "failed to update s3 bucket")
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) addCleanupFinalizer(ctx context.Context) (*ctrl.Result, error) {
	if objUpdated := controllerutil.AddFinalizer(r.s3Bucket, consts.S3BucketCleanupFinalizer); objUpdated {
		if err := r.Update(ctx, r.s3Bucket); err != nil {
			r.logger.Error(err, "failed to add finalizer to the s3Bucket")
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}
