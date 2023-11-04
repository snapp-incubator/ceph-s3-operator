package v1alpha1

import "k8s.io/apimachinery/pkg/api/resource"

// UserQuota specifies the quota for a user in Ceph
type UserQuota struct {
	// max number of bytes the user can store
	MaxSize resource.Quantity `json:"maxSize,omitempty"`
	// max number of objects the user can store
	MaxObjects resource.Quantity `json:"maxObjects,omitempty"`
	// max number of buckets the user can create
	MaxBuckets int `json:"maxBuckets,omitempty"`
}

// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$
type SubUser string
type SubUserBinding struct {
	// name of the subuser
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// access of the subuser which can be read or write
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=read
	// +kubebuilder:validation:Enum=read;write
	Access string `json:"access,omitempty"`
}
