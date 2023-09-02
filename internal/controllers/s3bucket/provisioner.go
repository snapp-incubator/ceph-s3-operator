package s3bucket

import (
	"context"
	"strings"

	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/opdev/subreconciler"
	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Provision provisions the required resources for the s3UserClaim object
func (r *Reconciler) Provision(ctx context.Context) (ctrl.Result, error) {
	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.ensureBucket,
		r.updateBucketStatus,
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
	err := r.s3Agent.createBucket(r.s3Bucket.GetName())
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeBucketAlreadyExists:
				return subreconciler.ContinueReconciling()
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				return subreconciler.ContinueReconciling()
			}
		}
		r.logger.Error(err, "failed to create the bucket")
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) updateBucketStatus(ctx context.Context) (*ctrl.Result, error) {
	status := s3v1alpha1.S3BucketStatus{
		Ready: true,
	}

	if !apiequality.Semantic.DeepEqual(r.s3Bucket.Status, status) {
		r.s3Bucket.Status = status
		if err := r.Status().Update(ctx, r.s3Bucket); err != nil {
			if strings.Contains(err.Error(), genericregistry.OptimisticLockErrorMsg) {
				r.logger.Info("re-queuing item due to optimistic locking on resource", "error", err.Error())
			} else {
				r.logger.Error(err, "failed to update s3 bucket")
			}
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}
