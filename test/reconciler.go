package test

import (
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/gomega"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
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
	gomega.Expect(name).To(gomega.Equal("TLS_VERIFY"))
	return "false"
}
