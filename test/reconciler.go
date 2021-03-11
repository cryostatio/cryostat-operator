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

package test

import (
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/gomega"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/controllers/client"
	"github.com/rh-jmc-team/container-jfr-operator/controllers/common"
)

// NewTestReconciler returns a common.Reconciler for use by unit tests
func NewTestReconciler(server *ContainerJFRServer, client client.Client) common.Reconciler {
	return common.NewReconciler(&common.ReconcilerConfig{
		Client:        client,
		ClientFactory: &testClientFactory{serverURL: server.impl.URL()},
		OS:            &testOSUtils{},
	})
}

type testClientFactory struct {
	serverURL string
}

func NewTestReconcilerNoServer(client client.Client) common.Reconciler {
	return common.NewReconciler(&common.ReconcilerConfig{
		Client:        client,
		ClientFactory: &testClientFactory{},
		OS:            &testOSUtils{},
	})
}

func NewTestReconcilerTLS(client client.Client) common.ReconcilerTLS {
	return common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
		Client: client,
		OS:     &testOSUtils{},
	})
}

func (c *testClientFactory) CreateClient(config *jfrclient.Config) (jfrclient.ContainerJfrClient, error) {
	// Verify the provided server URL before substituting it
	gomega.Expect(config.ServerURL.String()).To(gomega.Equal("https://containerjfr.default.svc:8181/"))

	// Replace server URL with one to httptest server
	url, err := url.Parse(c.serverURL)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	config.ServerURL = url

	return jfrclient.NewHTTPClient(config)
}

type testOSUtils struct{}

func (o *testOSUtils) GetFileContents(path string) ([]byte, error) {
	gomega.Expect(path).To(gomega.Equal("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	return []byte("myToken"), nil
}

func (o *testOSUtils) GetEnv(name string) string {
	gomega.Expect(name).To(gomega.Equal("DISABLE_SERVICE_TLS"))
	return ""
}
