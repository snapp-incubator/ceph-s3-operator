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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// S3UserSpec defines the desired state of S3User
type S3UserSpec struct {
	// +kubebuilder:validation:Optional
	S3UserClass string `json:"s3UserClass,omitempty"`

	// +kubebuilder:validation:Optional
	Quota *UserQuota `json:"quota,omitempty"`

	// +kubebuilder:validation:Optional
	ClaimRef *v1.ObjectReference `json:"claimRef,omitempty"`
}

// S3UserStatus defines the observed state of S3User
type S3UserStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="S3USERCLASS",type=string,JSONPath=`.spec.s3UserClass`
// +kubebuilder:printcolumn:name="CLAIM NS",type=string,JSONPath=`.spec.claimRef.namespace`
// +kubebuilder:printcolumn:name="CLAIM NAME",type=string,JSONPath=`.spec.claimRef.name`
// +kubebuilder:printcolumn:name="MAX OBJECTS",type=string,JSONPath=`.spec.quota.maxObjects`
// +kubebuilder:printcolumn:name="MAX SIZE",type=string,JSONPath=`.spec.quota.maxSize`
// +kubebuilder:printcolumn:name="MAX BUCKETS",type=string,JSONPath=`.spec.quota.maxBuckets`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// S3 User is created by the S3 User Claim instance. It's not applicable for the operator user.
type S3User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3UserSpec   `json:"spec,omitempty"`
	Status S3UserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3UserList contains a list of S3User
type S3UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3User `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3User{}, &S3UserList{})
}

func (suc *S3User) GetS3UserClass() string {
	return suc.Spec.S3UserClass
}
