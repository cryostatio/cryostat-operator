package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RecordingSpec defines the desired state of Recording
// +k8s:openapi-gen=true
type RecordingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	// TODO Full validation marker list: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Name of the recording to be created.
	Name string `json:"name"`
	// A list of event options to use when creating the recording.
	// These are used to enable and fine-tune individual events.
	// Examples: "jdk.ExecutionSample:enabled=true", "jdk.ExecutionSample:period=200ms"
	// +listType=set
	EventOptions []string `json:"eventOptions"` // TODO Maybe replace with more specific type (e.g. "typeID, option, value" tuples)
	// The requested total duration of the recording, a zero value will record indefinitely.
	Duration metav1.Duration `json:"duration"`
	// Desired state of the recording. If omitted, RUNNING will be assumed.
	// +kubebuilder:validation:Enum=RUNNING;STOPPED
	// +optional
	State RecordingState `json:"state,omitempty"`
	// Whether this recording should be saved to persistent storage. If true, the JFR file will be retained until
	// this object is deleted. If false, the JFR file will be deleted when its corresponding JVM exits.
	Archive bool `json:"archive"`
}

// RecordingState describes the current state of the recording according
// to JFR
type RecordingState string // FIXME From client/command_types.go

const (
	// RecordingStateCreated means the recording has been accepted, but
	// has not started yet.
	RecordingStateCreated RecordingState = "CREATED"
	// RecordingStateRunning means the recording has started and is
	// currently running.
	RecordingStateRunning RecordingState = "RUNNING"
	// RecordingStateStopping means that the recording is in the process
	// of finishing.
	RecordingStateStopping RecordingState = "STOPPING"
	// RecordingStateStopped means the recording has completed and the
	// JFR file is fully written.
	RecordingStateStopped RecordingState = "STOPPED"
)

// RecordingStatus defines the observed state of Recording
// +k8s:openapi-gen=true
type RecordingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Current state of the recording.
	// +kubebuilder:validation:Enum=CREATED;RUNNING;STOPPING;STOPPED
	// +optional
	State RecordingState `json:"state,omitempty"`
	// The date/time when the recording started.
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`
	// The duration of the recording specified during creation.
	// +optional
	Duration metav1.Duration `json:"duration,omitempty"` // FIXME Needed?
	// A URL to download the JFR file for the recording.
	// +optional
	DownloadURL string `json:"downloadURL,omitempty"`
	// TODO Consider adding Conditions:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Recording is the Schema for the recordings API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=recordings,scope=Namespaced
type Recording struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecordingSpec   `json:"spec,omitempty"`
	Status RecordingStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RecordingList contains a list of Recording
type RecordingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recording `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Recording{}, &RecordingList{})
}
