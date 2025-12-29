package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NASUserSpec defines a local NAS user backed by a Secret.
type NASUserSpec struct {
	Username          string            `json:"username"`
	PasswordSecretRef SMBShareSecretRef `json:"passwordSecretRef"`
}

type NASUserStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NASUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NASUserSpec   `json:"spec,omitempty"`
	Status NASUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NASUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NASUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NASUser{}, &NASUserList{})
}
