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

package controllers_test

import (
	"context"
	"time"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type cryostatTestInput struct {
	controller  *controllers.CryostatReconciler
	objs        []runtime.Object
	minimal     bool
	externalTLS bool
	test.TestReconcilerConfig
}

var _ = Describe("CryostatController", func() {
	var t *cryostatTestInput

	JustBeforeEach(func() {
		logger := zap.New()
		logf.SetLogger(logger)
		s := test.NewTestScheme()

		t.Client = fake.NewFakeClientWithScheme(s, t.objs...)
		t.controller = &controllers.CryostatReconciler{
			Client:        t.Client,
			Scheme:        s,
			IsOpenShift:   true,
			EventRecorder: record.NewFakeRecorder(1024),
			RESTMapper:    test.NewTESTRESTMapper(),
			Log:           logger,
			ReconcilerTLS: test.NewTestReconcilerTLS(&t.TestReconcilerConfig),
		}
	})

	BeforeEach(func() {
		t = &cryostatTestInput{
			TestReconcilerConfig: test.TestReconcilerConfig{
				TLS: true,
			},
			externalTLS: true,
		}
		t.objs = []runtime.Object{
			test.NewNamespace(),
			test.NewApiServer(),
		}
	})

	AfterEach(func() {
		t = nil
	})

	Describe("reconciling a request in OpenShift", func() {
		Context("successfully creates required resources", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create RBAC", func() {
				t.expectRBAC()
			})
			It("should create routes", func() {
				t.expectRoutes()
			})
			It("should create persistent volume claim and set owner", func() {
				t.expectPVC(test.NewDefaultPVC())
			})
			It("should create Grafana secret and set owner", func() {
				secret := &corev1.Secret{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana-basic", Namespace: "default"}, secret)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				t.reconcileCryostatFully()

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana-basic", Namespace: "default"}, secret)
				Expect(err).ToNot(HaveOccurred())

				// Compare to desired spec
				expectedSecret := resource_definitions.NewGrafanaSecretForCR(test.NewCryostat())
				checkMetadata(secret, expectedSecret)
				Expect(secret.StringData["GF_SECURITY_ADMIN_USER"]).To(Equal(expectedSecret.StringData["GF_SECURITY_ADMIN_USER"]))
			})
			It("should create JMX secret and set owner", func() {
				t.expectJMXSecret()
			})
			It("should create Grafana service and set owner", func() {
				service := &corev1.Service{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, service)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				t.reconcileCryostatFully()
				t.checkGrafanaService()
			})
			It("should create core service and set owner", func() {
				t.expectCoreService()
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
				t.objs = append(t.objs, test.NewMinimalCryostat())
				t.minimal = true
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create RBAC", func() {
				t.expectRBAC()
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
			It("should create core service and set owner", func() {
				t.expectCoreService()
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
		Context("after cryostat reconciled successfully", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
			})
			It("should be idempotent", func() {
				t.expectIdempotence()
			})
		})
		Context("After a minimal cryostat reconciled successfully", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewMinimalCryostat())
				t.minimal = true
			})
			It("should be idempotent", func() {
				t.expectIdempotence()
			})
		})
		Context("Cryostat does not exist", func() {
			It("Should do nothing", func() {
				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("Switching from a minimal to a non-minimal deployment", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewMinimalCryostat())
				t.minimal = true
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.minimal = false
				cryostat.Spec.Minimal = false
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
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
				t.objs = append(t.objs, test.NewCryostat())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.minimal = true
				cryostat.Spec.Minimal = true
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				_, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should delete grafana resources", func() {
				service := &corev1.Service{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, service)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())

				route := &openshiftv1.Route{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, route)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
			It("should configure deployment appropriately", func() {
				t.checkDeployment()
			})
		})
		Context("Cryostat CR has list of certificate secrets", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithSecrets(),
					newFakeSecret("testCert1"), newFakeSecret("testCert2"))
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.reconcileCryostatFully()
				deployment := &appsv1.Deployment{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := test.NewVolumesWithSecrets()
				Expect(volumes).To(Equal(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewCoreVolumeMounts(t.TLS)
				Expect(volumeMounts).To(Equal(expectedVolumeMounts))
			})
		})
		Context("Adding a certificate to the TrustedCertSecrets list", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat(), newFakeSecret("testCert1"),
					newFakeSecret("testCert2"))
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should update the corresponding deployment", func() {
				// Get Cryostat CR after reconciling
				cr := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
				Expect(err).ToNot(HaveOccurred())

				// Update it with new TrustedCertSecrets
				cr.Spec.TrustedCertSecrets = test.NewCryostatWithSecrets().Spec.TrustedCertSecrets
				err = t.Client.Update(context.Background(), cr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				deployment := &appsv1.Deployment{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := test.NewVolumesWithSecrets()
				Expect(volumes).To(Equal(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewCoreVolumeMounts(t.TLS)
				Expect(volumeMounts).To(Equal(expectedVolumeMounts))
			})
		})
		Context("Cryostat CR has list of event templates", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithTemplates(), test.NewTemplateConfigMap(),
					test.NewOtherTemplateConfigMap())
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.reconcileCryostatFully()
				t.checkDeploymentHasTemplates()
			})
		})
		Context("Cryostat CR has list of event templates with TLS disabled", func() {
			BeforeEach(func() {
				certManager := false
				cr := test.NewCryostatWithTemplates()
				cr.Spec.EnableCertManager = &certManager
				t.objs = append(t.objs, cr, test.NewTemplateConfigMap(),
					test.NewOtherTemplateConfigMap())
				t.TLS = false
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.reconcileCryostatFully()
				t.checkDeploymentHasTemplates()
			})
		})
		Context("Adding a template to the EventTemplates list", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat(), test.NewTemplateConfigMap(),
					test.NewOtherTemplateConfigMap())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should update the corresponding deployment", func() {
				// Get Cryostat CR after reconciling
				cr := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
				Expect(err).ToNot(HaveOccurred())

				// Update it with new EventTemplates
				cr.Spec.EventTemplates = test.NewCryostatWithTemplates().Spec.EventTemplates
				err = t.Client.Update(context.Background(), cr)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))

				t.checkDeploymentHasTemplates()
			})
		})
		Context("with custom PVC spec overriding all defaults", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithPVCSpec())
			})
			It("should create the PVC with requested spec", func() {
				t.expectPVC(test.NewCustomPVC())
			})
		})
		Context("with custom PVC spec overriding some defaults", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithPVCSpecSomeDefault())
			})
			It("should create the PVC with requested spec", func() {
				t.expectPVC(test.NewCustomPVCSomeDefault())
			})
		})
		Context("with custom PVC config with no spec", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithPVCLabelsOnly())
			})
			It("should create the PVC with requested label", func() {
				t.expectPVC(test.NewDefaultPVCWithLabel())
			})
		})
		Context("with overriden image tags", func() {
			var deploy *appsv1.Deployment
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
				deploy = &appsv1.Deployment{}
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deploy)
				Expect(err).ToNot(HaveOccurred())
			})
			Context("for development", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:1.0.0-SNAPSHOT"
					datasourceImg := "my/datasource-image:1.0.0-BETA25"
					grafanaImg := "my/grafana-image:1.0.0-dev"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkDeployment()
				})
				It("should set ImagePullPolicy to Always", func() {
					containers := deploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullAlways))
					}
				})
			})
			Context("for release", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:1.0.0"
					datasourceImg := "my/datasource-image:1.0.0"
					grafanaImg := "my/grafana-image:1.0.0"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkDeployment()
				})
				It("should set ImagePullPolicy to IfNotPresent", func() {
					containers := deploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
					}
				})
			})
			Context("by digest", func() {
				BeforeEach(func() {
					coreImg := "my/core-image@sha256:99b57e9b8880bc5d4d799b508603628c37c3e6a0d4bdd0988e9dc3ad8e04c495"
					datasourceImg := "my/datasource-image@sha256:59ded87392077c2371b26e021aade0409855b597383fa78e549eefafab8fc90c"
					grafanaImg := "my/grafana-image@sha256:e5bc16c2c5b69cd6fd8fdf1381d0a8b6cc9e01d92b9e1bb0a61ed89196563c72"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkDeployment()
				})
				It("should set ImagePullPolicy to IfNotPresent", func() {
					containers := deploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
					}
				})
			})
			Context("with latest", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:latest"
					datasourceImg := "my/datasource-image:latest"
					grafanaImg := "my/grafana-image:latest"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkDeployment()
				})
				It("should set ImagePullPolicy to Always", func() {
					containers := deploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullAlways))
					}
				})
			})
		})
		Context("when deleted", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("ClusterRoleBinding exists", func() {
				JustBeforeEach(func() {
					t.reconcileDeletedCryostat()
				})
				It("should delete the ClusterRoleBinding", func() {
					t.checkClusterRoleBindingDeleted()
				})
				It("should remove the finalizer", func() {
					t.expectCryostatFinalizerAbsent()
				})
			})
			Context("ClusterRoleBinding does not exist", func() {
				JustBeforeEach(func() {
					err := t.Client.Delete(context.Background(), test.NewClusterRoleBinding())
					Expect(err).ToNot(HaveOccurred())
					t.reconcileDeletedCryostat()
				})
				It("should remove the finalizer", func() {
					t.expectCryostatFinalizerAbsent()
				})
			})
		})
		Context("on OpenShift", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create ConsoleLink", func() {
				link := &consolev1.ConsoleLink{}
				expectedLink := test.NewConsoleLink()
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedLink.Name}, link)
				Expect(err).ToNot(HaveOccurred())
				Expect(link.Spec).To(Equal(expectedLink.Spec))
			})
			It("should add application url to APIServer AdditionalCORSAllowedOrigins", func() {
				apiServer := &configv1.APIServer{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cluster"}, apiServer)
				Expect(err).ToNot(HaveOccurred())
				Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).To(ContainElement("https://cryostat\\.example\\.com"))
			})
			It("should add the finalizer", func() {
				t.expectCryostatFinalizerPresent()
			})
			Context("with restricted SCC", func() {
				BeforeEach(func() {
					t.objs = []runtime.Object{
						test.NewCryostat(), test.NewNamespaceWithSCCSupGroups(), test.NewApiServer(),
					}
				})
				It("should set fsGroup to value derived from namespace", func() {
					deploy := &appsv1.Deployment{}
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deploy)
					Expect(err).ToNot(HaveOccurred())
					sc := deploy.Spec.Template.Spec.SecurityContext
					Expect(sc).ToNot(BeNil())
					Expect(sc.FSGroup).ToNot(BeNil())
					Expect(*sc.FSGroup).To(Equal(int64(1000130000)))
				})
			})
			Context("when deleted", func() {
				Context("ConsoleLink exists", func() {
					JustBeforeEach(func() {
						t.reconcileDeletedCryostat()
					})
					It("should delete the ConsoleLink", func() {
						link := &consolev1.ConsoleLink{}
						expectedLink := test.NewConsoleLink()
						err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedLink.Name}, link)
						Expect(kerrors.IsNotFound(err)).To(BeTrue())
					})
					It("should remove the application url from APIServer AdditionalCORSAllowedOrigins", func() {
						apiServer := &configv1.APIServer{}
						err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cluster"}, apiServer)
						Expect(err).ToNot(HaveOccurred())
						Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).ToNot(ContainElement("https://cryostat\\.example\\.com"))
						Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).To(ContainElement("https://an-existing-user-specified\\.allowed\\.origin\\.com"))
					})
					It("should remove the finalizer", func() {
						t.expectCryostatFinalizerAbsent()
					})
				})
				Context("ConsoleLink does not exist", func() {
					JustBeforeEach(func() {
						err := t.Client.Delete(context.Background(), test.NewConsoleLink())
						Expect(err).ToNot(HaveOccurred())
						t.reconcileDeletedCryostat()
					})
					It("should remove the finalizer", func() {
						t.expectCryostatFinalizerAbsent()
					})
				})
			})
		})
		Context("with cert-manager disabled in CR", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatCertManagerDisabled())
				t.TLS = false
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
		Context("with cert-manager not configured in CR", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatCertManagerUndefined())
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create routes with re-encrypt TLS termination", func() {
				t.expectRoutes()
			})
		})
		Context("with DISABLE_SERVICE_TLS=true", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatCertManagerUndefined())
				disableTLS := true
				t.EnvDisableTLS = &disableTLS
				t.TLS = false
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
		Context("Disable cert-manager after being enabled", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.TLS = false
				certManager := false
				cryostat.Spec.EnableCertManager = &certManager
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				_, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should update the deployment", func() {
				t.checkDeployment()
			})
			It("should create routes with edge TLS termination", func() {
				t.checkRoutes()
			})
		})
		Context("Enable cert-manager after being disabled", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatCertManagerDisabled())
				t.TLS = false
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.TLS = true
				certManager := true
				cryostat.Spec.EnableCertManager = &certManager
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				_, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())

				t.makeCertificatesReady()
				t.initializeSecrets()

				_, err = t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should update the deployment", func() {
				t.checkDeployment()
			})
			It("should create certificates", func() {
				t.checkCertificates()
			})
			It("should create routes with re-encrypt TLS termination", func() {
				t.checkRoutes()
			})
		})
		Context("cert-manager missing", func() {
			JustBeforeEach(func() {
				// Replace with an empty RESTMapper
				t.controller.RESTMapper = meta.NewDefaultRESTMapper([]schema.GroupVersion{})
			})
			Context("and enabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostat())
				})
				It("should emit a CertManagerUnavailable Event", func() {
					req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
					_, err := t.controller.Reconcile(context.Background(), req)
					Expect(err).To(HaveOccurred())

					recorder := t.controller.EventRecorder.(*record.FakeRecorder)
					var eventMsg string
					Expect(recorder.Events).To(Receive(&eventMsg))
					Expect(eventMsg).To(ContainSubstring("CertManagerUnavailable"))
				})
			})
			Context("and disabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatCertManagerDisabled())
					t.TLS = false
				})
				It("should not emit a CertManagerUnavailable Event", func() {
					req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
					_, err := t.controller.Reconcile(context.Background(), req)
					Expect(err).ToNot(HaveOccurred())

					recorder := t.controller.EventRecorder.(*record.FakeRecorder)
					Expect(recorder.Events).ToNot(Receive())
				})
			})
		})
		Context("with service options", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing core config", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithCoreSvc())
				})
				It("should created the service as described", func() {
					t.checkService("cryostat", test.NewCustomizedCoreService())
				})
			})
			Context("containing grafana config", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithGrafanaSvc())
				})
				It("should created the service as described", func() {
					t.checkService("cryostat-grafana", test.NewCustomizedGrafanaService())
				})
			})
		})
	})
	Describe("reconciling a request in Kubernetes", func() {
		JustBeforeEach(func() {
			t.controller.IsOpenShift = false
		})
		Context("with TLS ingress", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithIngress())
			})
			It("should create ingresses", func() {
				t.expectIngresses()
			})
			It("should not create routes", func() {
				t.reconcileCryostatFully()
				t.expectNoRoutes()
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
		})
		Context("with non-TLS ingress", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithIngressNoTLS())
				t.externalTLS = false
			})
			It("should create ingresses", func() {
				t.expectIngresses()
			})
			It("should not create routes", func() {
				t.reconcileCryostatFully()
				t.expectNoRoutes()
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
		})
		Context("no ingress configuration is provided", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostat())
			})
			It("should not create ingresses or routes", func() {
				t.reconcileCryostatFully()
				t.expectNoIngresses()
				t.expectNoRoutes()
			})
		})
		Context("networkConfig for one of the services is nil", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithIngress())
			})
			It("should only create specified ingresses", func() {
				c := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, c)
				Expect(err).ToNot(HaveOccurred())
				c.Spec.NetworkOptions.CommandConfig = nil
				err = t.Client.Update(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				t.reconcileCryostatFully()
				expectedConfig := test.NewNetworkConfigurationList(t.externalTLS)

				ingress := &netv1.Ingress{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.CoreConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.CoreConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.CoreConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, ingress)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("ingressSpec for one of the services is nil", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithIngress())
			})
			It("should only create specified ingresses", func() {
				c := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, c)
				Expect(err).ToNot(HaveOccurred())
				c.Spec.NetworkOptions.CoreConfig.IngressSpec = nil
				err = t.Client.Update(context.Background(), c)
				Expect(err).ToNot(HaveOccurred())

				t.reconcileCryostatFully()
				expectedConfig := test.NewNetworkConfigurationList(t.externalTLS)

				ingress := &netv1.Ingress{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.CommandConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.CommandConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.CommandConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ingress)
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

func (t *cryostatTestInput) makeCertificatesReady() {
	certNames := []string{"cryostat", "cryostat-ca", "cryostat-grafana"}
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

func (t *cryostatTestInput) initializeSecrets() {
	// Create secrets
	secretNames := []string{"cryostat-ca", "cryostat-tls", "cryostat-grafana-tls"}
	for _, secretName := range secretNames {
		secret := newFakeSecret(secretName)
		err := t.Client.Create(context.Background(), secret)
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *cryostatTestInput) ingressConfig(req reconcile.Request) {
	routes := []string{"cryostat", "cryostat-command"}
	if !t.minimal {
		routes = append([]string{"cryostat-grafana"}, routes...)
	}
	for _, routeName := range routes {
		route := t.checkRoute(routeName)
		route.Status.Ingress = append(route.Status.Ingress, openshiftv1.RouteIngress{
			Host: routeName + ".example.com",
		})
		err := t.Client.Status().Update(context.Background(), route)
		Expect(err).ToNot(HaveOccurred())
		_, err = t.controller.Reconcile(context.Background(), req)
	}
}

func (t *cryostatTestInput) checkRoutes() {
	routes := []string{"cryostat", "cryostat-command"}
	if !t.minimal {
		routes = append([]string{"cryostat-grafana"}, routes...)
	}
	for _, routeName := range routes {
		t.checkRoute(routeName)
	}
}

func (t *cryostatTestInput) checkRoute(name string) *openshiftv1.Route {
	route := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, route)
	Expect(err).ToNot(HaveOccurred())

	// Verify the TLS termination policy
	Expect(route.Spec.TLS).ToNot(BeNil())
	if !t.TLS {
		Expect(route.Spec.TLS.Termination).To(Equal(openshiftv1.TLSTerminationEdge))
		Expect(route.Spec.TLS.InsecureEdgeTerminationPolicy).To(Equal(openshiftv1.InsecureEdgeTerminationPolicyRedirect))
	} else {
		Expect(route.Spec.TLS.Termination).To(Equal(openshiftv1.TLSTerminationReencrypt))
		Expect(route.Spec.TLS.DestinationCACertificate).To(Equal("cryostat-ca-bytes"))
	}
	return route
}

func (t *cryostatTestInput) reconcileCryostatFully() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	if t.TLS {
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

func (t *cryostatTestInput) reconcileDeletedCryostat() {
	// Simulate deletion by setting DeletionTimestamp
	cr := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())

	delTime := metav1.Unix(0, 1598045501618*int64(time.Millisecond))
	cr.DeletionTimestamp = &delTime
	err = t.Client.Update(context.Background(), cr)
	Expect(err).ToNot(HaveOccurred())

	// Reconcile again
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}

func checkMetadata(object metav1.Object, expected metav1.Object) {
	Expect(object.GetName()).To(Equal(expected.GetName()))
	Expect(object.GetNamespace()).To(Equal(expected.GetNamespace()))
	Expect(object.GetLabels()).To(Equal(expected.GetLabels()))
	Expect(object.GetAnnotations()).To(Equal(expected.GetAnnotations()))
	ownerReferences := object.GetOwnerReferences()
	Expect(ownerReferences[0].Kind).To(Equal("Cryostat"))
	Expect(ownerReferences[0].Name).To(Equal("cryostat"))
}

func (t *cryostatTestInput) expectCertificates() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
	t.checkCertificates()
}

func (t *cryostatTestInput) checkCertificates() {
	certNames := []string{"cryostat", "cryostat-ca", "cryostat-grafana"}
	for _, certName := range certNames {
		cert := &certv1.Certificate{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: certName, Namespace: "default"}, cert)
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *cryostatTestInput) expectRBAC() {
	t.reconcileCryostatFully()

	sa := &corev1.ServiceAccount{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, sa)
	Expect(err).ToNot(HaveOccurred())
	expectedSA := test.NewServiceAccount()
	Expect(sa.Labels).To(Equal(expectedSA.Labels))
	Expect(sa.Annotations).To(Equal(expectedSA.Annotations))
	Expect(sa.Secrets).To(Equal(expectedSA.Secrets))
	Expect(sa.ImagePullSecrets).To(Equal(expectedSA.ImagePullSecrets))
	Expect(sa.AutomountServiceAccountToken).To(Equal(expectedSA.AutomountServiceAccountToken))

	role := &rbacv1.Role{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, role)
	Expect(err).ToNot(HaveOccurred())
	expectedRole := test.NewRole()
	Expect(role.Rules).To(Equal(expectedRole.Rules))

	binding := &rbacv1.RoleBinding{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, binding)
	Expect(err).ToNot(HaveOccurred())
	expectedBinding := test.NewRoleBinding()
	Expect(binding.Subjects).To(Equal(expectedBinding.Subjects))
	Expect(binding.RoleRef).To(Equal(expectedBinding.RoleRef))

	clusterBinding := &rbacv1.ClusterRoleBinding{}
	err = t.Client.Get(context.Background(), types.NamespacedName{
		Name: "cryostat-9ecd5050500c2566765bc593edfcce12434283e5da32a27476bc4a1569304a02"}, clusterBinding)
	Expect(err).ToNot(HaveOccurred())
	expectedClusterBinding := test.NewClusterRoleBinding()
	Expect(clusterBinding.Subjects).To(Equal(expectedClusterBinding.Subjects))
	Expect(clusterBinding.RoleRef).To(Equal(expectedClusterBinding.RoleRef))
}

func (t *cryostatTestInput) checkClusterRoleBindingDeleted() {
	clusterBinding := &rbacv1.ClusterRoleBinding{}
	err := t.Client.Get(context.Background(), types.NamespacedName{
		Name: "cryostat-9ecd5050500c2566765bc593edfcce12434283e5da32a27476bc4a1569304a02"}, clusterBinding)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectRoutes() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	if t.TLS {
		t.makeCertificatesReady()
		t.initializeSecrets()
	}

	// Check for routes, ingress configuration needs to be added as each
	// one is created so that they all reconcile successfully
	result, err = t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
	t.ingressConfig(req)
}

func (t *cryostatTestInput) expectNoRoutes() {
	svc := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectIngresses() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Update certificate status
	t.makeCertificatesReady()
	t.initializeSecrets()

	result, err = t.controller.Reconcile(context.Background(), req)
	expectedConfig := test.NewNetworkConfigurationList(t.externalTLS)

	ingress := &netv1.Ingress{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.CoreConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.CoreConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.CoreConfig.IngressSpec))

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.CommandConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.CommandConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.CommandConfig.IngressSpec))

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))
}

