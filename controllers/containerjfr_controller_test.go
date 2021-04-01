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

package controllers_test

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
	netv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/api/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/controllers"
	"github.com/rh-jmc-team/container-jfr-operator/controllers/common/resource_definitions"
	"github.com/rh-jmc-team/container-jfr-operator/test"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type containerJFRTestInput struct {
	controller *controllers.ContainerJFRReconciler
	objs       []runtime.Object
	minimal    bool
	test.TestReconcilerConfig
}

var _ = Describe("ContainerjfrController", func() {
	var t *containerJFRTestInput

	JustBeforeEach(func() {
		logger := zap.New()
		logf.SetLogger(logger)
		s := test.NewTestScheme()

		t.Client = fake.NewFakeClientWithScheme(s, t.objs...)
		t.controller = &controllers.ContainerJFRReconciler{
			Client:        t.Client,
			Scheme:        s,
			IsOpenShift:   true,
			Log:           logger,
			ReconcilerTLS: test.NewTestReconcilerTLS(&t.TestReconcilerConfig),
		}
	})

	BeforeEach(func() {
		t = &containerJFRTestInput{}
	})

	AfterEach(func() {
		t = nil
	})

	Describe("reconciling a request in OpenShift", func() {
		Context("succesfully creates required resources", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create routes", func() {
				t.expectRoutes()
			})
			It("should create persistent volume claim and set owner", func() {
				t.expectPVC(test.NewDefaultPVC())
			})
			It("should create Grafana secret and set owner", func() {
				secret := &corev1.Secret{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana-basic", Namespace: "default"}, secret)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				t.reconcileContainerJFRFully()

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana-basic", Namespace: "default"}, secret)
				Expect(err).ToNot(HaveOccurred())

				// Compare to desired spec
				expectedSecret := resource_definitions.NewGrafanaSecretForCR(test.NewContainerJFR())
				checkMetadata(secret, expectedSecret)
				Expect(secret.StringData["GF_SECURITY_ADMIN_USER"]).To(Equal(expectedSecret.StringData["GF_SECURITY_ADMIN_USER"]))
			})
			It("should create JMX secret and set owner", func() {
				t.expectJMXSecret()
			})
			It("should create Grafana service and set owner", func() {
				service := &corev1.Service{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				t.reconcileContainerJFRFully()
				t.checkGrafanaService()
			})
			It("should create exporter service and set owner", func() {
				t.expectExporterService()
			})
			It("should set ApplicationURL in CR Status", func() {
				t.expectStatusApplicationURL()
			})
			It("should create command channel service and set owner", func() {
				t.expectCommandChannel()
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
		})
		Context("succesfully creates required resources for minimal deployment", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
				t.minimal = true
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create routes", func() {
				t.expectRoutes()
			})
			It("should create persistent volume claim and set owner", func() {
				t.expectPVC(test.NewDefaultPVC())
			})
			It("should create JMX secret and set owner", func() {
				t.expectJMXSecret()
			})
			It("should create exporter service and set owner", func() {
				t.expectExporterService()
			})
			It("should set ApplicationURL in CR Status", func() {
				t.expectStatusApplicationURL()
			})
			It("should create command channel service and set owner", func() {
				t.expectCommandChannel()
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
		})
		Context("after containerjfr reconciled successfully", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should be idempotent", func() {
				t.expectIdempotence()
			})
		})
		Context("After a minimal containerjfr reconciled successfully", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
				t.minimal = true
			})
			It("should be idempotent", func() {
				t.expectIdempotence()
			})
		})
		Context("ContainerJFR does not exist", func() {
			It("Should do nothing", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("Switching from a minimal to a non-minimal deployment", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewMinimalContainerJFR(),
				}
				t.minimal = true
			})
			JustBeforeEach(func() {
				t.reconcileContainerJFRFully()

				cjfr := &rhjmcv1beta1.ContainerJFR{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cjfr)
				Expect(err).ToNot(HaveOccurred())

				t.minimal = false
				cjfr.Spec.Minimal = false
				err = t.Client.Status().Update(context.Background(), cjfr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
				Expect(err).ToNot(HaveOccurred())
				t.ingressConfig(req)
				result, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should create grafana resources", func() {
				t.checkGrafanaService()
			})
			It("should configure deployment appropriately", func() {
				t.checkDeployment()
			})
		})
		Context("Switching from a non-minimal to a minimal deployment", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			JustBeforeEach(func() {
				t.reconcileContainerJFRFully()

				cjfr := &rhjmcv1beta1.ContainerJFR{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cjfr)
				Expect(err).ToNot(HaveOccurred())

				t.minimal = true
				cjfr.Spec.Minimal = true
				err = t.Client.Status().Update(context.Background(), cjfr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				_, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should delete grafana resources", func() {
				service := &corev1.Service{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				route := &openshiftv1.Route{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, route)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
			It("should configure deployment appropriately", func() {
				t.checkDeployment()
			})
		})
		Context("Container jfr has list of certificate secrets", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithSecrets(), newFakeSecret("testCert1"), newFakeSecret("testCert2"),
				}
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.reconcileContainerJFRFully()
				deployment := &appsv1.Deployment{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := test.NewVolumesWithSecrets()
				Expect(volumes).To(Equal(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewVolumeMountsWithSecrets()
				Expect(volumeMounts).To(Equal(expectedVolumeMounts))
			})
		})
		Context("Adding a certificate to the TrustedCertSecrets list", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(), newFakeSecret("testCert1"), newFakeSecret("testCert2"),
				}
			})
			JustBeforeEach(func() {
				t.reconcileContainerJFRFully()
			})
			It("Should update the corresponding deployment", func() {
				// Get ContainerJFR CR after reconciling
				cr := &rhjmcv1beta1.ContainerJFR{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
				Expect(err).ToNot(HaveOccurred())

				// Update it with new TrustedCertSecrets
				cr.Spec.TrustedCertSecrets = test.NewContainerJFRWithSecrets().Spec.TrustedCertSecrets
				err = t.Client.Update(context.Background(), cr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				deployment := &appsv1.Deployment{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := test.NewVolumesWithSecrets()
				Expect(volumes).To(Equal(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewVolumeMountsWithSecrets()
				Expect(volumeMounts).To(Equal(expectedVolumeMounts))
			})
		})
		Context("with custom PVC spec overriding all defaults", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithPVCSpec(),
				}
			})
			It("should create the PVC with requested spec", func() {
				t.expectPVC(test.NewCustomPVC())
			})
		})
		Context("with custom PVC spec overriding some defaults", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithPVCSpecSomeDefault(),
				}
			})
			It("should create the PVC with requested spec", func() {
				t.expectPVC(test.NewCustomPVCSomeDefault())
			})
		})
		Context("with custom PVC config with no spec", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithPVCLabelsOnly(),
				}
			})
			It("should create the PVC with requested label", func() {
				t.expectPVC(test.NewDefaultPVCWithLabel())
			})
		})
		Context("with overriden image tags", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
				coreImg := "my/core-image:1.0"
				datasourceImg := "my/datasource-image:1.0"
				grafanaImg := "my/grafana-image:1.0"
				t.CoreImageTag = &coreImg
				t.DatasourceImageTag = &datasourceImg
				t.GrafanaImageTag = &grafanaImg
			})
			It("should create deployment with the expected tags", func() {
				t.expectDeployment()
			})
		})
		Context("on OpenShift", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			JustBeforeEach(func() {
				t.reconcileContainerJFRFully()
			})
			It("should create ConsoleLink", func() {
				links := &consolev1.ConsoleLinkList{}
				err := t.Client.List(context.Background(), links, &ctrlclient.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"rhjmc.redhat.com/containerjfr-consolelink-namespace": "default",
						"rhjmc.redhat.com/containerjfr-consolelink-name":      "containerjfr",
					}),
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(links.Items).To(HaveLen(1))
				link := links.Items[0]
				Expect(link.Spec.Text).To(Equal("Container JDK Flight Recorder"))
				Expect(link.Spec.Href).To(Equal("https://containerjfr.example.com"))
				// Should be added to the NamespaceDashboard for only the current namespace
				Expect(link.Spec.Location).To(Equal(consolev1.NamespaceDashboard))
				Expect(link.Spec.NamespaceDashboard.Namespaces).To(Equal([]string{"default"}))
			})
			It("should add the finalizer", func() {
				cr := &rhjmcv1beta1.ContainerJFR{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
				Expect(err).ToNot(HaveOccurred())
				Expect(cr.GetFinalizers()).To(ContainElement("rhjmc.redhat.com/containerjfr.finalizer"))
			})
			Context("when deleted", func() {
				JustBeforeEach(func() {
					// Simulate deletion by setting DeletionTimestamp
					cr := &rhjmcv1beta1.ContainerJFR{}
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
					Expect(err).ToNot(HaveOccurred())

					delTime := metav1.Unix(0, 1598045501618*int64(time.Millisecond))
					cr.DeletionTimestamp = &delTime
					err = t.Client.Update(context.Background(), cr)
					Expect(err).ToNot(HaveOccurred())

					// Reconcile again
					req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
					result, err := t.controller.Reconcile(context.Background(), req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(reconcile.Result{}))
				})
				It("should delete the ConsoleLink", func() {
					links := &consolev1.ConsoleLinkList{}
					err := t.Client.List(context.Background(), links, &ctrlclient.ListOptions{
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
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
					Expect(err).ToNot(HaveOccurred())
					Expect(cr.GetFinalizers()).ToNot(ContainElement("rhjmc.redhat.com/containerjfr.finalizer"))
				})
			})
		})
		Context("with service TLS disabled", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
				disableTLS := true
				t.DisableTLS = &disableTLS
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
			It("should not create certificates", func() {
				certs := &certv1.CertificateList{}
				t.Client.List(context.Background(), certs, &ctrlclient.ListOptions{
					Namespace: "default",
				})
				Expect(certs.Items).To(BeEmpty())
			})
			It("should create routes with edge TLS termination", func() {
				t.expectRoutes()
			})
		})
	})
	Describe("reconciling a request in Kubernetes", func() {
		JustBeforeEach(func() {
			t.controller.IsOpenShift = false
		})
		Context("succesfully creates required resources", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithIngress(),
				}
			})
			It("should create ingresses", func() {
				t.expectIngresses()
			})
			It("should not create routes", func() {
				t.reconcileContainerJFRFully()
				t.expectNoRoutes()
			})
		})
		Context("no ingress configuration is provided", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFR(),
				}
			})
			It("should not create ingresses or routes", func() {
				t.reconcileContainerJFRFully()
				t.expectNoIngresses()
				t.expectNoRoutes()
			})
		})
		Context("networkConfig for one of the services is nil", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithIngress(),
				}
			})
			It("should only create specified ingresses", func() {
				c := &rhjmcv1beta1.ContainerJFR{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, c)
				Expect(err).ToNot(HaveOccurred())
				c.Spec.NetworkOptions.CommandConfig = nil
				err = t.Client.Update(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				t.reconcileContainerJFRFully()
				expectedConfig := test.NewNetworkConfigurationList()

				ingress := &netv1.Ingress{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.ExporterConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.ExporterConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.ExporterConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, ingress)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("ingressSpec for one of the services is nil", func() {
			BeforeEach(func() {
				t.objs = []runtime.Object{
					test.NewContainerJFRWithIngress(),
				}
			})
			It("should only create specified ingresses", func() {
				c := &rhjmcv1beta1.ContainerJFR{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, c)
				Expect(err).ToNot(HaveOccurred())
				c.Spec.NetworkOptions.ExporterConfig.IngressSpec = nil
				err = t.Client.Update(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				t.reconcileContainerJFRFully()
				expectedConfig := test.NewNetworkConfigurationList()

				ingress := &netv1.Ingress{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.CommandConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.CommandConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.CommandConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, ingress)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
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
			corev1.TLSCertKey: []byte(name + "-bytes"),
		},
	}
}

func (t *containerJFRTestInput) makeCertificatesReady() {
	certNames := []string{"containerjfr", "containerjfr-ca", "containerjfr-grafana"}
	for _, certName := range certNames {
		cert := &certv1.Certificate{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: certName, Namespace: "default"}, cert)
		Expect(err).ToNot(HaveOccurred())
		cert.Status.Conditions = append(cert.Status.Conditions, certv1.CertificateCondition{
			Type:   certv1.CertificateConditionReady,
			Status: certMeta.ConditionTrue,
		})
		err = t.Client.Status().Update(context.Background(), cert)
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *containerJFRTestInput) initializeSecrets() {
	// Create secrets
	secretNames := []string{"containerjfr-ca", "containerjfr-tls", "containerjfr-grafana-tls"}
	for _, secretName := range secretNames {
		secret := newFakeSecret(secretName)
		err := t.Client.Create(context.Background(), secret)
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *containerJFRTestInput) ingressConfig(req reconcile.Request) {
	routes := []string{"containerjfr", "containerjfr-command"}
	if !t.minimal {
		routes = append([]string{"containerjfr-grafana"}, routes...)
	}
	for _, routeName := range routes {
		route := &openshiftv1.Route{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: routeName, Namespace: "default"}, route)
		Expect(err).ToNot(HaveOccurred())

		// Verify the TLS termination policy
		Expect(route.Spec.TLS).ToNot(BeNil())
		if t.DisableTLS != nil && *t.DisableTLS {
			Expect(route.Spec.TLS.Termination).To(Equal(openshiftv1.TLSTerminationEdge))
			Expect(route.Spec.TLS.InsecureEdgeTerminationPolicy).To(Equal(openshiftv1.InsecureEdgeTerminationPolicyRedirect))
		} else {
			Expect(route.Spec.TLS.Termination).To(Equal(openshiftv1.TLSTerminationReencrypt))
			Expect(route.Spec.TLS.DestinationCACertificate).To(Equal("containerjfr-ca-bytes"))
		}
		route.Status.Ingress = append(route.Status.Ingress, openshiftv1.RouteIngress{
			Host: routeName + ".example.com",
		})
		err = t.Client.Status().Update(context.Background(), route)
		Expect(err).ToNot(HaveOccurred())
		_, err = t.controller.Reconcile(context.Background(), req)
	}
}

func (t *containerJFRTestInput) reconcileContainerJFRFully() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	if t.DisableTLS == nil || !*t.DisableTLS {
		t.makeCertificatesReady()
		t.initializeSecrets()
	}

	// Add ingress config to routes
	if t.controller.IsOpenShift {
		result, err = t.controller.Reconcile(context.Background(), req)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
		t.ingressConfig(req)
	}

	result, err = t.controller.Reconcile(context.Background(), req)
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

func (t *containerJFRTestInput) expectCertificates() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
	certNames := []string{"containerjfr", "containerjfr-ca", "containerjfr-grafana"}
	for _, certName := range certNames {
		cert := &certv1.Certificate{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: certName, Namespace: "default"}, cert)
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *containerJFRTestInput) expectRoutes() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	if t.DisableTLS == nil || !*t.DisableTLS {
		t.makeCertificatesReady()
		t.initializeSecrets()
	}

	// Check for routes, ingress configuration needs to be added as each
	// one is created so that they all reconcile successfully
	result, err = t.controller.Reconcile(context.Background(), req)
	t.ingressConfig(req)
}

func (t *containerJFRTestInput) expectNoRoutes() {
	svc := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *containerJFRTestInput) expectIngresses() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	t.makeCertificatesReady()
	t.initializeSecrets()

	result, err = t.controller.Reconcile(context.Background(), req)
	expectedConfig := test.NewNetworkConfigurationList()

	ingress := &netv1.Ingress{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.ExporterConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.ExporterConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.ExporterConfig.IngressSpec))

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.CommandConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.CommandConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.CommandConfig.IngressSpec))

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))
}

