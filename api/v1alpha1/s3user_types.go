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

// S3UserSpec defines the desired state of S3User
type S3UserSpec struct {
	Quota UserQuota `json:"quota,omitempty"`
}

// S3UserStatus defines the observed state of S3User
type S3UserStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// S3User is the Schema for the s3users API
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
