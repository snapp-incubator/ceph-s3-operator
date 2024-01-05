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
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/snapp-cab/ceph-s3-operator/pkg/consts"
)

// log is for logging in this package.
var s3bucketlog = logf.Log.WithName("s3bucket-resource")

func (sb *S3Bucket) SetupWebhookWithManager(mgr ctrl.Manager) error {
	runtimeClient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(sb).
		Complete()
}

//+kubebuilder:webhook:path=/validate-s3-snappcloud-io-v1alpha1-s3bucket,mutating=false,failurePolicy=fail,sideEffects=None,groups=s3.snappcloud.io,resources=s3buckets,verbs=create;update,versions=v1alpha1,name=vs3bucket.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &S3Bucket{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (sb *S3Bucket) ValidateCreate() error {
	s3bucketlog.Info("validate create", "name", sb.Name)
	allErrs := field.ErrorList{}

	ctx, cancel := context.WithTimeout(context.Background(), ValidationTimeout)
	defer cancel()

	// S3UserRef Validator: S3UserRef must be previously defeind as S3UserClaim CR.
	s3UserRef := sb.Spec.S3UserRef
	s3UserClaim := &S3UserClaim{}
	err := runtimeClient.Get(ctx, types.NamespacedName{Name: s3UserRef, Namespace: sb.Namespace}, s3UserClaim)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Forbidden(field.NewPath("spec").Child("s3UserRef"), consts.S3UserRefNotFoundErrMessage),
		)
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(sb.GroupVersionKind().GroupKind(), sb.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (sb *S3Bucket) ValidateUpdate(old runtime.Object) error {
	s3bucketlog.Info("validate update", "name", sb.Name)

	allErrs := field.ErrorList{}

	// S3UserRef Validator: Err if s3userref is changed.
	oldS3Bucket, ok := old.(*S3Bucket)
	if !ok {
		s3bucketlog.Info("invalid object passed as old s3Bucket", "type", old.GetObjectKind())
		return fmt.Errorf(internalErrorMessage)
	}
	if sb.Spec.S3UserRef != oldS3Bucket.Spec.S3UserRef {
		allErrs = append(
			allErrs,
			field.Forbidden(field.NewPath("spec").Child("s3UserRef"), consts.S3UserRefImmutableErrMessage),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(sb.GroupVersionKind().GroupKind(), sb.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (sb *S3Bucket) ValidateDelete() error {
	s3bucketlog.Info("validate delete", "name", sb.Name)

	return nil
}
