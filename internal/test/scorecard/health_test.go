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
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
)

type healthRoundTripper func(*http.Request) (*http.Response, error)

func (f healthRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func healthyResponse(_ *http.Response, _ *scapiv1alpha3.TestResult) (bool, error) {
	return true, nil
}

func TestHealthPollingRetriesStartupConnectionErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"closed connection", io.EOF},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		{"connection reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var successfulRequests atomic.Int32
			api := testCryostatAPI(t, func(w http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.Path != "/health" {
					t.Errorf("unexpected health request: %s %s", request.Method, request.URL.Path)
				}
				successfulRequests.Add(1)
				w.WriteHeader(http.StatusOK)
			})
			client := NewHttpClient()
			transport := client.Transport
			defer transport.(*http.Transport).CloseIdleConnections()
			attempts := 0
			client.Transport = healthRoundTripper(func(request *http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return nil, tc.err
				}
				return transport.RoundTrip(request)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := newTestResources("test", "test").pollHealth(ctx, client, api.Targets().Base, healthyResponse)
			if err != nil {
				t.Fatalf("health polling stopped at a startup connection error: %v", err)
			}
			if attempts != 2 || successfulRequests.Load() != 1 {
				t.Fatalf("expected retry followed by a real successful response, attempts=%d, responses=%d", attempts, successfulRequests.Load())
			}
		})
	}
}

func TestHealthPollingConnectionFailuresRespectDeadline(t *testing.T) {
	api := testCryostatAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unreachable server received a request")
	})
	client := &http.Client{Transport: healthRoundTripper(func(_ *http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	resources := newTestResources("test", "test")
	err := resources.pollHealth(ctx, client, api.Targets().Base, healthyResponse)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded retries until the health deadline, got %v", err)
	}
	if !strings.Contains(resources.Log, "connection refused") {
		t.Fatalf("missing connection failure diagnostics: %s", resources.Log)
	}
}

func TestHealthPollingDoesNotRetryPermanentTransportErrors(t *testing.T) {
	api := testCryostatAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("invalid transport sent a request")
	})
	permanent := errors.New("invalid transport configuration")
	attempts := 0
	client := &http.Client{Transport: healthRoundTripper(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return nil, permanent
	})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := newTestResources("test", "test").pollHealth(ctx, client, api.Targets().Base, healthyResponse)
	if !errors.Is(err, permanent) || attempts != 1 {
		t.Fatalf("expected immediate permanent error, got error=%v, attempts=%d", err, attempts)
	}
}

func TestHealthPollingDoesNotRetryUnauthorizedResponses(t *testing.T) {
	var attempts atomic.Int32
	api := testCryostatAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	client := NewHttpClient()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := newTestResources("test", "test").pollHealth(ctx, client, api.Targets().Base, healthyResponse)
	if err == nil || !strings.Contains(err.Error(), "401") || attempts.Load() != 1 {
		t.Fatalf("expected immediate authentication failure, got error=%v, attempts=%d", err, attempts.Load())
	}
}
