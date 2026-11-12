// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scorecard

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Simulate Cryostat returning 503, then ineligible targets, then the matching Agent.
// Verify registration selects the Pod's HTTP(S) target without falling back to JMX.
// Check Agent injection and preservation of the application's JVM options.
// Require newly created recordings to appear as RUNNING in the recording list.
var _ = Describe("Agent registration", func() {
	It("selects only the matching HTTP target after Cryostat and the Agent become ready", func() {
		var calls atomic.Int32
		api := testCryostatAPI(func(w http.ResponseWriter, request *http.Request) {
			Expect(request.Method).To(Equal(http.MethodGet))
			Expect(request.URL.Path).To(Equal("/api/v4/targets"))
			call := calls.Add(1)
			if call == 1 {
				http.Error(w, "starting", http.StatusServiceUnavailable)
				return
			}
			targets := []Target{
				{Id: 1, Alias: "application-pod", ConnectUrl: "service:jmx:rmi:///jndi/rmi://localhost:9091/jmxrmi"},
				{Id: 2, Alias: "different-pod", ConnectUrl: "https://another-app:9977"},
				{Id: 0, Alias: "application-pod", ConnectUrl: "https://uninitialized:9977"},
			}
			if call >= 3 {
				targets = append(targets, Target{Id: 3, Alias: "application-pod", ConnectUrl: "https://agent:9977"})
			}
			Expect(json.NewEncoder(w).Encode(targets)).To(Succeed())
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		target, err := waitForAgentTarget(ctx, api.Targets(), "application-pod")
		Expect(err).ToNot(HaveOccurred())
		Expect(target.Id).To(Equal(uint32(3)))
		Expect(calls.Load()).To(Equal(int32(3)))
	})

	It("times out without falling back to a JMX target", func() {
		api := testCryostatAPI(func(w http.ResponseWriter, _ *http.Request) {
			Expect(json.NewEncoder(w).Encode([]Target{{Id: 1, Alias: "application-pod", ConnectUrl: "service:jmx:rmi://localhost:9091"}})).To(Succeed())
		})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		target, err := waitForAgentTarget(ctx, api.Targets(), "application-pod")
		Expect(err).To(MatchError(ContainSubstring(context.DeadlineExceeded.Error())))
		Expect(target).To(BeNil())
	})
})

var _ = Describe("Agent injection verification", func() {
	DescribeTable("detects webhook failures", func(injected bool, options string, autoConfig, wantError bool) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "application-pod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "sample", Env: []corev1.EnvVar{{Name: "JAVA_TOOL_OPTIONS", Value: options}}}}},
		}
		if injected {
			pod.Spec.InitContainers = []corev1.Container{{Name: "cryostat-agent-init"}}
		}
		err := verifyAgentApplication(pod, autoConfig)
		if wantError {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	},
		Entry("injection and options preserved", true, "-Xmx128m -javaagent:/opt/cryostat/agent.jar", true, false),
		Entry("fail-open webhook skipped injection", false, "-Xmx128m", true, true),
		Entry("init container without JVM attachment", true, "-Xmx128m", true, true),
		Entry("application options overwritten", true, "-javaagent:/opt/cryostat/agent.jar", true, true),
		Entry("manual Agent", false, "-Xmx128m -javaagent:/opt/cryostat/agent.jar", false, false),
		Entry("manual test unexpectedly used injection", true, "-Xmx128m -javaagent:/opt/cryostat/agent.jar", false, true),
	)
})

var _ = Describe("Recording lifecycle", func() {
	It("requires the running recording to appear in the recording list", func() {
		api := testCryostatAPI(func(w http.ResponseWriter, request *http.Request) {
			switch request.Method {
			case http.MethodPost:
				Expect(json.NewEncoder(w).Encode(Recording{Id: 7, Name: "scorecard_test_rec", State: "RUNNING"})).To(Succeed())
			case http.MethodGet:
				_, err := w.Write([]byte("[]"))
				Expect(err).ToNot(HaveOccurred())
			default:
				Fail("lifecycle proceeded after missing recording: " + request.Method)
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := newTestResources("test", "test").exerciseRecording(ctx, api, &Target{Id: 1})
		Expect(err).To(MatchError(ContainSubstring("not found in recording list")))
	})
})
