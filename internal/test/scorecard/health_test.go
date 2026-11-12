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
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
)

type healthRoundTripper func(*http.Request) (*http.Response, error)

func (f healthRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func healthyResponse(_ *http.Response, _ *scapiv1alpha3.TestResult) (bool, error) {
	return true, nil
}

// Simulate /health requests failing during Cryostat startup, then succeeding.
// Verify that transient connection errors are retried and recovery is detected.
// Check that persistent failures stop at the deadline and log the cause.
// Configuration errors and HTTP 401 responses must fail without retries.
var _ = Describe("Health polling", func() {
	DescribeTable("retries startup connection errors", func(startupError error) {
		// Fail the first request at the transport layer, then forward the retry to
		// the real local HTTP server to verify that polling recovers successfully.
		var successfulRequests atomic.Int32
		api := testCryostatAPI(func(w http.ResponseWriter, request *http.Request) {
			Expect(request.Method).To(Equal(http.MethodGet))
			Expect(request.URL.Path).To(Equal("/health"))
			successfulRequests.Add(1)
			w.WriteHeader(http.StatusOK)
		})
		client := NewHttpClient()
		transport := client.Transport
		DeferCleanup(transport.(*http.Transport).CloseIdleConnections)
		attempts := 0
		client.Transport = healthRoundTripper(func(request *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return nil, startupError
			}
			return transport.RoundTrip(request)
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := newTestResources("test", "test").pollHealth(ctx, client, api.Targets().Base, healthyResponse)
		Expect(err).ToNot(HaveOccurred())
		Expect(attempts).To(Equal(2))
		Expect(successfulRequests.Load()).To(Equal(int32(1)))
	},
		Entry("closed connection", io.EOF),
		Entry("connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}),
		Entry("connection reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}),
	)

	It("stops retrying connection failures at the deadline and logs the cause", func() {
		// Refuse every connection without contacting the server. Polling must stop
		// at the short context deadline and retain the connection error in its log.
		api := testCryostatAPI(func(_ http.ResponseWriter, _ *http.Request) {
			Fail("unreachable server received a request")
		})
		client := &http.Client{Transport: healthRoundTripper(func(_ *http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		})}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		resources := newTestResources("test", "test")
		err := resources.pollHealth(ctx, client, api.Targets().Base, healthyResponse)
		Expect(err).To(MatchError(context.DeadlineExceeded))
		Expect(resources.Log).To(ContainSubstring("connection refused"))
	})

	It("returns permanent transport errors without retrying", func() {
		api := testCryostatAPI(func(_ http.ResponseWriter, _ *http.Request) {
			Fail("invalid transport sent a request")
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
		Expect(err).To(MatchError(permanent))
		Expect(attempts).To(Equal(1))
	})

	It("returns authentication failures without retrying", func() {
		var attempts atomic.Int32
		api := testCryostatAPI(func(w http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
		client := NewHttpClient()
		DeferCleanup(client.CloseIdleConnections)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := newTestResources("test", "test").pollHealth(ctx, client, api.Targets().Base, healthyResponse)
		Expect(err).To(MatchError(ContainSubstring("401")))
		Expect(attempts.Load()).To(Equal(int32(1)))
	})
})
