package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ZPoolSpec defines the desired state of ZPool.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZPoolSpec struct {
	// NodeName is the target node for single-node/affinity scenarios.
	// For Phase 1 (single node) you can set this to the Kubernetes node name.
	NodeName string `json:"nodeName"`

	// PoolName is the ZFS pool name (e.g. "tank").
	PoolName string `json:"poolName"`

	// Vdevs describes the vdev configuration.
	Vdevs []ZPoolVdevSpec `json:"vdevs"`
}

type ZPoolVdevSpec struct {
	// Type is one of mirror/raidz1/raidz2/stripe/log/cache/spare.
	Type string `json:"type"`
	// Devices are device paths (prefer /dev/disk/by-id or /dev/disk/by-path).
	Devices []string `json:"devices"`
}

type ZPoolStatus struct {
	Phase   string      `json:"phase,omitempty"`
	Message string      `json:"message,omitempty"`
	Usage   *ZPoolUsage `json:"usage,omitempty"`
}

type ZPoolUsage struct {
	Total     int64 `json:"total,omitempty"`
	Used      int64 `json:"used,omitempty"`
	Available int64 `json:"available,omitempty"`
	RawTotal  int64 `json:"rawTotal,omitempty"`
}

func (in *ZPoolUsage) DeepCopyInto(out *ZPoolUsage) {
	*out = *in
}

func (in *ZPoolUsage) DeepCopy() *ZPoolUsage {
	if in == nil {
		return nil
	}
	out := new(ZPoolUsage)
	in.DeepCopyInto(out)
	return out
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZPoolSpec   `json:"spec,omitempty"`
	Status ZPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZPool `json:"items"`
}

func (in *ZPoolVdevSpec) DeepCopyInto(out *ZPoolVdevSpec) {
	*out = *in
	if in.Devices != nil {
		out.Devices = make([]string, len(in.Devices))
		copy(out.Devices, in.Devices)
	}
}

func (in *ZPoolVdevSpec) DeepCopy() *ZPoolVdevSpec {
	if in == nil {
		return nil
	}
	out := new(ZPoolVdevSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ZPoolSpec) DeepCopyInto(out *ZPoolSpec) {
	*out = *in
	if in.Vdevs != nil {
		out.Vdevs = make([]ZPoolVdevSpec, len(in.Vdevs))
		for i := range in.Vdevs {
			in.Vdevs[i].DeepCopyInto(&out.Vdevs[i])
		}
	}
}

func (in *ZPoolSpec) DeepCopy() *ZPoolSpec {
	if in == nil {
		return nil
	}
	out := new(ZPoolSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ZPool) DeepCopyInto(out *ZPool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status.Phase = in.Status.Phase
	out.Status.Message = in.Status.Message
	if in.Status.Usage != nil {
		out.Status.Usage = in.Status.Usage.DeepCopy()
	}
}

func (in *ZPool) DeepCopy() *ZPool {
	if in == nil {
		return nil
	}
	out := new(ZPool)
	in.DeepCopyInto(out)
	return out
}

func (in *ZPool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ZPoolList) DeepCopyInto(out *ZPoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ZPool, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *ZPoolList) DeepCopy() *ZPoolList {
	if in == nil {
		return nil
	}
	out := new(ZPoolList)
	in.DeepCopyInto(out)
	return out
}

func (in *ZPoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ZDatasetSpec) DeepCopyInto(out *ZDatasetSpec) {
	*out = *in
	if in.Properties != nil {
		out.Properties = make(map[string]string, len(in.Properties))
		for k, v := range in.Properties {
			out.Properties[k] = v
		}
	}
}

func (in *ZDatasetSpec) DeepCopy() *ZDatasetSpec {
	if in == nil {
		return nil
	}
	out := new(ZDatasetSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ZDatasetStatus) DeepCopyInto(out *ZDatasetStatus) {
	*out = *in
}

func (in *ZDatasetStatus) DeepCopy() *ZDatasetStatus {
	if in == nil {
		return nil
	}
	out := new(ZDatasetStatus)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&ZPool{}, &ZPoolList{})
}
