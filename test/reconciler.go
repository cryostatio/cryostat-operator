// Copyright The Cryostat Authors
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
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	cryostatClient "github.com/cryostatio/cryostat-operator/controllers/client"
	"github.com/cryostatio/cryostat-operator/controllers/common"
	"github.com/onsi/gomega"
)

// TestReconcilerConfig groups parameters used to create a test Reconciler
type TestReconcilerConfig struct {
	Server             *CryostatServer
	Client             client.Client
	DisableTLS         *bool
	CoreImageTag       *string
	DatasourceImageTag *string
	GrafanaImageTag    *string
}

// NewTestReconciler returns a common.Reconciler for use by unit tests
func NewTestReconciler(config *TestReconcilerConfig) common.Reconciler {
	return common.NewReconciler(&common.ReconcilerConfig{
		Client:        config.Client,
		ClientFactory: &testClientFactory{config},
		OS:            newTestOSUtils(config),
	})
}

type testClientFactory struct {
	*TestReconcilerConfig
}

func NewTestReconcilerNoServer(client client.Client) common.Reconciler {
	return common.NewReconciler(&common.ReconcilerConfig{
		Client:        client,
		ClientFactory: &testClientFactory{},
		OS:            &testOSUtils{},
	})
}

func NewTestReconcilerTLS(config *TestReconcilerConfig) common.ReconcilerTLS {
	return common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
		Client:  config.Client,
		OSUtils: newTestOSUtils(config),
	})
}

func (c *testClientFactory) CreateClient(config *cryostatClient.Config) (cryostatClient.CryostatClient, error) {
	protocol := "https"
	if c.DisableTLS != nil && *c.DisableTLS {
		protocol = "http"
	}
	// Verify the provided server URL before substituting it
	gomega.Expect(config.ServerURL.String()).To(gomega.Equal(protocol + "://cryostat.default.svc:8181/"))

	// Replace server URL with one to httptest server
	url, err := url.Parse(c.Server.impl.URL())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	config.ServerURL = url

	return cryostatClient.NewHTTPClient(config)
}

type testOSUtils struct {
	envs map[string]string
}

func newTestOSUtils(config *TestReconcilerConfig) *testOSUtils {
	envs := map[string]string{}
	if config.DisableTLS != nil {
		envs["DISABLE_SERVICE_TLS"] = strconv.FormatBool(*config.DisableTLS)
	}
	if config.CoreImageTag != nil {
		envs["RELATED_IMAGE_CORE"] = *config.CoreImageTag
	}
	if config.DatasourceImageTag != nil {
		envs["RELATED_IMAGE_DATASOURCE"] = *config.DatasourceImageTag
	}
	if config.GrafanaImageTag != nil {
		envs["RELATED_IMAGE_GRAFANA"] = *config.GrafanaImageTag
	}
	return &testOSUtils{envs}
}

func (o *testOSUtils) GetFileContents(path string) ([]byte, error) {
	gomega.Expect(path).To(gomega.Equal("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	return []byte("myToken"), nil
}

func (o *testOSUtils) GetEnv(name string) string {
	return o.envs[name]
}
