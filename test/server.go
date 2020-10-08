package test

import (
	"context"
	"encoding/pem"
	"net/http"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
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
	s := ghttp.NewTLSServer()
	s.AppendHandlers(handlers...)
	updateCACert(client, s)
	return &ContainerJFRServer{
		impl: s,
	}
}

// Close shuts down this test server
func (s *ContainerJFRServer) Close() {
	s.impl.Close()
}

func updateCACert(client client.Client, server *ghttp.Server) {
	ctx := context.Background()

	// Fetch CA Certificate
	caCert := &certv1.Certificate{}
	err := client.Get(ctx, types.NamespacedName{Name: "containerjfr-ca", Namespace: "default"}, caCert)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Get the test server's certificate
	certs := server.HTTPTestServer.TLS.Certificates
	gomega.Expect(certs).To(gomega.HaveLen(1))
	rawCerts := certs[0].Certificate
	gomega.Expect(rawCerts).To(gomega.HaveLen(1))

	// Encode certificate in PEM format
	certPem := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rawCerts[0],
	}
	pemData := pem.EncodeToMemory(certPem)

	// Create corresponding Secret
	secret := newCASecret(pemData)
	err = client.Create(ctx, secret)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Update Certificate to Ready
	caCert.Status.Conditions = append(caCert.Status.Conditions, certv1.CertificateCondition{
		Type:   certv1.CertificateConditionReady,
		Status: certMeta.ConditionTrue,
	})
	err = client.Status().Update(ctx, caCert)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}
