package test

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/gomega"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
)

// NewTestReconciler returns a common.Reconciler for use by unit tests
func NewTestReconciler(client client.Client) common.Reconciler {
	return common.NewReconciler(&common.ReconcilerConfig{
		Client:     client,
		OS:         &testOSUtils{},
		DisableTLS: true, // FIXME look into switching tests to HTTPS
	})
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
