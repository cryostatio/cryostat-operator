// Copyright (c) 2020 Red Hat, Inc.
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

package test

import (
	"time"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewContainerJFR() *rhjmcv1alpha1.ContainerJFR {
	return &rhjmcv1alpha1.ContainerJFR{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "containerjfr",
			Namespace: "default",
		},
		Spec: rhjmcv1alpha1.ContainerJFRSpec{
			Minimal: false,
		},
	}
}

func NewFlightRecorder() *rhjmcv1alpha2.FlightRecorder {
	return &rhjmcv1alpha2.FlightRecorder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: rhjmcv1alpha2.FlightRecorderSpec{
			RecordingSelector: metav1.AddLabelToSelector(&metav1.LabelSelector{}, rhjmcv1alpha2.RecordingLabel, "test-pod"),
		},
		Status: rhjmcv1alpha2.FlightRecorderStatus{
			Target: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       "test-pod",
				Namespace:  "default",
			},
			Port: 8001,
		},
	}
}

func NewRecording(continuous bool) *rhjmcv1alpha2.Recording {
	return &rhjmcv1alpha2.Recording{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-recording",
			Namespace: "default",
		},
		Spec: rhjmcv1alpha2.RecordingSpec{
			Name: "test-recording",
			EventOptions: []string{
				"jdk.socketRead:enabled=true",
				"jdk.socketWrite:enabled=true",
			},
			Duration: metav1.Duration{Duration: getDuration(continuous)},
			Archive:  true,
			FlightRecorder: &corev1.LocalObjectReference{
				Name: "test-pod",
			},
		},
	}
}

func NewRunningRecording(continuous bool) *rhjmcv1alpha2.Recording {
	state := rhjmcv1alpha2.RecordingStateRunning
	url := "http://path/to/test-recording.jfr"
	return &rhjmcv1alpha2.Recording{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-recording",
			Namespace:  "default",
			Finalizers: []string{"recording.finalizer.rhjmc.redhat.com"},
		},
		Spec: rhjmcv1alpha2.RecordingSpec{
			Name: "test-recording",
			EventOptions: []string{
				"jdk.socketRead:enabled=true",
				"jdk.socketWrite:enabled=true",
			},
			Duration: metav1.Duration{Duration: getDuration(continuous)},
			Archive:  true,
			FlightRecorder: &corev1.LocalObjectReference{
				Name: "test-pod",
			},
		},
		Status: rhjmcv1alpha2.RecordingStatus{
			State:       &state,
			StartTime:   metav1.Unix(0, 1597090030341*int64(time.Millisecond)),
			Duration:    metav1.Duration{Duration: 30 * time.Second},
			DownloadURL: &url,
		},
	}
}

func getDuration(continuous bool) time.Duration {
	seconds := 0
	if !continuous {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func NewTargetPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			PodIP: "1.2.3.4",
		},
	}
}

func NewContainerJFRService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "containerjfr",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "TBD",
			Ports: []corev1.ServicePort{
				{
					Name: "export",
					Port: -1,
				},
			},
		},
	}
}
