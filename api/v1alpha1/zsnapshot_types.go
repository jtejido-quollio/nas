package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ZSnapshotSpec defines the desired state of ZSnapshot.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZSnapshotSpec struct {
	PVCName           string `json:"pvcName"`
	SnapshotClassName string `json:"snapshotClassName,omitempty"`
}

type ZSnapshotStatus struct {
	Phase              string `json:"phase,omitempty"`
	Message            string `json:"message,omitempty"`
	VolumeSnapshotName string `json:"volumeSnapshotName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZSnapshotSpec   `json:"spec,omitempty"`
	Status ZSnapshotStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZSnapshot `json:"items"`
}

func (in *ZSnapshotSpec) DeepCopyInto(out *ZSnapshotSpec) { *out = *in }

func (in *ZSnapshotSpec) DeepCopy() *ZSnapshotSpec {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotStatus) DeepCopyInto(out *ZSnapshotStatus) { *out = *in }

func (in *ZSnapshotStatus) DeepCopy() *ZSnapshotStatus {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshot) DeepCopyInto(out *ZSnapshot) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ZSnapshot) DeepCopy() *ZSnapshot {
	if in == nil {
		return nil
	}
	out := new(ZSnapshot)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshot) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ZSnapshotList) DeepCopyInto(out *ZSnapshotList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ZSnapshot, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *ZSnapshotList) DeepCopy() *ZSnapshotList {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotList)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&ZSnapshot{}, &ZSnapshotList{})
}
