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
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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
		client     ctrlclient.Client
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
		Context("succesfully creates required resources", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should create certificates", func() {
				expectCertificates(client, controller)
			})
			It("should create routes", func() {
				expectRoutes(client, controller, false)
			})
			It("should create persistent volume claim and set owner", func() {
				expectPVC(client, controller, false)
			})
			It("should create Grafana secret and set owner", func() {
				secret := &corev1.Secret{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana-basic", Namespace: "default"}, secret)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				reconcileFully(client, controller, false)

				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana-basic", Namespace: "default"}, secret)
				Expect(err).ToNot(HaveOccurred())

				// Compare to desired spec
				expectedSecret := resource_definitions.NewGrafanaSecretForCR(test.NewContainerJFR())
				checkMetadata(secret, expectedSecret)
				Expect(secret.StringData["GF_SECURITY_ADMIN_USER"]).To(Equal(expectedSecret.StringData["GF_SECURITY_ADMIN_USER"]))
			})
			It("should create JMX secret and set owner", func() {
				expectJMXSecret(client, controller, false)
			})
			It("should create Grafana service and set owner", func() {
				service := &corev1.Service{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				reconcileFully(client, controller, false)
				checkGrafanaService(client)
			})
			It("should create exporter service and set owner", func() {
				expectExporterService(client, controller, false)
			})
			It("should create command channel service and set owner", func() {
				expectCommandChannel(client, controller, false)
			})
			It("should create deployment and set owner", func() {
				expectDeployment(client, controller, false)
			})
		})
		Context("succesfully creates required resources for minimal deployment", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
			})
			It("should create certificates", func() {
				expectCertificates(client, controller)
			})
			It("should create routes", func() {
				expectRoutes(client, controller, true)
			})
			It("should create persistent volume claim and set owner", func() {
				expectPVC(client, controller, true)
			})
			It("should create JMX secret and set owner", func() {
				expectJMXSecret(client, controller, true)
			})
			It("should create exporter service and set owner", func() {
				expectExporterService(client, controller, true)
			})
			It("should create command channel service and set owner", func() {
				expectCommandChannel(client, controller, true)
			})
			It("should create deployment and set owner", func() {
				expectDeployment(client, controller, true)
			})
		})
		Context("after containerjfr reconciled successfully", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should be idempotent", func() {
				expectIdempotence(client, controller, false)
			})
		})
		Context("After a minimal containerjfr reconciled successfully", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
			})
			It("should be idempotent", func() {
				expectIdempotence(client, controller, true)
			})
		})
		Context("ContainerJFR does not exist", func() {
			It("Should do nothing", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("Switching from a minimal to a non-minimal deployment", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
			})
			JustBeforeEach(func() {
				reconcileFully(client, controller, true)

				cjfr := &rhjmcv1beta1.ContainerJFR{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cjfr)
				Expect(err).ToNot(HaveOccurred())

				cjfr.Spec.Minimal = false
				err = client.Status().Update(context.Background(), cjfr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				_, err = controller.Reconcile(req)
				Expect(err).To(HaveOccurred())
				ingressConfig(client, controller, req, false)
				_, err = controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should create grafana resources", func() {
				checkGrafanaService(client)
			})
			It("should configure deployment appropriately", func() {
				checkDeployment(client, false)
			})
		})
		Context("Switching from a non-minimal to a minimal deployment", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			JustBeforeEach(func() {
				reconcileFully(client, controller, false)

				cjfr := &rhjmcv1beta1.ContainerJFR{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cjfr)
				Expect(err).ToNot(HaveOccurred())

				cjfr.Spec.Minimal = true
				err = client.Status().Update(context.Background(), cjfr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				_, err = controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should delete grafana resources", func() {
				service := &corev1.Service{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				route := &openshiftv1.Route{}
				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, route)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
			It("should configure deployment appropriately", func() {
				checkDeployment(client, true)
			})
		})
		Context("Container jfr has list of certificate secrets", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFRWithSecrets(), newFakeSecret("testCert1"), newFakeSecret("testCert2"),
				}
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				reconcileFully(client, controller, false)
				deployment := &appsv1.Deployment{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := test.NewVolumesWithSecrets()
				Expect(&volumes).To(Equal(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewVolumeMountsWithSecrets()
				Expect(&volumeMounts).To(Equal(expectedVolumeMounts))
			})
		})
		Context("Adding a certificate to the TrustedCertSecrets list", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(), newFakeSecret("testCert1"), newFakeSecret("testCert2"),
				}
			})
			JustBeforeEach(func() {
				reconcileFully(client, controller, false)
			})
			It("Should update the corresponding deployment", func() {
				// Get ContainerJFR CR after reconciling
				cr := &rhjmcv1beta1.ContainerJFR{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
				Expect(err).ToNot(HaveOccurred())

				// Update it with new TrustedCertSecrets
				cr.Spec.TrustedCertSecrets = test.NewContainerJFRWithSecrets().Spec.TrustedCertSecrets
				err = client.Update(context.Background(), cr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := controller.Reconcile(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				deployment := &appsv1.Deployment{}
				err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := test.NewVolumesWithSecrets()
				Expect(&volumes).To(Equal(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewVolumeMountsWithSecrets()
				Expect(&volumeMounts).To(Equal(expectedVolumeMounts))
			})
		})
		Context("on OpenShift", func() {
			BeforeEach(func() {
				objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			JustBeforeEach(func() {
				reconcileFully(client, controller, false)
			})
			It("should create ConsoleLink", func() {
				links := &consolev1.ConsoleLinkList{}
				err := client.List(context.Background(), links, &ctrlclient.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"rhjmc.redhat.com/containerjfr-consolelink-namespace": "default",
						"rhjmc.redhat.com/containerjfr-consolelink-name":      "containerjfr",
					}),
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(links.Items).To(HaveLen(1))
				link := links.Items[0]
				Expect(link.Spec.Text).To(Equal("Container JDK Flight Recorder"))
				Expect(link.Spec.Href).To(Equal("https://test"))
				// Should be added to the NamespaceDashboard for only the current namespace
				Expect(link.Spec.Location).To(Equal(consolev1.NamespaceDashboard))
				Expect(link.Spec.NamespaceDashboard.Namespaces).To(Equal([]string{"default"}))
			})
			It("should add the finalizer", func() {
				cr := &rhjmcv1beta1.ContainerJFR{}
				err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
				Expect(err).ToNot(HaveOccurred())
				Expect(cr.GetFinalizers()).To(ContainElement("containerjfr.finalizer.rhjmc.redhat.com"))
			})
			Context("when deleted", func() {
				JustBeforeEach(func() {
					// Simulate deletion by setting DeletionTimestamp
					cr := &rhjmcv1beta1.ContainerJFR{}
					err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
					Expect(err).ToNot(HaveOccurred())

					delTime := metav1.Unix(0, 1598045501618*int64(time.Millisecond))
					cr.DeletionTimestamp = &delTime
					err = client.Update(context.Background(), cr)
					Expect(err).ToNot(HaveOccurred())

					// Reconcile again
					req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
					result, err := controller.Reconcile(req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(reconcile.Result{}))
				})
				It("should delete the ConsoleLink", func() {
					links := &consolev1.ConsoleLinkList{}
					err := client.List(context.Background(), links, &ctrlclient.ListOptions{
						LabelSelector: labels.SelectorFromSet(labels.Set{
							"rhjmc.redhat.com/containerjfr-consolelink-namespace": "default",
							"rhjmc.redhat.com/containerjfr-consolelink-name":      "containerjfr",
						}),
					})
					Expect(err).ToNot(HaveOccurred())
					Expect(links.Items).To(BeEmpty())
				})
				It("should remove the finalizer", func() {
					cr := &rhjmcv1beta1.ContainerJFR{}
					err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
					Expect(err).ToNot(HaveOccurred())
					Expect(cr.GetFinalizers()).ToNot(ContainElement("containerjfr.finalizer.rhjmc.redhat.com"))
				})
			})
		})
	})
})

