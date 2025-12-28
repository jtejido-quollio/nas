package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZSnapshotRestoreSpec defines the desired state of ZSnapshotRestore.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZSnapshotRestoreSpec struct {
	// mode: "clone" (ZFS dataset clone via node-agent) OR "csi" (PVC restore from VolumeSnapshot).
	Mode string `json:"mode"`

	// clone mode
	NodeName          string `json:"nodeName,omitempty"`
	SourceSnapshot    string `json:"sourceSnapshot,omitempty"`
	TargetDataset     string `json:"targetDataset,omitempty"`
	ForceRollback     bool   `json:"forceRollback,omitempty"`
	ConfirmationToken string `json:"confirmationToken,omitempty"`

	// csi mode
	SourceVolumeSnapshot string         `json:"sourceVolumeSnapshot,omitempty"`
	TargetPVC            string         `json:"targetPVC,omitempty"`
	StorageClassName     string         `json:"storageClassName,omitempty"`
	AccessModes          []string       `json:"accessModes,omitempty"`
	Resources            map[string]any `json:"resources,omitempty"`
}

type ZSnapshotRestoreStatus struct {
	Phase         string `json:"phase,omitempty"`
	Message       string `json:"message,omitempty"`
	ResultDataset string `json:"resultDataset,omitempty"`
	ResultPVC     string `json:"resultPVC,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZSnapshotRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZSnapshotRestoreSpec   `json:"spec,omitempty"`
	Status ZSnapshotRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZSnapshotRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZSnapshotRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZSnapshotRestore{}, &ZSnapshotRestoreList{})
}
