package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZPoolSpec defines the desired state of ZPool.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZPoolSpec struct {
	// NodeName is the target node for single-node/affinity scenarios.
	// For Phase 1 (single node) you can set this to the Kubernetes node name.
	NodeName string `json:"nodeName"`

	// PoolName is the ZFS pool name (e.g. "tank").
	PoolName string `json:"poolName"`

	// Vdevs describes the vdev configuration.
	Vdevs []ZPoolVdevSpec `json:"vdevs"`
}

type ZPoolVdevSpec struct {
	// Type is one of mirror/raidz1/raidz2/stripe/log/cache/spare.
	Type string `json:"type"`
	// Devices are device paths (prefer /dev/disk/by-id or /dev/disk/by-path).
	Devices []string `json:"devices"`
}

type ZPoolStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZPoolSpec   `json:"spec,omitempty"`
	Status ZPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZPool{}, &ZPoolList{})
}
