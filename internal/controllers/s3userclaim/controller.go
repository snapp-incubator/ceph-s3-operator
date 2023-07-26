package s3userclaim

import (
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/predicates"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	s3UserClassPredicate := predicates.NewS3ClassPredicate(r.s3UserClass)

	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3UserClaim{}, builder.WithPredicates(s3UserClassPredicate)).
		Watches(
			&source.Kind{Type: &s3v1alpha1.S3User{}},
			handler.EnqueueRequestsFromMapFunc(s3UsertoS3UserClaim)).
		Complete(r)
}

func s3UsertoS3UserClaim(object client.Object) []reconcile.Request {
	s3User, ok := object.(*s3v1alpha1.S3User)
	if !ok {
		return nil
	}

	claimRef := s3User.Spec.ClaimRef
	if claimRef == nil {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Namespace: claimRef.Namespace,
			Name:      claimRef.Name,
		}},
	}
}
