package s3user

import (
	ctrl "sigs.k8s.io/controller-runtime"

	s3v1alpha1 "github.com/snapp-incubator/s3-operator/api/v1alpha1"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3User{}).
		Complete(r)
}
