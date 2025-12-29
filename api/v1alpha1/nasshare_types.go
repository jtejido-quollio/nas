package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NASNFSExport defines kernel NFS export options.
type NASNFSExport struct {
	Clients []string `json:"clients,omitempty"`
	Options string   `json:"options,omitempty"`
}

// NASShareSpec defines an abstract share across SMB/NFS.
type NASShareSpec struct {
	Protocol    string         `json:"protocol"`
	DatasetName string         `json:"datasetName"`
	PVCName     string         `json:"pvcName,omitempty"`
	MountPath   string         `json:"mountPath"`
	ShareName   string         `json:"shareName"`
	ReadOnly    bool           `json:"readOnly,omitempty"`
	ServiceType string         `json:"serviceType,omitempty"`
	NodePort    int32          `json:"nodePort,omitempty"`
	Users       []string       `json:"users,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
	NFS         *NASNFSExport  `json:"nfs,omitempty"`
}

type NASShareStatus struct {
	Phase    string `json:"phase,omitempty"`
	Message  string `json:"message,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NASShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NASShareSpec   `json:"spec,omitempty"`
	Status NASShareStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NASShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NASShare `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NASShare{}, &NASShareList{})
}