func (t *containerJFRTestInput) expectNoIngresses() {
	ing := &netv1.Ingress{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *containerJFRTestInput) expectPVC(expectedPvc *corev1.PersistentVolumeClaim) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileContainerJFRFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, pvc)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	checkMetadata(pvc, expectedPvc)
	Expect(pvc.Spec.AccessModes).To(Equal(expectedPvc.Spec.AccessModes))
	Expect(pvc.Spec.StorageClassName).To(Equal(expectedPvc.Spec.StorageClassName))

	pvcStorage := pvc.Spec.Resources.Requests["storage"]
	expectedPvcStorage := expectedPvc.Spec.Resources.Requests["storage"]
	Expect(pvcStorage.Equal(expectedPvcStorage)).To(BeTrue())
}

func (t *containerJFRTestInput) expectJMXSecret() {
	secret := &corev1.Secret{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-jmx-auth", Namespace: "default"}, secret)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileContainerJFRFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-jmx-auth", Namespace: "default"}, secret)
	Expect(err).ToNot(HaveOccurred())

	expectedSecret := resource_definitions.NewJmxSecretForCR(test.NewContainerJFR())
	checkMetadata(secret, expectedSecret)
	Expect(secret.StringData["CONTAINER_JFR_RJMX_USER"]).To(Equal(expectedSecret.StringData["CONTAINER_JFR_RJMX_USER"]))
}

