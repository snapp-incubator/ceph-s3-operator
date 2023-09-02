package s3bucket

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/opdev/subreconciler"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Cleanup cleans up the provisioned resources for the s3UserClaim object
func (r *Reconciler) Cleanup(ctx context.Context) (ctrl.Result, error) {
	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.removeBucket,
	}
	for _, subrec := range subrecs {
		result, err := subrec(ctx)
		if subreconciler.ShouldHaltOrRequeue(result, err) {
			return subreconciler.Evaluate(result, err)
		}
	}

	return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}

func (r *Reconciler) removeBucket(ctx context.Context) (*ctrl.Result, error) {
	err := r.s3Agent.deleteBucket(r.s3BucketName)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				r.logger.Error(err, "No such bucket")
				return subreconciler.ContinueReconciling()
			}
		}
		r.logger.Error(err, "failed to remove the bucket")
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}
