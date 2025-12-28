package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZDatasetSpec defines the desired state of ZDataset.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZDatasetSpec struct {
	NodeName    string            `json:"nodeName"`
	DatasetName string            `json:"datasetName"`
	Properties  map[string]string `json:"properties"`
}

type ZDatasetStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZDataset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZDatasetSpec   `json:"spec,omitempty"`
	Status ZDatasetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZDatasetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZDataset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZDataset{}, &ZDatasetList{})
}