func (t *containerJFRTestInput) expectExporterService() {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileContainerJFRFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewExporterService(test.NewContainerJFR())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func (t *containerJFRTestInput) expectStatusApplicationURL() {
	instance := &rhjmcv1beta1.ContainerJFR{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, instance)

	t.reconcileContainerJFRFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, instance)
	Expect(err).ToNot(HaveOccurred())

	Expect(instance.Status.ApplicationURL).To(Equal("https://containerjfr.example.com"))
}

func (t *containerJFRTestInput) expectCommandChannel() {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileContainerJFRFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-command", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewCommandChannelService(test.NewContainerJFR())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func (t *containerJFRTestInput) expectDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileContainerJFRFully()
	t.checkDeployment()
}

func (t *containerJFRTestInput) expectIdempotence() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "containerjfr", Namespace: "default"}}
	t.reconcileContainerJFRFully()

	obj := &rhjmcv1beta1.ContainerJFR{}
	err := t.Client.Get(context.Background(), req.NamespacedName, obj)
	Expect(err).ToNot(HaveOccurred())

	// Reconcile again
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))

	obj2 := &rhjmcv1beta1.ContainerJFR{}
	err = t.Client.Get(context.Background(), req.NamespacedName, obj2)
	Expect(err).ToNot(HaveOccurred())
	Expect(obj2.Status).To(Equal(obj.Status))
	Expect(obj2.Spec).To(Equal(obj.Spec))
}

