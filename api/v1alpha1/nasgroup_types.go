package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NASGroupSpec defines a local NAS group.
type NASGroupSpec struct {
	Members []string `json:"members,omitempty"`
}

type NASGroupStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NASGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NASGroupSpec   `json:"spec,omitempty"`
	Status NASGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NASGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NASGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NASGroup{}, &NASGroupList{})
}
