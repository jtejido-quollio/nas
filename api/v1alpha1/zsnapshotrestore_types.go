package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ZSnapshotRestoreSpec defines the desired state of ZSnapshotRestore.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZSnapshotRestoreSpec struct {
	// mode: "clone" (ZFS dataset clone via node-agent) OR "csi" (PVC restore from VolumeSnapshot).
	Mode string `json:"mode"`

	// clone mode
	NodeName          string `json:"nodeName,omitempty"`
	SourceSnapshot    string `json:"sourceSnapshot,omitempty"`
	TargetDataset     string `json:"targetDataset,omitempty"`
	ForceRollback     bool   `json:"forceRollback,omitempty"`
	ConfirmationToken string `json:"confirmationToken,omitempty"`

	// csi mode
	SourceVolumeSnapshot string         `json:"sourceVolumeSnapshot,omitempty"`
	TargetPVC            string         `json:"targetPVC,omitempty"`
	StorageClassName     string         `json:"storageClassName,omitempty"`
	AccessModes          []string       `json:"accessModes,omitempty"`
	Resources            map[string]any `json:"resources,omitempty"`
}

type ZSnapshotRestoreStatus struct {
	Phase         string `json:"phase,omitempty"`
	Message       string `json:"message,omitempty"`
	ResultDataset string `json:"resultDataset,omitempty"`
	ResultPVC     string `json:"resultPVC,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZSnapshotRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZSnapshotRestoreSpec   `json:"spec,omitempty"`
	Status ZSnapshotRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZSnapshotRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZSnapshotRestore `json:"items"`
}

func (in *ZSnapshotRestoreSpec) DeepCopyInto(out *ZSnapshotRestoreSpec) {
	*out = *in
	if in.AccessModes != nil {
		out.AccessModes = make([]string, len(in.AccessModes))
		copy(out.AccessModes, in.AccessModes)
	}
	if in.Resources != nil {
		out.Resources = runtime.DeepCopyJSON(in.Resources)
	}
}

func (in *ZSnapshotRestoreSpec) DeepCopy() *ZSnapshotRestoreSpec {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotRestoreSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotRestoreStatus) DeepCopyInto(out *ZSnapshotRestoreStatus) { *out = *in }

func (in *ZSnapshotRestoreStatus) DeepCopy() *ZSnapshotRestoreStatus {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotRestoreStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotRestore) DeepCopyInto(out *ZSnapshotRestore) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ZSnapshotRestore) DeepCopy() *ZSnapshotRestore {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotRestore)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotRestore) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ZSnapshotRestoreList) DeepCopyInto(out *ZSnapshotRestoreList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ZSnapshotRestore, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *ZSnapshotRestoreList) DeepCopy() *ZSnapshotRestoreList {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotRestoreList)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotRestoreList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&ZSnapshotRestore{}, &ZSnapshotRestoreList{})
}