func (t *cryostatTestInput) expectNoIngresses() {
	ing := &netv1.Ingress{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectPVC(expectedPvc *corev1.PersistentVolumeClaim) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, pvc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileCryostatFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, pvc)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	checkMetadata(pvc, expectedPvc)
	Expect(pvc.Spec.AccessModes).To(Equal(expectedPvc.Spec.AccessModes))
	Expect(pvc.Spec.StorageClassName).To(Equal(expectedPvc.Spec.StorageClassName))

	pvcStorage := pvc.Spec.Resources.Requests["storage"]
	expectedPvcStorage := expectedPvc.Spec.Resources.Requests["storage"]
	Expect(pvcStorage.Equal(expectedPvcStorage)).To(BeTrue())
}

func (t *cryostatTestInput) expectJMXSecret() {
	secret := &corev1.Secret{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-jmx-auth", Namespace: "default"}, secret)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileCryostatFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-jmx-auth", Namespace: "default"}, secret)
	Expect(err).ToNot(HaveOccurred())

	expectedSecret := resource_definitions.NewJmxSecretForCR(test.NewCryostat())
	checkMetadata(secret, expectedSecret)
	Expect(secret.StringData["CRYOSTAT_RJMX_USER"]).To(Equal(expectedSecret.StringData["CRYOSTAT_RJMX_USER"]))
}

