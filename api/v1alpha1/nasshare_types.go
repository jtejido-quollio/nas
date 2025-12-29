package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NASNFSExport defines kernel NFS export options.
type NASNFSExport struct {
	Clients []string `json:"clients,omitempty"`
	Options string   `json:"options,omitempty"`
}

type NASSharePrincipalSelector struct {
	Users  []string `json:"users,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

type NASSharePermissions struct {
	Allow    NASSharePrincipalSelector `json:"allow,omitempty"`
	ReadOnly NASSharePrincipalSelector `json:"readOnly,omitempty"`
}

// NASShareSpec defines an abstract share across SMB/NFS.
type NASShareSpec struct {
	Protocol    string         `json:"protocol"`
	DatasetName string         `json:"datasetName"`
	PVCName     string         `json:"pvcName,omitempty"`
	MountPath   string         `json:"mountPath"`
	ShareName   string         `json:"shareName"`
	DirectoryRef string        `json:"directoryRef,omitempty"`
	ReadOnly    bool           `json:"readOnly,omitempty"`
	ServiceType string         `json:"serviceType,omitempty"`
	NodePort    int32          `json:"nodePort,omitempty"`
	Permissions *NASSharePermissions `json:"permissions,omitempty"`
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

func (in *NASNFSExport) DeepCopyInto(out *NASNFSExport) {
	*out = *in
	if in.Clients != nil {
		out.Clients = make([]string, len(in.Clients))
		copy(out.Clients, in.Clients)
	}
}

func (in *NASNFSExport) DeepCopy() *NASNFSExport {
	if in == nil {
		return nil
	}
	out := new(NASNFSExport)
	in.DeepCopyInto(out)
	return out
}

func (in *NASSharePrincipalSelector) DeepCopyInto(out *NASSharePrincipalSelector) {
	*out = *in
	if in.Users != nil {
		out.Users = make([]string, len(in.Users))
		copy(out.Users, in.Users)
	}
	if in.Groups != nil {
		out.Groups = make([]string, len(in.Groups))
		copy(out.Groups, in.Groups)
	}
}

func (in *NASSharePrincipalSelector) DeepCopy() *NASSharePrincipalSelector {
	if in == nil {
		return nil
	}
	out := new(NASSharePrincipalSelector)
	in.DeepCopyInto(out)
	return out
}

func (in *NASSharePermissions) DeepCopyInto(out *NASSharePermissions) {
	*out = *in
	in.Allow.DeepCopyInto(&out.Allow)
	in.ReadOnly.DeepCopyInto(&out.ReadOnly)
}

func (in *NASSharePermissions) DeepCopy() *NASSharePermissions {
	if in == nil {
		return nil
	}
	out := new(NASSharePermissions)
	in.DeepCopyInto(out)
	return out
}

func (in *NASShareSpec) DeepCopyInto(out *NASShareSpec) {
	*out = *in
	if in.Permissions != nil {
		out.Permissions = new(NASSharePermissions)
		in.Permissions.DeepCopyInto(out.Permissions)
	}
	if in.Options != nil {
		out.Options = make(map[string]any, len(in.Options))
		for k, v := range in.Options {
			out.Options[k] = v
		}
	}
	if in.NFS != nil {
		out.NFS = new(NASNFSExport)
		in.NFS.DeepCopyInto(out.NFS)
	}
}

func (in *NASShareSpec) DeepCopy() *NASShareSpec {
	if in == nil {
		return nil
	}
	out := new(NASShareSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NASShareStatus) DeepCopyInto(out *NASShareStatus) { *out = *in }

func (in *NASShareStatus) DeepCopy() *NASShareStatus {
	if in == nil {
		return nil
	}
	out := new(NASShareStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *NASShare) DeepCopyInto(out *NASShare) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NASShare) DeepCopy() *NASShare {
	if in == nil {
		return nil
	}
	out := new(NASShare)
	in.DeepCopyInto(out)
	return out
}

func (in *NASShare) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NASShareList) DeepCopyInto(out *NASShareList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NASShare, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NASShareList) DeepCopy() *NASShareList {
	if in == nil {
		return nil
	}
	out := new(NASShareList)
	in.DeepCopyInto(out)
	return out
}

func (in *NASShareList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&NASShare{}, &NASShareList{})
}
