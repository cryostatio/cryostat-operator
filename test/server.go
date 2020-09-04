package test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ContainerJFRServer is a test HTTP server used to simulate the Container JFR
// backend in unit tests
type ContainerJFRServer struct {
	impl *ghttp.Server
}

// NewServer creates a ContainerJFRServer for use by unit tests
func NewServer(client client.Client, handlers []http.HandlerFunc) *ContainerJFRServer {
	s := ghttp.NewServer()
	s.AppendHandlers(handlers...)

	// Update Service if present
	updateContainerJFRService(s, client)
	return &ContainerJFRServer{
		impl: s,
	}
}

// Close shuts down this test server
func (s *ContainerJFRServer) Close() {
	s.impl.Close()
}

func updateContainerJFRService(server *ghttp.Server, client client.Client) {
	// Look for the Container JFR service as returned by NewContainerJFRService
	svc := &corev1.Service{}
	ctx := context.Background()
	err := client.Get(ctx, types.NamespacedName{Name: "containerjfr", Namespace: "default"}, svc)
	if err == nil {
		// Fill in ClusterIP and Port with values from server URL
		serverURL, err := url.Parse(server.URL())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		svc.Spec.ClusterIP = serverURL.Hostname()
		port, err := strconv.Atoi(serverURL.Port())
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		svc.Spec.Ports[0].Port = int32(port)

		err = client.Update(ctx, svc)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}
