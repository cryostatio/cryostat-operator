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
	"context"
	"time"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	"github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type commonTestClient struct {
	ctrlclient.Client
}

func newCommonTestClient(client ctrlclient.Client) *commonTestClient {
	return &commonTestClient{
		Client: client,
	}
}

type testClient struct {
	*commonTestClient
	*TestResources
}

// NewTestClient returns a client to be used by the Cryostat controller under test.
// This client wraps an existing client and mocks the behaviour of external Kubernetes
// controllers that are not present in the test environment.
func NewTestClient(client ctrlclient.Client, resources *TestResources) ctrlclient.Client {
	return &testClient{
		commonTestClient: newCommonTestClient(client),
		TestResources:    resources,
	}
}

func (c *testClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
	err := c.Client.Get(ctx, key, obj)
	if err != nil {
		return err
	}
	// If this is a certificate or route, update the status after the first successful Get operation
	c.makeCertificatesReady(ctx, obj)
	c.updateRouteStatus(ctx, obj)
	return nil
}

func (c *testClient) makeCertificatesReady(ctx context.Context, obj runtime.Object) {
	// If this object is one of the operator-managed certificates, mock the behaviour
	// of cert-manager processing those certificates
	cert, ok := obj.(*certv1.Certificate)
	if ok && c.matchesName(cert, c.NewCryostatCert(), c.NewCACert(), c.NewGrafanaCert(), c.NewReportsCert()) &&
		len(cert.Status.Conditions) == 0 {
		// Create certificate secret
		c.createCertSecret(ctx, cert)
		// Mark certificate as ready
		cert.Status.Conditions = append(cert.Status.Conditions, certv1.CertificateCondition{
			Type:   certv1.CertificateConditionReady,
			Status: certMeta.ConditionTrue,
		})
		err := c.Status().Update(context.Background(), cert)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

func (c *testClient) createCertSecret(ctx context.Context, cert *certv1.Certificate) {
	// The secret's data isn't important, we simply need it to exist
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cert.Spec.SecretName,
			Namespace: cert.Namespace,
		},
		Data: map[string][]byte{
			corev1.TLSCertKey: []byte(cert.Name + "-bytes"),
		},
	}
	err := c.Create(ctx, secret)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

func (c *testClient) updateRouteStatus(ctx context.Context, obj runtime.Object) {
	// If this object is an operator-managed route, mock the behaviour
	// of OpenShift's router by setting a dummy hostname in its Status
	route, ok := obj.(*routev1.Route)
	if ok && c.matchesName(route, c.NewGrafanaRoute(), c.NewCoreRoute()) &&
		len(route.Status.Ingress) == 0 {
		route.Status.Ingress = append(route.Status.Ingress, routev1.RouteIngress{
			Host: route.Name + ".example.com",
		})
		err := c.Status().Update(context.Background(), route)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
}

// TODO When using envtest instead of fake client, this is probably no longer needed
type timestampClient struct {
	*commonTestClient
}

func NewClientWithTimestamp(client ctrlclient.Client) ctrlclient.Client {
	return &timestampClient{
		commonTestClient: newCommonTestClient(client),
	}
}

func (c *timestampClient) Create(ctx context.Context, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
	err := SetCreationTimestamp(obj)
	if err != nil {
		return err
	}
	return c.Client.Create(ctx, obj, opts...)
}

var creationTimestamp = metav1.NewTime(time.Unix(1664573254, 0))

func SetCreationTimestamp(objs ...runtime.Object) error {
	for _, obj := range objs {
		metaObj, err := meta.Accessor(obj)
		if err != nil {
			return err
		}
		metaObj.SetCreationTimestamp(creationTimestamp)
	}
	return nil
}

type clientUpdateError struct {
	*commonTestClient
	failObj ctrlclient.Object
	err     *kerrors.StatusError
}

// NewClientWithUpdateError wraps a Client by returning an error when updating
// a specified object
func NewClientWithUpdateError(client ctrlclient.Client, failObj ctrlclient.Object,
	err *kerrors.StatusError) ctrlclient.Client {
	return &clientUpdateError{
		commonTestClient: newCommonTestClient(client),
		failObj:          failObj,
		err:              err,
	}
}

func (c *clientUpdateError) Update(ctx context.Context, obj ctrlclient.Object,
	opts ...ctrlclient.UpdateOption) error {
	if obj.GetName() == c.failObj.GetName() && obj.GetNamespace() == c.failObj.GetNamespace() {
		// Look up Kind and compare against object to fail on
		match, err := c.matchesKind(obj, c.failObj)
		if err != nil {
			return err
		}
		if *match {
			return c.err
		}
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *commonTestClient) matchesKind(obj, expected ctrlclient.Object) (*bool, error) {
	match := false
	expectKinds, _, err := c.Scheme().ObjectKinds(expected)
	if err != nil {
		return nil, err
	}
	kinds, _, err := c.Scheme().ObjectKinds(obj)
	if err != nil {
		return nil, err
	}

	for _, expectKind := range expectKinds {
		for _, kind := range kinds {
			if expectKind == kind {
				match = true
				return &match, nil
			}
		}
	}
	return &match, nil
}

func (c *commonTestClient) matchesName(obj ctrlclient.Object, expectedObjs ...ctrlclient.Object) bool {
	for _, expected := range expectedObjs {
		if obj.GetName() == expected.GetName() {
			return true
		}
	}
	return false
}