func (t *cryostatTestInput) expectCoreService() {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileCryostatFully()

	t.checkService("cryostat", test.NewCryostatService())
}

func (t *cryostatTestInput) expectStatusApplicationURL() {
	instance := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, instance)

	t.reconcileCryostatFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, instance)
	Expect(err).ToNot(HaveOccurred())

	Expect(instance.Status.ApplicationURL).To(Equal("https://cryostat.example.com"))
}

func (t *cryostatTestInput) expectCommandChannel() {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileCryostatFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-command", Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	expectedService := resource_definitions.NewCommandChannelService(test.NewCryostat())
	checkMetadata(service, expectedService)
	Expect(service.Spec.Type).To(Equal(expectedService.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expectedService.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expectedService.Spec.Ports))
}

func (t *cryostatTestInput) expectDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileCryostatFully()
	t.checkDeployment()
}

func (t *cryostatTestInput) expectIdempotence() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	t.reconcileCryostatFully()

	obj := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), req.NamespacedName, obj)
	Expect(err).ToNot(HaveOccurred())

	// Reconcile again
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))

	obj2 := &operatorv1beta1.Cryostat{}
	err = t.Client.Get(context.Background(), req.NamespacedName, obj2)
	Expect(err).ToNot(HaveOccurred())
	Expect(obj2.Status).To(Equal(obj.Status))
	Expect(obj2.Spec).To(Equal(obj.Spec))
}

