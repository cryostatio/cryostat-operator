// Copyright (c) 2021 Red Hat, Inc.
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecordingSpec defines the desired state of Recording
type RecordingSpec struct {
	// Name of the recording to be created.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Name string `json:"name"`
	// TODO Maybe replace with more specific type (e.g. "typeID, option, value" tuples)

	// A list of event options to use when creating the recording.
	// These are used to enable and fine-tune individual events.
	// Examples: "jdk.ExecutionSample:enabled=true", "jdk.ExecutionSample:period=200ms"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	// +listType=atomic
	EventOptions []string `json:"eventOptions"`
	// The requested total duration of the recording, a zero value will record indefinitely.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:number"}
	Duration metav1.Duration `json:"duration"`
	// Desired state of the recording. If omitted, RUNNING will be assumed.
	// +kubebuilder:validation:Enum=RUNNING;STOPPED
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select:RUNNING","urn:alm:descriptor:com.tectonic.ui:select:STOPPED"}

	// +optional
	State *RecordingState `json:"state,omitempty"`
	// Whether this recording should be saved to persistent storage. If true, the JFR file will be retained until
	// this object is deleted. If false, the JFR file will be deleted when its corresponding JVM exits.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:checkbox"}
	Archive bool `json:"archive"`
	// Reference to the FlightRecorder object that corresponds to this Recording
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	FlightRecorder *corev1.LocalObjectReference `json:"flightRecorder"`
}

// RecordingState describes the current state of the recording according
// to JFR
type RecordingState string

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
type RecordingStatus struct {
	// Current state of the recording.
	// +kubebuilder:validation:Enum=CREATED;RUNNING;STOPPING;STOPPED
	// +operator-sdk:csv:customresourcedefinitions:type=status,xDescriptors={"urn:alm:descriptor:text"}
	// +optional
	State *RecordingState `json:"state,omitempty"`
	// The date/time when the recording started.
	// +operator-sdk:csv:customresourcedefinitions:type=status,xDescriptors={"urn:alm:descriptor:text"}
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`
	// The duration of the recording specified during creation.
	// +operator-sdk:csv:customresourcedefinitions:type=status,xDescriptors={"urn:alm:descriptor:text"}
	// +optional
	Duration metav1.Duration `json:"duration,omitempty"`
	// A URL to download the JFR file for the recording.
	// +operator-sdk:csv:customresourcedefinitions:type=status,xDescriptors={"urn:alm:descriptor:org.w3:link"}
	// +optional
	DownloadURL *string `json:"downloadURL,omitempty"`
	// A URL to download the autogenerated HTML report for the recording
	// +operator-sdk:csv:customresourcedefinitions:type=status,xDescriptors={"urn:alm:descriptor:org.w3:link"}
	// +optional
	ReportURL *string `json:"reportURL,omitempty"`
	// TODO Consider adding Conditions:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=recordings,scope=Namespaced

// Recording is the Schema for the recordings API
//+operator-sdk:csv:customresourcedefinitions:resources={{Pod,v1},{Secret,v1},{Service,v1}}
type Recording struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RecordingSpec   `json:"spec,omitempty"`
	Status RecordingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RecordingList contains a list of Recording
type RecordingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Recording `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Recording{}, &RecordingList{})
}
