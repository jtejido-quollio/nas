package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func (in *ZDataset) DeepCopyInto(out *ZDataset) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ZDataset) DeepCopy() *ZDataset {
	if in == nil {
		return nil
	}
	out := new(ZDataset)
	in.DeepCopyInto(out)
	return out
}

func (in *ZDataset) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ZDatasetList) DeepCopyInto(out *ZDatasetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ZDataset, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *ZDatasetList) DeepCopy() *ZDatasetList {
	if in == nil {
		return nil
	}
	out := new(ZDatasetList)
	in.DeepCopyInto(out)
	return out
}

func (in *ZDatasetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&ZDataset{}, &ZDatasetList{})
}
