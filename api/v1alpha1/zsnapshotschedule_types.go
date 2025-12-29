package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ZSnapshotScheduleSpec defines the desired state of ZSnapshotSchedule.
//
// NOTE: This matches the existing CRD schema in config/crd/bases.
type ZSnapshotScheduleSpec struct {
	NodeName    string                      `json:"nodeName"`
	DatasetName string                      `json:"datasetName"`
	Recursive   bool                        `json:"recursive,omitempty"`
	Schedule    string                      `json:"schedule"`
	NamePrefix  string                      `json:"namePrefix,omitempty"`
	Format      string                      `json:"format,omitempty"`
	Retention   *ZSnapshotScheduleRetention `json:"retention,omitempty"`
}

type ZSnapshotScheduleRetention struct {
	KeepLast    int64 `json:"keepLast,omitempty"`
	KeepHourly  int64 `json:"keepHourly,omitempty"`
	KeepDaily   int64 `json:"keepDaily,omitempty"`
	KeepWeekly  int64 `json:"keepWeekly,omitempty"`
	KeepMonthly int64 `json:"keepMonthly,omitempty"`
}

type ZSnapshotScheduleStatus struct {
	LastSnapshotName string `json:"lastSnapshotName,omitempty"`
	LastRunTime      string `json:"lastRunTime,omitempty"`
	NextRunTime      string `json:"nextRunTime,omitempty"`
	Message          string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ZSnapshotSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZSnapshotScheduleSpec   `json:"spec,omitempty"`
	Status ZSnapshotScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ZSnapshotScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZSnapshotSchedule `json:"items"`
}

func (in *ZSnapshotScheduleRetention) DeepCopyInto(out *ZSnapshotScheduleRetention) { *out = *in }

func (in *ZSnapshotScheduleRetention) DeepCopy() *ZSnapshotScheduleRetention {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotScheduleRetention)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotScheduleSpec) DeepCopyInto(out *ZSnapshotScheduleSpec) {
	*out = *in
	if in.Retention != nil {
		out.Retention = new(ZSnapshotScheduleRetention)
		in.Retention.DeepCopyInto(out.Retention)
	}
}

func (in *ZSnapshotScheduleSpec) DeepCopy() *ZSnapshotScheduleSpec {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotScheduleSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotScheduleStatus) DeepCopyInto(out *ZSnapshotScheduleStatus) { *out = *in }

func (in *ZSnapshotScheduleStatus) DeepCopy() *ZSnapshotScheduleStatus {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotScheduleStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotSchedule) DeepCopyInto(out *ZSnapshotSchedule) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *ZSnapshotSchedule) DeepCopy() *ZSnapshotSchedule {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotSchedule)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotSchedule) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ZSnapshotScheduleList) DeepCopyInto(out *ZSnapshotScheduleList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]ZSnapshotSchedule, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *ZSnapshotScheduleList) DeepCopy() *ZSnapshotScheduleList {
	if in == nil {
		return nil
	}
	out := new(ZSnapshotScheduleList)
	in.DeepCopyInto(out)
	return out
}

func (in *ZSnapshotScheduleList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&ZSnapshotSchedule{}, &ZSnapshotScheduleList{})
}
