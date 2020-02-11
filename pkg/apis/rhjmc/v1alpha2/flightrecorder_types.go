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
}

// FlightRecorderStatus defines the observed state of FlightRecorder
// +k8s:openapi-gen=true
type FlightRecorderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Listing of events available in the target JVM
	// +listType=set
	Events []EventInfo `json:"events"`
	// TODO Can we do this with labels/selectors instead?
	// TODO Need to potentially figure out how to manage this across both services and pods in future
	// Reference to the pod/service that this object controls JFR for
	Target *corev1.ObjectReference `json:"target"`
	// JMX port for target JVM
	Port int32 `json:"port"`
	// Recordings that match this selector belong to this FlightRecorder
	RecordingSelector *metav1.LabelSelector `json:"recordingSelector"`
}

const RecordingLabel = "rhjmc.redhat.com/flightrecorder"

type EventInfo struct { // TODO if this becomes too much to store in each object, consider making JFREvent resource
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// +listType=atomic
	Category []string                    `json:"category"`
	Options  map[string]OptionDescriptor `json:"options"`
}

type OptionDescriptor struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	DefaultValue string `json:"defaultValue"`
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
