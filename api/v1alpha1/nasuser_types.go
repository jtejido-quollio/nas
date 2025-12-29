package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NASUserSpec defines a local NAS user backed by a Secret.
type NASUserSpec struct {
	Username          string            `json:"username"`
	PasswordSecretRef PasswordSecretRef `json:"passwordSecretRef"`
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

func (in *NASUserSpec) DeepCopyInto(out *NASUserSpec) { *out = *in }

func (in *NASUserSpec) DeepCopy() *NASUserSpec {
	if in == nil {
		return nil
	}
	out := new(NASUserSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NASUserStatus) DeepCopyInto(out *NASUserStatus) { *out = *in }

func (in *NASUserStatus) DeepCopy() *NASUserStatus {
	if in == nil {
		return nil
	}
	out := new(NASUserStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *NASUser) DeepCopyInto(out *NASUser) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NASUser) DeepCopy() *NASUser {
	if in == nil {
		return nil
	}
	out := new(NASUser)
	in.DeepCopyInto(out)
	return out
}

func (in *NASUser) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NASUserList) DeepCopyInto(out *NASUserList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NASUser, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NASUserList) DeepCopy() *NASUserList {
	if in == nil {
		return nil
	}
	out := new(NASUserList)
	in.DeepCopyInto(out)
	return out
}

func (in *NASUserList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&NASUser{}, &NASUserList{})
}
