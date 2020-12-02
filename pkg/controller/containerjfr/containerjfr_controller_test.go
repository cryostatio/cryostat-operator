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
	appsv1 "k8s.io/api/apps/v1"
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

	AfterEach(func() {
		objs = nil
	})

	Describe("reconciling a request", func() {
		Context("when reconciling", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should create certificates", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
				certNames := []string{"containerjfr", "containerjfr-ca", "containerjfr-grafana"}
				for _, certName := range certNames {
					cert := &certv1.Certificate{}
					err := client.Get(context.Background(), types.NamespacedName{Name: certName, Namespace: "default"}, cert)
					Expect(err).ToNot(HaveOccurred())
				}
			})
			It("should create routes", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

				// Update certificate status
				MakeCertificatesReady(client)
				InitializeSecrets(client)

				// Check for routes, ingress configuration needs to be added as each
				// one is created so that they all reconcile successfully
				result, err = controller.Reconcile(req)
				IngressConfig(client, *controller, req, false)
			})
		})
		Context("succesfully creates required resources", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should create persistent volume claim and set owner", func() {
				pvc := &corev1.PersistentVolumeClaim{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
				Expect(err).ToNot(HaveOccurred())

				// Compare to desired spec
				expectedPvc := resource_definitions.NewPersistentVolumeClaimForCR(test.NewContainerJFR())
				Expect(pvc.ObjectMeta.Name).To(Equal(expectedPvc.ObjectMeta.Name))
				Expect(pvc.ObjectMeta.Namespace).To(Equal(expectedPvc.ObjectMeta.Namespace))
				Expect(pvc.Spec.AccessModes).To(Equal(expectedPvc.Spec.AccessModes))
				Expect(pvc.Spec.StorageClassName).To(Equal(expectedPvc.Spec.StorageClassName))

				pvcStorage := pvc.Spec.Resources.Requests["storage"]
				expectedPvcStorage := expectedPvc.Spec.Resources.Requests["storage"]
				Expect(pvcStorage.Equal(expectedPvcStorage)).To(BeTrue())

				// Check for owner
				Expect(pvc.ObjectMeta.OwnerReferences[0].Kind).To(Equal("ContainerJFR"))
				Expect(pvc.ObjectMeta.OwnerReferences[0].Name).To(Equal("containerjfr"))
			})
			It("should create Grafana secret and set owner", func() {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana-basic", Namespace: "default"}, secret)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana-basic", Namespace: "default"}, secret)
				Expect(err).ToNot(HaveOccurred())

				// Compare to desired spec
				expectedSecret := resource_definitions.NewGrafanaSecretForCR(test.NewContainerJFR())
				Expect(secret.ObjectMeta.Name).To(Equal(expectedSecret.ObjectMeta.Name))
				Expect(secret.ObjectMeta.Namespace).To(Equal(expectedSecret.ObjectMeta.Namespace))
				Expect(secret.StringData["GF_SECURITY_ADMIN_USER"]).To(Equal(expectedSecret.StringData["GF_SECURITY_ADMIN_USER"]))

				// Check for owner
				Expect(secret.ObjectMeta.OwnerReferences[0].Kind).To(Equal("ContainerJFR"))
				Expect(secret.ObjectMeta.OwnerReferences[0].Name).To(Equal("containerjfr"))
			})
			It("should create JMX secret and set owner", func() {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-jmx-auth", Namespace: "default"}, secret)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-jmx-auth", Namespace: "default"}, secret)
				Expect(err).ToNot(HaveOccurred())

				expectedSecret := resource_definitions.NewJmxSecretForCR(test.NewContainerJFR())
				Expect(secret.ObjectMeta.Name).To(Equal(expectedSecret.ObjectMeta.Name))
				Expect(secret.ObjectMeta.Namespace).To(Equal(expectedSecret.ObjectMeta.Namespace))
				Expect(secret.StringData["CONTAINER_JFR_RJMX_USER"]).To(Equal(expectedSecret.StringData["CONTAINER_JFR_RJMX_USER"]))

				// Check for owner
				Expect(secret.ObjectMeta.OwnerReferences[0].Kind).To(Equal("ContainerJFR"))
				Expect(secret.ObjectMeta.OwnerReferences[0].Name).To(Equal("containerjfr"))
			})
			It("should create Grafana service and set owner", func() {
				service := &corev1.Service{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
				Expect(err).ToNot(HaveOccurred())

				expectedService := resource_definitions.NewGrafanaService(test.NewContainerJFR())
				Expect(service.ObjectMeta.Name).To(Equal(expectedService.ObjectMeta.Name))
				Expect(service.ObjectMeta.Namespace).To(Equal(expectedService.ObjectMeta.Namespace))
				Expect(service.ObjectMeta.Labels).To(Equal(expectedService.ObjectMeta.Labels))
				Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
				Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
				Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))

				// Check for owner
				Expect(service.ObjectMeta.OwnerReferences[0].Kind).To(Equal("ContainerJFR"))
				Expect(service.ObjectMeta.OwnerReferences[0].Name).To(Equal("containerjfr"))
			})
			It("should create exporter service and set owner", func() {
				service := &corev1.Service{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, service)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, service)
				Expect(err).ToNot(HaveOccurred())

				expectedService := resource_definitions.NewExporterService(test.NewContainerJFR())
				Expect(service.ObjectMeta.Name).To(Equal(expectedService.ObjectMeta.Name))
				Expect(service.ObjectMeta.Namespace).To(Equal(expectedService.ObjectMeta.Namespace))
				Expect(service.ObjectMeta.Labels).To(Equal(expectedService.ObjectMeta.Labels))
				Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
				Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
				Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))

				// Check for owner
				Expect(service.ObjectMeta.OwnerReferences[0].Kind).To(Equal("ContainerJFR"))
				Expect(service.ObjectMeta.OwnerReferences[0].Name).To(Equal("containerjfr"))
			})
			It("should create command channel service and set owner", func() {
				service := &corev1.Service{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, service)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, service)
				Expect(err).ToNot(HaveOccurred())

				expectedService := resource_definitions.NewCommandChannelService(test.NewContainerJFR())
				Expect(service.ObjectMeta.Name).To(Equal(expectedService.ObjectMeta.Name))
				Expect(service.ObjectMeta.Namespace).To(Equal(expectedService.ObjectMeta.Namespace))
				Expect(service.ObjectMeta.Labels).To(Equal(expectedService.ObjectMeta.Labels))
				Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
				Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
				Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))

				// Check for owner
				Expect(service.ObjectMeta.OwnerReferences[0].Kind).To(Equal("ContainerJFR"))
				Expect(service.ObjectMeta.OwnerReferences[0].Name).To(Equal("containerjfr"))
			})
			It("should create deployment and set owner", func() {
				deployment := appsv1.Deployment{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, &deployment)
				Expect(err).To(HaveOccurred())

				ReconcileFully(client, *controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, &deployment)
				Expect(err).ToNot(HaveOccurred())

				testSpecs := NewFakeServiceSpecs()
				testTLSConfig := NewFakeTLSConfig()
				expectedDeployment := resource_definitions.NewDeploymentForCR(test.NewContainerJFR(), &testSpecs, &testTLSConfig)
				Expect(deployment.ObjectMeta.Name).To(Equal(expectedDeployment.ObjectMeta.Name))
				Expect(deployment.ObjectMeta.Namespace).To(Equal(expectedDeployment.ObjectMeta.Namespace))
				Expect(deployment.ObjectMeta.Labels).To(Equal(expectedDeployment.ObjectMeta.Labels))
				Expect(deployment.ObjectMeta.Annotations).To(Equal(expectedDeployment.ObjectMeta.Annotations))
				Expect(deployment.Spec.Selector).To(Equal(expectedDeployment.Spec.Selector))

				// compare Pod template
				template := deployment.Spec.Template
				expectedTemplate := expectedDeployment.Spec.Template
				Expect(template.ObjectMeta).To(Equal(expectedTemplate.ObjectMeta))
				Expect(template.Spec.Containers).To(Equal(expectedTemplate.Spec.Containers))
				Expect(template.Spec.Volumes).To(Equal(expectedTemplate.Spec.Volumes))
			})
		})
		Context("after containerjfr reconciled successfully", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should be idempotent", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				ReconcileFully(client, *controller, false)

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
		Context("After a minimal containerjfr reconciled successfully", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
			})
			It("should be idempotent", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				ReconcileFully(client, *controller, true)

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

func NewFakeServiceSpecs() resource_definitions.ServiceSpecs {
	return resource_definitions.ServiceSpecs{
		CoreHostname:    "test",
		CommandHostname: "test",
		GrafanaURL:      "https://test",
	}
}

func NewFakeTLSConfig() resource_definitions.TLSConfig {
	return resource_definitions.TLSConfig{
		ContainerJFRSecret: "containerjfr-tls",
		GrafanaSecret:      "containerjfr-grafana-tls",
		KeystorePassSecret: "containerjfr-keystore",
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

func IngressConfig(client client.Client, controller containerjfr.ReconcileContainerJFR, req reconcile.Request, minimal bool) {
	routes := []string{"containerjfr", "containerjfr-command"}
	if !minimal {
		routes = append([]string{"containerjfr-grafana"}, routes...)
	}
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

func ReconcileFully(client client.Client, controller containerjfr.ReconcileContainerJFR, minimal bool) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	MakeCertificatesReady(client)
	InitializeSecrets(client)

	// Add ingress config to routes
	result, err = controller.Reconcile(req)
	IngressConfig(client, controller, req, minimal)

	result, err = controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}
