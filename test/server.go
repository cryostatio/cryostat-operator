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
func NewServer(client client.Client, handlers []http.HandlerFunc, tls bool) *ContainerJFRServer {
	var server *ghttp.Server
	if !tls {
		server = ghttp.NewServer()
	} else {
		server = ghttp.NewTLSServer()
		updateCACert(client, server)
	}
	server.AppendHandlers(handlers...)
	return &ContainerJFRServer{
		impl: server,
	}
}

// VerifyRequestsReceived checks that the number of requests received by the server
// match the length of the handlers argument
func (s *ContainerJFRServer) VerifyRequestsReceived(handlers []http.HandlerFunc) {
	gomega.Expect(s.impl.ReceivedRequests()).To(gomega.HaveLen(len(handlers)))
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
