package v1alpha1

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

	// Specifies whether a JFR recording should be started or stopped
	RecordingActive bool `json:"recordingActive"`
}

// FlightRecorderStatus defines the observed state of FlightRecorder
// +k8s:openapi-gen=true
type FlightRecorderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Reference to the pod/service that this object controls JFR for
	Target *corev1.ObjectReference `json:"target"`
	// Whether the pod/service is currently recording
	RecordingActive bool `json:"recordingActive"` // TODO Maybe replace with a Status with STOPPED, STARTED, ERROR, etc.
	// Lists all recordings for the pod/service that may be downloaded
	// +listType=set
	Recordings []string `json:"recordings"` // TODO Name, URL, composite type with a route?
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
