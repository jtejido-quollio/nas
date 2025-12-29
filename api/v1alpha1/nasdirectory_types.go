package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NASDirectorySpec defines an identity source for SMB/NFS.
type NASDirectorySpec struct {
	// Type is one of: local, ldap, activeDirectory.
	Type string `json:"type"`

	Servers []string `json:"servers,omitempty"`
	BaseDN  string   `json:"baseDN,omitempty"`

	Bind *NASDirectoryBind `json:"bind,omitempty"`
	TLS  *NASDirectoryTLS  `json:"tls,omitempty"`

	IDMapping      *NASDirectoryIDMapping      `json:"idMapping,omitempty"`
	GroupResolution *NASDirectoryGroupResolution `json:"groupResolution,omitempty"`

	// Local config is used when type=local.
	Local *NASDirectoryLocal `json:"local,omitempty"`
}

type NASDirectoryBind struct {
	Username  string            `json:"username,omitempty"`
	SecretRef *PasswordSecretRef `json:"secretRef,omitempty"`
}

type NASDirectoryTLS struct {
	CABundleSecretRef *SecretRef `json:"caBundleSecretRef,omitempty"`
	Verify            bool       `json:"verify,omitempty"`
}

type NASDirectoryIDMapping struct {
	Strategy     string `json:"strategy,omitempty"`
	UIDAttribute string `json:"uidAttribute,omitempty"`
	GIDAttribute string `json:"gidAttribute,omitempty"`
	UIDStart     int64  `json:"uidStart,omitempty"`
	GIDStart     int64  `json:"gidStart,omitempty"`
}

type NASDirectoryGroupResolution struct {
	NestedGroups bool `json:"nestedGroups,omitempty"`
}

type NASDirectoryLocal struct {
	UIDStart int64  `json:"uidStart,omitempty"`
	GIDStart int64  `json:"gidStart,omitempty"`
	Strategy string `json:"strategy,omitempty"`
}

type NASDirectoryStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NASDirectory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NASDirectorySpec   `json:"spec,omitempty"`
	Status NASDirectoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NASDirectoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NASDirectory `json:"items"`
}

func (in *NASDirectoryBind) DeepCopyInto(out *NASDirectoryBind) {
	*out = *in
	if in.SecretRef != nil {
		out.SecretRef = &PasswordSecretRef{Name: in.SecretRef.Name}
	}
}

func (in *NASDirectoryBind) DeepCopy() *NASDirectoryBind {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryBind)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectoryTLS) DeepCopyInto(out *NASDirectoryTLS) {
	*out = *in
	if in.CABundleSecretRef != nil {
		out.CABundleSecretRef = &SecretRef{Name: in.CABundleSecretRef.Name}
	}
}

func (in *NASDirectoryTLS) DeepCopy() *NASDirectoryTLS {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryTLS)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectoryIDMapping) DeepCopyInto(out *NASDirectoryIDMapping) { *out = *in }

func (in *NASDirectoryIDMapping) DeepCopy() *NASDirectoryIDMapping {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryIDMapping)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectoryGroupResolution) DeepCopyInto(out *NASDirectoryGroupResolution) { *out = *in }

func (in *NASDirectoryGroupResolution) DeepCopy() *NASDirectoryGroupResolution {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryGroupResolution)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectoryLocal) DeepCopyInto(out *NASDirectoryLocal) { *out = *in }

func (in *NASDirectoryLocal) DeepCopy() *NASDirectoryLocal {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryLocal)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectorySpec) DeepCopyInto(out *NASDirectorySpec) {
	*out = *in
	if in.Servers != nil {
		out.Servers = make([]string, len(in.Servers))
		copy(out.Servers, in.Servers)
	}
	if in.Bind != nil {
		out.Bind = new(NASDirectoryBind)
		in.Bind.DeepCopyInto(out.Bind)
	}
	if in.TLS != nil {
		out.TLS = new(NASDirectoryTLS)
		in.TLS.DeepCopyInto(out.TLS)
	}
	if in.IDMapping != nil {
		out.IDMapping = new(NASDirectoryIDMapping)
		in.IDMapping.DeepCopyInto(out.IDMapping)
	}
	if in.GroupResolution != nil {
		out.GroupResolution = new(NASDirectoryGroupResolution)
		in.GroupResolution.DeepCopyInto(out.GroupResolution)
	}
	if in.Local != nil {
		out.Local = new(NASDirectoryLocal)
		in.Local.DeepCopyInto(out.Local)
	}
}

func (in *NASDirectorySpec) DeepCopy() *NASDirectorySpec {
	if in == nil {
		return nil
	}
	out := new(NASDirectorySpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectoryStatus) DeepCopyInto(out *NASDirectoryStatus) { *out = *in }

func (in *NASDirectoryStatus) DeepCopy() *NASDirectoryStatus {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectory) DeepCopyInto(out *NASDirectory) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NASDirectory) DeepCopy() *NASDirectory {
	if in == nil {
		return nil
	}
	out := new(NASDirectory)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectory) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NASDirectoryList) DeepCopyInto(out *NASDirectoryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NASDirectory, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NASDirectoryList) DeepCopy() *NASDirectoryList {
	if in == nil {
		return nil
	}
	out := new(NASDirectoryList)
	in.DeepCopyInto(out)
	return out
}

func (in *NASDirectoryList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&NASDirectory{}, &NASDirectoryList{})
}
