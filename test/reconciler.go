package test

import (
	"strconv"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/gomega"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
)

// NewTestReconciler returns a common.Reconciler for use by unit tests
func NewTestReconciler(client client.Client) common.Reconciler {
	return common.NewReconciler(&common.ReconcilerConfig{
		Client:    client,
		Connector: &testConnector{},
		OS:        &testOSUtils{},
	})
}

type testConnector struct {
	uidCount int
}

func (c *testConnector) Connect(config *jfrclient.Config) (jfrclient.ContainerJfrClient, error) {
	uidFunc := func() types.UID {
		uid := strconv.Itoa(c.uidCount)
		c.uidCount++
		return types.UID(uid)
	}

	config.UIDProvider = uidFunc
	return jfrclient.Create(config)
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
