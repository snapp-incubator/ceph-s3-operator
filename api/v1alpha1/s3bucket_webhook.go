/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"

	"github.com/snapp-incubator/s3-operator/pkg/consts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var s3bucketlog = logf.Log.WithName("s3bucket-resource")

func (r *S3Bucket) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-s3-snappcloud-io-v1alpha1-s3bucket,mutating=false,failurePolicy=fail,sideEffects=None,groups=s3.snappcloud.io,resources=s3buckets,verbs=create;update,versions=v1alpha1,name=vs3bucket.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &S3Bucket{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *S3Bucket) ValidateCreate() error {
	s3bucketlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *S3Bucket) ValidateUpdate(old runtime.Object) error {
	s3bucketlog.Info("validate update", "name", r.Name)

	allErrs := field.ErrorList{}

	// Err if s3UserClass is changed
	oldS3Bucket, ok := old.(*S3Bucket)
	if !ok {
		s3bucketlog.Info("invalid object passed as old s3Bucket", "type", old.GetObjectKind())
		return fmt.Errorf(internalErrorMessage)
	}
	if r.Spec.S3UserRef != oldS3Bucket.Spec.S3UserRef {
		allErrs = append(
			allErrs,
			field.Forbidden(field.NewPath("spec").Child("s3UserRef"), consts.S3UserRefImmutableErrMessage),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(r.GroupVersionKind().GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *S3Bucket) ValidateDelete() error {
	s3bucketlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
