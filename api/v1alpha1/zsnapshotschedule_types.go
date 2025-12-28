package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func init() {
	SchemeBuilder.Register(&ZSnapshotSchedule{}, &ZSnapshotScheduleList{})
}
