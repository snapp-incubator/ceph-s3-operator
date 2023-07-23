package s3userclaim

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
	"github.com/snapp-incubator/s3-operator/internal/predicates"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	s3UserClassPredicate := predicates.NewS3ClassPredicate(r.s3UserClass)

	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3User{}, builder.WithPredicates(s3UserClassPredicate)).
		Owns(&s3v1alpha1.S3User{}).
		Complete(r)
}
