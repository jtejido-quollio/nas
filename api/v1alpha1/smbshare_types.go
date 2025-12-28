package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SMBShareSpec defines the desired state of SMBShare.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type SMBShareSpec struct {
	NodeName    string         `json:"nodeName"`
	DatasetName string         `json:"datasetName"`
	PVCName     string         `json:"pvcName,omitempty"`
	MountPath   string         `json:"mountPath"`
	ShareName   string         `json:"shareName"`
	ReadOnly    bool           `json:"readOnly,omitempty"`
	ServiceType string         `json:"serviceType"`
	NodePort    int32          `json:"nodePort,omitempty"`
	Users       []SMBShareUser `json:"users,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
}

type SMBShareUser struct {
	Username          string            `json:"username"`
	PasswordSecretRef SMBShareSecretRef `json:"passwordSecretRef"`
}

type SMBShareSecretRef struct {
	Name string `json:"name"`
}

type SMBShareStatus struct {
	Phase    string `json:"phase,omitempty"`
	Message  string `json:"message,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type SMBShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SMBShareSpec   `json:"spec,omitempty"`
	Status SMBShareStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SMBShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SMBShare `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SMBShare{}, &SMBShareList{})
}
