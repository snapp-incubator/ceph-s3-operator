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

type SubUserBinding struct {
	// name of the subuser
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// access of the subuser which can be read, write or full
	// +kubebuilder:default=read
	// +kubebuilder:validation:Enum=read;write;full
	Access string `json:"access,omitempty"`
}