func (t *cryostatTestInput) expectCryostatFinalizerPresent() {
	cr := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())
	Expect(cr.GetFinalizers()).To(ContainElement("operator.cryostat.io/cryostat.finalizer"))
}

func (t *cryostatTestInput) expectCryostatFinalizerAbsent() {
	cr := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())
	Expect(cr.GetFinalizers()).ToNot(ContainElement("operator.cryostat.io/cryostat.finalizer"))
}

func (t *cryostatTestInput) checkGrafanaService() {
	t.checkService("cryostat-grafana", test.NewGrafanaService())
}

func (t *cryostatTestInput) checkService(svcName string, expected *corev1.Service) {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: svcName, Namespace: "default"}, service)
	Expect(err).ToNot(HaveOccurred())

	checkMetadata(service, expected)
	Expect(service.Spec.Type).To(Equal(expected.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expected.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expected.Spec.Ports))
}

func (t *cryostatTestInput) checkDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	cr := &operatorv1beta1.Cryostat{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())

	Expect(deployment.Name).To(Equal("cryostat"))
	Expect(deployment.Namespace).To(Equal("default"))
	Expect(deployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to": "cryostat-operator-controller-manager",
	}))
	Expect(deployment.Labels).To(Equal(map[string]string{
		"app":                    "cryostat",
		"kind":                   "cryostat",
		"app.kubernetes.io/name": "cryostat",
	}))
	Expect(metav1.IsControlledBy(deployment, cr)).To(BeTrue())
	Expect(deployment.Spec.Selector).To(Equal(test.NewDeploymentSelector()))

	// compare Pod template
	template := deployment.Spec.Template
	Expect(template.Name).To(Equal("cryostat"))
	Expect(template.Namespace).To(Equal("default"))
	Expect(template.Labels).To(Equal(map[string]string{
		"app":  "cryostat",
		"kind": "cryostat",
	}))
	Expect(template.Spec.Volumes).To(Equal(test.NewVolumes(t.minimal, t.TLS)))
	Expect(template.Spec.SecurityContext).To(Equal(test.NewPodSecurityContext()))

	// Check that the networking environment variables are set correctly
	coreContainer := template.Spec.Containers[0]
	checkCoreContainer(&coreContainer, t.minimal, t.TLS, t.externalTLS, t.EnvCoreImageTag, t.controller.IsOpenShift)

	if !t.minimal {
		// Check that Grafana is configured properly, depending on the environment
		grafanaContainer := template.Spec.Containers[1]
		checkGrafanaContainer(&grafanaContainer, t.TLS, t.EnvGrafanaImageTag)

		// Check that JFR Datasource is configured properly
		datasourceContainer := template.Spec.Containers[2]
		checkDatasourceContainer(&datasourceContainer, t.EnvDatasourceImageTag)
	}

	// Check that the proper Service Account is set
	Expect(template.Spec.ServiceAccountName).To(Equal("cryostat"))
}