func newFakeSecret(name string) *corev1.Secret {
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

func newFakeServiceSpecs(minimal bool) resource_definitions.ServiceSpecs {
	grafanaUrl := "https://test"
	if minimal {
		grafanaUrl = ""
	}
	return resource_definitions.ServiceSpecs{
		CoreHostname:    "test",
		CommandHostname: "test",
		GrafanaURL:      grafanaUrl,
	}
}

func newFakeTLSConfig() resource_definitions.TLSConfig {
	return resource_definitions.TLSConfig{
		ContainerJFRSecret: "containerjfr-tls",
		GrafanaSecret:      "containerjfr-grafana-tls",
		KeystorePassSecret: "containerjfr-keystore",
	}
}

func makeCertificatesReady(client ctrlclient.Client) {
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

func initializeSecrets(client ctrlclient.Client) {
	// Create secrets
	secretNames := []string{"containerjfr-ca", "containerjfr-tls", "containerjfr-grafana-tls"}
	for _, secretName := range secretNames {
		secret := newFakeSecret(secretName)
		err := client.Create(context.Background(), secret)
		Expect(err).ToNot(HaveOccurred())
	}
}

func ingressConfig(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, req reconcile.Request, minimal bool) {
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

func reconcileFully(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	makeCertificatesReady(client)
	initializeSecrets(client)

	// Add ingress config to routes
	result, err = controller.Reconcile(req)
	ingressConfig(client, controller, req, minimal)

	result, err = controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}

func checkMetadata(object metav1.Object, expected metav1.Object) {
	Expect(object.GetName()).To(Equal(expected.GetName()))
	Expect(object.GetNamespace()).To(Equal(expected.GetNamespace()))
	Expect(object.GetLabels()).To(Equal(expected.GetLabels()))
	Expect(object.GetAnnotations()).To(Equal(expected.GetAnnotations()))
	ownerReferences := object.GetOwnerReferences()
	Expect(ownerReferences[0].Kind).To(Equal("ContainerJFR"))
	Expect(ownerReferences[0].Name).To(Equal("containerjfr"))
}

func expectCertificates(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR) {
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
}

func expectRoutes(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := controller.Reconcile(req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	makeCertificatesReady(client)
	initializeSecrets(client)

	// Check for routes, ingress configuration needs to be added as each
	// one is created so that they all reconcile successfully
	result, err = controller.Reconcile(req)
	ingressConfig(client, controller, req, minimal)
}

func expectPVC(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	reconcileFully(client, controller, minimal)

	err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	expectedPvc := resource_definitions.NewPersistentVolumeClaimForCR(test.NewContainerJFR())
	checkMetadata(pvc, expectedPvc)
	Expect(pvc.Spec.AccessModes).To(Equal(expectedPvc.Spec.AccessModes))
	Expect(pvc.Spec.StorageClassName).To(Equal(expectedPvc.Spec.StorageClassName))

	pvcStorage := pvc.Spec.Resources.Requests["storage"]
	expectedPvcStorage := expectedPvc.Spec.Resources.Requests["storage"]
	Expect(pvcStorage.Equal(expectedPvcStorage)).To(BeTrue())
}

func expectJMXSecret(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	secret := &corev1.Secret{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-jmx-auth", Namespace: "default"}, secret)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	reconcileFully(client, controller, minimal)

	err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-jmx-auth", Namespace: "default"}, secret)
	Expect(err).ToNot(HaveOccurred())

	expectedSecret := resource_definitions.NewJmxSecretForCR(test.NewContainerJFR())
	checkMetadata(secret, expectedSecret)
	Expect(secret.StringData["CONTAINER_JFR_RJMX_USER"]).To(Equal(expectedSecret.StringData["CONTAINER_JFR_RJMX_USER"]))
}

func expectExporterService(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	service := &corev1.Service{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	reconcileFully(client, controller, minimal)

	err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewExporterService(test.NewContainerJFR())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func expectCommandChannel(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	service := &corev1.Service{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	reconcileFully(client, controller, minimal)

	err = client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewCommandChannelService(test.NewContainerJFR())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func expectDeployment(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	deployment := &appsv1.Deployment{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	reconcileFully(client, controller, minimal)
	checkDeployment(client, minimal)
}

func expectIdempotence(client ctrlclient.Client, controller *containerjfr.ReconcileContainerJFR, minimal bool) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	reconcileFully(client, controller, minimal)

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
}

func checkGrafanaService(client ctrlclient.Client) {
	service := &corev1.Service{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewGrafanaService(test.NewContainerJFR())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func checkDeployment(client ctrlclient.Client, minimal bool) {
	deployment := &appsv1.Deployment{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	testSpecs := newFakeServiceSpecs(minimal)
	testTLSConfig := newFakeTLSConfig()
	testContainer := &rhjmcv1beta1.ContainerJFR{}
	if minimal {
		testContainer = test.NewMinimalContainerJFR()
	} else {
		testContainer = test.NewContainerJFR()
	}
	expectedDeployment := resource_definitions.NewDeploymentForCR(testContainer, &testSpecs, &testTLSConfig)
	checkMetadata(deployment, expectedDeployment)
	Expect(deployment.Spec.Selector).To(Equal(expectedDeployment.Spec.Selector))

	// compare Pod template
	template := deployment.Spec.Template
	expectedTemplate := expectedDeployment.Spec.Template
	Expect(template.ObjectMeta).To(Equal(expectedTemplate.ObjectMeta))
	Expect(template.Spec.Containers).To(Equal(expectedTemplate.Spec.Containers))
	Expect(template.Spec.Volumes).To(Equal(expectedTemplate.Spec.Volumes))
}
