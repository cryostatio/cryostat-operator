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
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// JMX port for target JVM
	Port int32 `json:"port"`
	// Requests to create new flight recordings
	// +listType=set
	RecordingRequests []RecordingRequest `json:"recordingRequests"`
}

// FlightRecorderStatus defines the observed state of FlightRecorder
// +k8s:openapi-gen=true
type FlightRecorderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Reference to the pod/service that this object controls JFR for
	Target *corev1.ObjectReference `json:"target"`
	// Lists all recordings for the pod/service that may be downloaded
	// +listType=set
	Recordings []RecordingInfo `json:"recordings"`
}

// RecordingRequest allows the user to specify new recordings for
// Container JFR to create
type RecordingRequest struct {
	// Name of the recording to be created
	Name string `json:"name"`
	// A list of event options to use when creating the recording.
	// These are used to enable and fine-tune individual events.
	// Examples: "jdk.ExecutionSample:enabled=true", "jdk.ExecutionSample:period=200ms"
	// +listType=set
	EventOptions []string `json:"eventOptions"`
	// The requested total duration of the recording
	Duration metav1.Duration `json:"duration"`
}

// RecordingInfo contains the status of recordings that have already
// been created by Container JFR
type RecordingInfo struct {
	// Name of the created recording
	Name string `json:"name"`
	// Whether the recording is currently running
	Active bool `json:"active"`
	// The date/time when the recording started
	StartTime metav1.Time `json:"startTime"`
	// The duration of the recording specified during creation
	Duration metav1.Duration `json:"duration"`
	// A URL to download the JFR file for the recording
	DownloadURL string `json:"downloadUrl,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FlightRecorder is the Schema for the flightrecorders API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=flightrecorders,scope=Namespaced
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
