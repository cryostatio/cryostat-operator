package test

import (
	"net/http"

	"github.com/onsi/gomega/ghttp"
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
	return &ContainerJFRServer{
		impl: s,
	}
}

// Close shuts down this test server
func (s *ContainerJFRServer) Close() {
	s.impl.Close()
}