func (t *containerJFRTestInput) checkGrafanaService() {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr-grafana", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewGrafanaService(test.NewContainerJFR())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func (t *containerJFRTestInput) checkDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	cr := &rhjmcv1beta1.ContainerJFR{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "containerjfr", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())

	Expect(deployment.Name).To(Equal("containerjfr"))
	Expect(deployment.Namespace).To(Equal("default"))
	Expect(deployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to": "container-jfr-operator",
	}))
	Expect(deployment.Labels).To(Equal(map[string]string{
		"app":                    "containerjfr",
		"kind":                   "containerjfr",
		"app.kubernetes.io/name": "container-jfr",
	}))
	Expect(metav1.IsControlledBy(deployment, cr)).To(BeTrue())
	Expect(deployment.Spec.Selector).To(Equal(test.NewDeploymentSelector()))

	// compare Pod template
	tls := t.DisableTLS == nil || !*t.DisableTLS
	template := deployment.Spec.Template
	Expect(template.Name).To(Equal("containerjfr"))
	Expect(template.Namespace).To(Equal("default"))
	Expect(template.Labels).To(Equal(map[string]string{
		"app":  "containerjfr",
		"kind": "containerjfr",
	}))
	Expect(template.Spec.Volumes).To(Equal(test.NewVolumes(t.minimal, tls)))

	// Check that the networking environment variables are set correctly
	coreContainer := template.Spec.Containers[0]
	checkCoreContainer(&coreContainer, t.minimal, tls, t.CoreImageTag)

	if !t.minimal {
		// Check that Grafana is configured properly, depending on the environment
		grafanaContainer := template.Spec.Containers[1]
		checkGrafanaContainer(&grafanaContainer, tls, t.GrafanaImageTag)

		// Check that JFR Datasource is configured properly
		datasourceContainer := template.Spec.Containers[2]
		checkDatasourceContainer(&datasourceContainer, t.DatasourceImageTag)
	}
}

