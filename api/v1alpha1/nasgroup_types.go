package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func (in *NASGroupSpec) DeepCopyInto(out *NASGroupSpec) {
	*out = *in
	if in.Members != nil {
		out.Members = make([]string, len(in.Members))
		copy(out.Members, in.Members)
	}
}

func (in *NASGroupSpec) DeepCopy() *NASGroupSpec {
	if in == nil {
		return nil
	}
	out := new(NASGroupSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NASGroupStatus) DeepCopyInto(out *NASGroupStatus) { *out = *in }

func (in *NASGroupStatus) DeepCopy() *NASGroupStatus {
	if in == nil {
		return nil
	}
	out := new(NASGroupStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *NASGroup) DeepCopyInto(out *NASGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NASGroup) DeepCopy() *NASGroup {
	if in == nil {
		return nil
	}
	out := new(NASGroup)
	in.DeepCopyInto(out)
	return out
}

func (in *NASGroup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NASGroupList) DeepCopyInto(out *NASGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NASGroup, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NASGroupList) DeepCopy() *NASGroupList {
	if in == nil {
		return nil
	}
	out := new(NASGroupList)
	in.DeepCopyInto(out)
	return out
}

func (in *NASGroupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&NASGroup{}, &NASGroupList{})
}
