package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZSnapshotSpec defines the desired state of ZSnapshot.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZSnapshotSpec struct {
	PVCName           string `json:"pvcName"`
	SnapshotClassName string `json:"snapshotClassName,omitempty"`
}

type ZSnapshotStatus struct {
	Phase              string `json:"phase,omitempty"`
	Message            string `json:"message,omitempty"`
	VolumeSnapshotName string `json:"volumeSnapshotName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZSnapshotSpec   `json:"spec,omitempty"`
	Status ZSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZSnapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZSnapshot{}, &ZSnapshotList{})
}
