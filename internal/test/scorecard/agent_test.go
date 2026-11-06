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
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testCryostatAPI(t *testing.T, handler http.HandlerFunc) *CryostatRESTClientset {
	t.Helper()
	// REST requests load a Kubernetes bearer token even when talking to this fake API.
	config := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(config, []byte(`apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: test
    user: test
clusters:
- name: test
  cluster:
    server: https://unused.invalid
users:
- name: test
  user:
    token: scorecard-test
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", config)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return newTestResources("test", "test").NewCryostatRESTClientset(base)
}

func TestAgentRegistrationSelectsOnlyMatchingHTTPTarget(t *testing.T) {
	calls := 0
	api := testCryostatAPI(t, func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v4/targets" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		calls++
		if calls == 1 {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		targets := []Target{
			{Id: 1, Alias: "application-pod", ConnectUrl: "service:jmx:rmi:///jndi/rmi://localhost:9091/jmxrmi"},
			{Id: 2, Alias: "different-pod", ConnectUrl: "https://another-app:9977"},
			{Id: 0, Alias: "application-pod", ConnectUrl: "https://uninitialized:9977"},
		}
		if calls >= 3 {
			targets = append(targets, Target{Id: 3, Alias: "application-pod", ConnectUrl: "https://agent:9977"})
		}
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	target, err := waitForAgentTarget(ctx, api.Targets(), "application-pod")
	if err != nil {
		t.Fatal(err)
	}
	if target.Id != 3 {
		t.Fatalf("selected an unrelated or JMX target: %+v", target)
	}
}

func TestAgentRegistrationTimeoutDoesNotFallBackToJMX(t *testing.T) {
	api := testCryostatAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode([]Target{{Id: 1, Alias: "application-pod", ConnectUrl: "service:jmx:rmi://localhost:9091"}}); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	target, err := waitForAgentTarget(ctx, api.Targets(), "application-pod")
	if err == nil || target != nil {
		t.Fatalf("expected registration failure, got target=%+v, err=%v", target, err)
	}
}

func TestAgentInjectionVerificationDetectsWebhookFailures(t *testing.T) {
	cases := []struct {
		name       string
		injected   bool
		options    string
		autoConfig bool
		wantError  bool
	}{
		{"injection and options preserved", true, "-Xmx128m -javaagent:/opt/cryostat/agent.jar", true, false},
		{"fail-open webhook skipped injection", false, "-Xmx128m", true, true},
		{"init container without JVM attachment", true, "-Xmx128m", true, true},
		{"application options overwritten", true, "-javaagent:/opt/cryostat/agent.jar", true, true},
		{"manual Agent", false, "-Xmx128m -javaagent:/opt/cryostat/agent.jar", false, false},
		{"manual test unexpectedly used injection", true, "-Xmx128m -javaagent:/opt/cryostat/agent.jar", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "application-pod"},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "sample", Env: []corev1.EnvVar{{Name: "JAVA_TOOL_OPTIONS", Value: tc.options}}}}},
			}
			if tc.injected {
				pod.Spec.InitContainers = []corev1.Container{{Name: "cryostat-agent-init"}}
			}
			if err := verifyAgentApplication(pod, tc.autoConfig); (err != nil) != tc.wantError {
				t.Fatalf("verification error = %v, wantError = %t", err, tc.wantError)
			}
		})
	}
}

func TestRecordingLifecycleChecksRunningRecordingIsListed(t *testing.T) {
	api := testCryostatAPI(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			if err := json.NewEncoder(w).Encode(Recording{Id: 7, Name: "scorecard_test_rec", State: "RUNNING"}); err != nil {
				t.Error(err)
			}
		case http.MethodGet:
			if _, err := w.Write([]byte("[]")); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("lifecycle proceeded after missing recording: %s", request.Method)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := newTestResources("test", "test").exerciseRecording(ctx, api, &Target{Id: 1})
	if err == nil || !strings.Contains(err.Error(), "not found in recording list") {
		t.Fatalf("expected missing-recording failure, got %v", err)
	}
}
