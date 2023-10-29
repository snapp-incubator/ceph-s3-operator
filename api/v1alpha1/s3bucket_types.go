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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// S3BucketSpec defines the desired state of S3Bucket
type S3BucketSpec struct {
	// +kubebuilder:validation:Required
	S3UserRef string `json:"s3UserRef"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=delete;retain
	// +kubebuilder:default=delete
	S3DeletionPolicy string `json:"s3DeletionPolicy,omitempty"`

	// +kubebuilder:validation:Optional
	S3SubUserBinding []SubUserBinding `json:"s3SubUserBinding,omitempty"`
}

// S3BucketStatus defines the observed state of S3Bucket
type S3BucketStatus struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Ready bool `json:"ready,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="S3USERREF",type=string,JSONPath=`.spec.s3UserRef`
// +kubebuilder:resource:shortName=s3b

// S3Bucket is the Schema for the s3buckets API
type S3Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BucketSpec   `json:"spec,omitempty"`
	Status S3BucketStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3BucketList contains a list of S3Bucket
type S3BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Bucket{}, &S3BucketList{})
}
