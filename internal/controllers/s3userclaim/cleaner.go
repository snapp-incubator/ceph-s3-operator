package s3userclaim

import (
	"context"
	goerrors "errors"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/opdev/subreconciler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	s3v1alpha1 "github.com/snapp-cab/ceph-s3-operator/api/v1alpha1"
	"github.com/snapp-cab/ceph-s3-operator/pkg/consts"
)

// Cleanup cleans up the provisioned resources for the s3UserClaim object
func (r *Reconciler) Cleanup(ctx context.Context) (ctrl.Result, error) {
	// Do the actual reconcile work
	subrecs := []subreconciler.Fn{
		r.removeCephUser,
		r.removeS3User,
		r.updateNamespaceQuotaStatusExclusive,
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

func (r *Reconciler) removeCephUser(ctx context.Context) (*ctrl.Result, error) {
	switch err := r.rgwClient.RemoveUser(ctx, admin.User{ID: r.cephUserFullId, PurgeData: pointer.Int(1)}); {
	case goerrors.Is(err, admin.ErrNoSuchUser):
		return subreconciler.ContinueReconciling()
	case err != nil:
		r.logger.Error(err, "failed to remove Ceph user")
		return subreconciler.Requeue()
	default:
		return subreconciler.ContinueReconciling()
	}
}

func (r *Reconciler) removeS3User(ctx context.Context) (*ctrl.Result, error) {
	s3User := &s3v1alpha1.S3User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.s3UserName,
			Namespace: r.s3UserClaimNamespace,
		},
	}
	switch err := r.Delete(ctx, s3User); {
	case apierrors.IsNotFound(err):
		return subreconciler.ContinueReconciling()
	case err != nil:
		r.logger.Error(err, "failed to remove S3User")
		return subreconciler.Requeue()
	default:
		return subreconciler.ContinueReconciling()
	}
}

func (r *Reconciler) removeCleanupFinalizer(ctx context.Context) (*ctrl.Result, error) {
	if r.s3UserClaim == nil {
		return subreconciler.ContinueReconciling()
	}

	if objUpdated := controllerutil.RemoveFinalizer(r.s3UserClaim, consts.S3UserClaimCleanupFinalizer); objUpdated {
		if err := r.Update(ctx, r.s3UserClaim); err != nil {
			r.logger.Error(err, "failed to update s3UserClaim")
			return subreconciler.Requeue()
		}
	}
	return subreconciler.ContinueReconciling()
}