func checkCoreContainer(container *corev1.Container, minimal bool, tls bool, tag *string) {
	Expect(container.Name).To(Equal("containerjfr"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/rh-jmc-team/container-jfr:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewCorePorts()))
	Expect(container.Env).To(ConsistOf(test.NewCoreEnvironmentVariables(minimal, tls)))
	Expect(container.EnvFrom).To(ConsistOf(test.NewCoreEnvFromSource(tls)))
	Expect(container.VolumeMounts).To(ConsistOf(test.NewCoreVolumeMounts(tls)))
	Expect(container.LivenessProbe).To(Equal(test.NewCoreLivenessProbe(tls)))
	Expect(container.StartupProbe).To(Equal(test.NewCoreStartupProbe(tls)))
}

func checkGrafanaContainer(container *corev1.Container, tls bool, tag *string) {
	Expect(container.Name).To(Equal("containerjfr-grafana"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/rh-jmc-team/container-jfr-grafana-dashboard:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewGrafanaPorts()))
	Expect(container.Env).To(ConsistOf(test.NewGrafanaEnvironmentVariables(tls)))
	Expect(container.EnvFrom).To(ConsistOf(test.NewGrafanaEnvFromSource()))
	Expect(container.VolumeMounts).To(ConsistOf(test.NewGrafanaVolumeMounts(tls)))
	Expect(container.LivenessProbe).To(Equal(test.NewGrafanaLivenessProbe(tls)))
}

func checkDatasourceContainer(container *corev1.Container, tag *string) {
	Expect(container.Name).To(Equal("containerjfr-jfr-datasource"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/rh-jmc-team/jfr-datasource:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewDatasourcePorts()))
	Expect(container.Env).To(ConsistOf(test.NewDatasourceEnvironmentVariables()))
	Expect(container.EnvFrom).To(BeEmpty())
	Expect(container.VolumeMounts).To(BeEmpty())
	Expect(container.LivenessProbe).To(Equal(test.NewDatasourceLivenessProbe()))
}
