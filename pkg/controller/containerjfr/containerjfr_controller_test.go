// Copyright (c) 2020 Red Hat, Inc.
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

package containerjfr_test

import (
	"context"
	"time"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	openshiftv1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/containerjfr"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/containerjfr/resource_definitions"
	"github.com/rh-jmc-team/container-jfr-operator/test"
)

var _ = Describe("ContainerjfrController", func() {
	var (
		objs       []runtime.Object
		client     client.Client
		controller *containerjfr.ReconcileContainerJFR
	)

	JustBeforeEach(func() {
		logf.SetLogger(zap.Logger())
		s := test.NewTestScheme()

		client = fake.NewFakeClientWithScheme(s, objs...)
		controller = &containerjfr.ReconcileContainerJFR{
			Client:        client,
			Scheme:        s,
			ReconcilerTLS: test.NewTestReconcilerTLS(client),
		}
	})

	BeforeEach(func() {
		objs = []runtime.Object{
			test.NewContainerJFR(),
		}
	})

	AfterEach(func() {
		objs = nil
	})

	Describe("reconciling a request", func() {
		Context("after Containerjfr already reconciled successfully", func() {
			It("should be idempotent", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				ReconcileFully(client, *controller)

				obj := &rhjmcv1beta1.ContainerJFR{}
				err := client.Get(context.Background(), req.NamespacedName, obj)
				Expect(err).ToNot(HaveOccurred())

				// Reconcile again
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				obj2 := &rhjmcv1beta1.ContainerJFR{}
				err = client.Get(context.Background(), req.NamespacedName, obj2)
				Expect(err).ToNot(HaveOccurred())
				Expect(obj2.Status).To(Equal(obj.Status))
				Expect(obj2.Spec).To(Equal(obj.Spec))
			})
		})
		Context("succesfully creates the resources", func() {
			It("should create persistent volume claim", func() {
				pvc := &corev1.PersistentVolumeClaim{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
				Expect(err).ToNot(HaveOccurred())
				// To Do: compare created pvc to desired spec
				Expect(pvc.Spec).To(Equal(resource_definitions.NewPersistentVolumeClaimForCR(test.NewContainerJFR()).Spec))
			})

		})

	})
})

func NewFakeSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Data: map[string][]byte{
			corev1.TLSCertKey: nil,
		},
	}

}

func MakeCertificatesReady(client client.Client) {
	certNames := []string{"containerjfr", "containerjfr-ca", "containerjfr-grafana"}
	for _, certName := range certNames {
		cert := &certv1.Certificate{}
		err := client.Get(context.Background(), types.NamespacedName{Name: certName, Namespace: "default"}, cert)
		Expect(err).ToNot(HaveOccurred())
		cert.Status.Conditions = append(cert.Status.Conditions, certv1.CertificateCondition{
			Type:   certv1.CertificateConditionReady,
			Status: certMeta.ConditionTrue,
		})
		err = client.Status().Update(context.Background(), cert)
		Expect(err).ToNot(HaveOccurred())
	}
}

func InitializeSecrets(client client.Client) {
	// Create secrets
	secretNames := []string{"containerjfr-ca", "containerjfr-tls", "containerjfr-grafana-tls"}
	for _, secretName := range secretNames {
		secret := NewFakeSecret(secretName)
		err := client.Create(context.Background(), secret)
		Expect(err).ToNot(HaveOccurred())
	}
}

func ingressConfig(client client.Client, controller containerjfr.ReconcileContainerJFR, req reconcile.Request) {
	routes := []string{"containerjfr-grafana", "containerjfr", "containerjfr-command"}
	for _, routeName := range routes {
		route := &openshiftv1.Route{}
		err := client.Get(context.Background(), types.NamespacedName{Name: routeName, Namespace: "default"}, route)
		Expect(err).ToNot(HaveOccurred())
		route.Status.Ingress = append(route.Status.Ingress, openshiftv1.RouteIngress{
			Host: "test",
		})
		err = client.Status().Update(context.Background(), route)
		Expect(err).ToNot(HaveOccurred())
		_, err = controller.Reconcile(req)
	}
}

func ReconcileFully(client client.Client, controller containerjfr.ReconcileContainerJFR) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	MakeCertificatesReady(client)
	InitializeSecrets(client)

	// Add ingress config to routes
	result, err = controller.Reconcile(req)
	ingressConfig(client, controller, req)

	result, err = controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}
