package s3bucket

import (
	"context"
	"strings"

	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"

	"github.com/opdev/subreconciler"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/pkg/consts"
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
	err := r.s3Agent.SetBucketPolicy(r.subUserAccessMap,
		r.cephTenant, r.s3UserRef, r.s3BucketName)
	if err != nil {
		return subreconciler.Requeue()
	}
	return subreconciler.ContinueReconciling()
}

func (r *Reconciler) updateBucketStatusSuccess(ctx context.Context) (*ctrl.Result, error) {
	return r.updateBucketStatus(ctx, true, "", r.s3Bucket.Spec.S3SubUserBinding)
}
func (r *Reconciler) updateBucketStatus(ctx context.Context,
	ready bool, reason string, s3subUserBinding []s3v1alpha1.SubUserBinding) (*ctrl.Result, error) {
	status := s3v1alpha1.S3BucketStatus{
		Ready:            ready,
		Reason:           reason,
		S3SubUserBinding: s3subUserBinding,
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

func (r *Reconciler) addCleanupFinalizer(ctx context.Context) (*ctrl.Result, error) {
	if objUpdated := controllerutil.AddFinalizer(r.s3Bucket, consts.S3BucketCleanupFinalizer); objUpdated {
		if err := r.Update(ctx, r.s3Bucket); err != nil {
			r.logger.Error(err, "failed to add finalizer to the s3Bucket")
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}
