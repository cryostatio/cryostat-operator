package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FlightRecorderSpec defines the desired state of FlightRecorder
// +k8s:openapi-gen=true
type FlightRecorderSpec struct {
	// Recordings that match this selector belong to this FlightRecorder
	RecordingSelector *metav1.LabelSelector `json:"recordingSelector"`
}

// FlightRecorderStatus defines the observed state of FlightRecorder
// +k8s:openapi-gen=true
type FlightRecorderStatus struct {
	// Listing of events available in the target JVM
	// +listType=set
	Events []EventInfo `json:"events"`
	// Reference to the pod/service that this object controls JFR for
	Target *corev1.ObjectReference `json:"target"`
	// JMX port for target JVM
	// +kubebuilder:validation:Minimum=0
	Port int32 `json:"port"`
}

// RecordingLabel is the label name to be used with FlightRecorderSpec.RecordingSelector
const RecordingLabel = "rhjmc.redhat.com/flightrecorder"

// EventInfo contains metadata for a JFR event type
type EventInfo struct {
	// The ID used by JFR to uniquely identify this event type
	TypeID string `json:"typeId"`
	// Human-readable name for this type of event
	Name string `json:"name"`
	// A description detailing what this event does
	Description string `json:"description"`
	// A hierarchical category used to organize related event types
	// +listType=atomic
	Category []string `json:"category"`
	// Options that may be used to tune this event. This map is indexed
	// by the option IDs.
	Options map[string]OptionDescriptor `json:"options"`
}

// OptionDescriptor contains metadata for an option for a particular event type
type OptionDescriptor struct {
	// Human-readable name for this option
	Name string `json:"name"`
	// A description of what this option does
	Description string `json:"description"`
	// The value implicitly used when this option isn't specified
	DefaultValue string `json:"defaultValue"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FlightRecorder is the Schema for the flightrecorders API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=flightrecorders,scope=Namespaced
// +kubebuilder:storageversion
type FlightRecorder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FlightRecorderSpec   `json:"spec,omitempty"`
	Status FlightRecorderStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FlightRecorderList contains a list of FlightRecorder
type FlightRecorderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FlightRecorder `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FlightRecorder{}, &FlightRecorderList{})
}