func (t *cryostatTestInput) checkDeploymentHasTemplates() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := test.NewVolumesWithTemplates(t.TLS)
	Expect(volumes).To(Equal(expectedVolumes))

	volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	expectedVolumeMounts := test.NewVolumeMountsWithTemplates(t.TLS)
	Expect(volumeMounts).To(Equal(expectedVolumeMounts))
}

func checkCoreContainer(container *corev1.Container, minimal bool, tls bool, externalTLS bool,
	tag *string, openshift bool) {
	Expect(container.Name).To(Equal("cryostat"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewCorePorts()))
	Expect(container.Env).To(ConsistOf(test.NewCoreEnvironmentVariables(minimal, tls, externalTLS, openshift)))
	Expect(container.EnvFrom).To(ConsistOf(test.NewCoreEnvFromSource(tls)))
	Expect(container.VolumeMounts).To(ConsistOf(test.NewCoreVolumeMounts(tls)))
	Expect(container.LivenessProbe).To(Equal(test.NewCoreLivenessProbe(tls)))
	Expect(container.StartupProbe).To(Equal(test.NewCoreStartupProbe(tls)))
}

func checkGrafanaContainer(container *corev1.Container, tls bool, tag *string) {
	Expect(container.Name).To(Equal("cryostat-grafana"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat-grafana-dashboard:"))
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
	Expect(container.Name).To(Equal("cryostat-jfr-datasource"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/jfr-datasource:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewDatasourcePorts()))
	Expect(container.Env).To(ConsistOf(test.NewDatasourceEnvironmentVariables()))
	Expect(container.EnvFrom).To(BeEmpty())
	Expect(container.VolumeMounts).To(BeEmpty())
	Expect(container.LivenessProbe).To(Equal(test.NewDatasourceLivenessProbe()))
}
