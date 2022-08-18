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
	"strconv"
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
	controller     *controllers.CryostatReconciler
	objs           []runtime.Object
	minimal        bool
	reportReplicas int32
	externalTLS    bool
	test.TestReconcilerConfig
}

var _ = Describe("CryostatController", func() {
	var t *cryostatTestInput

	JustBeforeEach(func() {
		logger := zap.New()
		logf.SetLogger(logger)
		s := test.NewTestScheme()

		t.Client = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(t.objs...).Build()
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
			It("should set GrafanaSecret in CR Status", func() {
				t.expectStatusGrafanaSecretName()
			})
			It("should create deployment and set owner", func() {
				t.expectDeployment()
			})
			It("should set TLSSetupComplete condition", func() {
				t.reconcileCryostatFully()
				t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"AllCertificatesReady")
			})
			Context("deployment is progressing", func() {
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
					t.makeDeploymentProgress("cryostat")
				})
				It("should update conditions", func() {
					t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentAvailable, metav1.ConditionFalse,
						"TestAvailable")
					t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentProgressing, metav1.ConditionTrue,
						"TestProgressing")
					t.checkConditionAbsent(operatorv1beta1.ConditionTypeMainDeploymentReplicaFailure)
				})
				Context("then becomes available", func() {
					JustBeforeEach(func() {
						t.makeDeploymentAvailable("cryostat")
					})
					It("should update conditions", func() {
						t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentAvailable, metav1.ConditionTrue,
							"TestAvailable")
						t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentProgressing, metav1.ConditionTrue,
							"TestProgressing")
						t.checkConditionAbsent(operatorv1beta1.ConditionTypeMainDeploymentReplicaFailure)
					})
				})
				Context("then fails to roll out", func() {
					JustBeforeEach(func() {
						t.makeDeploymentFail("cryostat")
					})
					It("should update conditions", func() {
						t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentAvailable, metav1.ConditionFalse,
							"TestAvailable")
						t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentProgressing, metav1.ConditionFalse,
							"TestProgressing")
						t.checkConditionPresent(operatorv1beta1.ConditionTypeMainDeploymentReplicaFailure, metav1.ConditionTrue,
							"TestReplicaFailure")
					})
				})
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
			It("should set GrafanaSecret in CR Status", func() {
				t.expectStatusGrafanaSecretName()
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
		Context("with an existing main Deployment", func() {
			var cr *operatorv1beta1.Cryostat
			var oldDeploy *appsv1.Deployment
			BeforeEach(func() {
				cr = test.NewCryostat()
				oldDeploy = test.OtherDeployment()
				t.objs = append(t.objs, cr, oldDeploy)
			})
			It("should update the Deployment", func() {
				t.reconcileCryostatFully()

				deploy := &appsv1.Deployment{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deploy)
				Expect(err).ToNot(HaveOccurred())

				Expect(deploy.Annotations).To(Equal(map[string]string{
					"app.openshift.io/connects-to": "cryostat-operator-controller-manager",
					"other":                        "annotation",
				}))
				Expect(deploy.Labels).To(Equal(map[string]string{
					"app":                    "cryostat",
					"kind":                   "cryostat",
					"component":              "cryostat",
					"app.kubernetes.io/name": "cryostat",
					"other":                  "label",
				}))
				Expect(metav1.IsControlledBy(deploy, cr)).To(BeTrue())

				t.checkMainPodTemplate(deploy, cr)

				Expect(deploy.Spec.Selector).To(Equal(test.NewMainDeploymentSelector()))
				Expect(deploy.Spec.Replicas).To(Equal(oldDeploy.Spec.Replicas))
			})
		})
		Context("with an existing Service Account", func() {
			var cr *operatorv1beta1.Cryostat
			var oldSA *corev1.ServiceAccount
			BeforeEach(func() {
				cr = test.NewCryostat()
				oldSA = test.OtherServiceAccount()
				t.objs = append(t.objs, cr, oldSA)
			})
			It("should update the Service Account", func() {
				t.reconcileCryostatFully()

				sa := &corev1.ServiceAccount{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, sa)
				Expect(err).ToNot(HaveOccurred())

				Expect(sa.Annotations).To(Equal(map[string]string{
					"hello": "world",
					"serviceaccounts.openshift.io/oauth-redirectreference.route": `{"metadata":{"creationTimestamp":null},"reference":{"group":"","kind":"Route","name":"cryostat"}}`,
				}))

				Expect(sa.Labels).To(Equal(map[string]string{
					"app":   "cryostat",
					"other": "label",
				}))

				Expect(metav1.IsControlledBy(sa, cr)).To(BeTrue())

				Expect(sa.ImagePullSecrets).To(Equal(oldSA.ImagePullSecrets))
				Expect(sa.Secrets).To(Equal(oldSA.Secrets))
				Expect(sa.AutomountServiceAccountToken).To(BeNil())
			})
		})
		Context("with existing Routes", func() {
			var cr *operatorv1beta1.Cryostat
			var oldCoreRoute *openshiftv1.Route
			var oldGrafanaRoute *openshiftv1.Route
			BeforeEach(func() {
				cr = test.NewCryostat()
				oldCoreRoute = test.OtherCoreRoute()
				oldGrafanaRoute = test.OtherGrafanaRoute()
				t.objs = append(t.objs, cr, oldCoreRoute, oldGrafanaRoute)
			})
			It("should update the Routes", func() {
				t.reconcileCryostatFully()

				// Routes should be replaced
				t.checkRoutes()
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
				err = t.Client.Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				t.updateRouteStatus(req, test.NewGrafanaRoute(t.TLS))
			})
			It("should create grafana resources", func() {
				t.checkGrafanaService()
			})
			It("should configure deployment appropriately", func() {
				t.checkMainDeployment()
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
				t.checkMainDeployment()
			})
		})
		Context("Switching from 0 report sidecars to 1", func() {
			var cr *operatorv1beta1.Cryostat
			BeforeEach(func() {
				cr = test.NewCryostat()
				t.objs = append(t.objs, cr)
				t.reportReplicas = 1
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				cryostat.Spec.ReportOptions.Replicas = t.reportReplicas
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should configure deployment appropriately", func() {
				t.checkMainDeployment()
				t.checkReportsDeployment()
				t.checkService("cryostat-reports", test.NewReportsService())
			})
			Context("with cert-manager disabled", func() {
				BeforeEach(func() {
					disable := false
					cr.Spec.EnableCertManager = &disable
					t.TLS = false
				})
				It("should configure deployment appropriately", func() {
					t.checkMainDeployment()
					t.checkReportsDeployment()
					t.checkService("cryostat-reports", test.NewReportsService())
				})
			})
			Context("with resource requirements", func() {
				BeforeEach(func() {
					*cr = *test.NewCryostatWithReportsResources()
				})
				It("should configure deployment appropriately", func() {
					t.checkMainDeployment()
					t.checkReportsDeployment()
					t.checkService("cryostat-reports", test.NewReportsService())
				})
			})
			Context("deployment is progressing", func() {
				JustBeforeEach(func() {
					t.makeDeploymentProgress("cryostat-reports")
				})
				It("should update conditions", func() {
					t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentAvailable, metav1.ConditionFalse,
						"TestAvailable")
					t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentProgressing, metav1.ConditionTrue,
						"TestProgressing")
					t.checkConditionAbsent(operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure)
				})
				Context("then becomes available", func() {
					JustBeforeEach(func() {
						t.makeDeploymentAvailable("cryostat-reports")
					})
					It("should update conditions", func() {
						t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentAvailable, metav1.ConditionTrue,
							"TestAvailable")
						t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentProgressing, metav1.ConditionTrue,
							"TestProgressing")
						t.checkConditionAbsent(operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure)
					})
				})
				Context("then fails to roll out", func() {
					JustBeforeEach(func() {
						t.makeDeploymentFail("cryostat-reports")
					})
					It("should update conditions", func() {
						t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentAvailable, metav1.ConditionFalse,
							"TestAvailable")
						t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentProgressing, metav1.ConditionFalse,
							"TestProgressing")
						t.checkConditionPresent(operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure, metav1.ConditionTrue,
							"TestReplicaFailure")
					})
				})
			})
		})
		Context("Switching from 1 report sidecar to 2", func() {
			BeforeEach(func() {
				t.reportReplicas = 1
				cr := test.NewCryostat()
				cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
					Replicas: t.reportReplicas,
				}
				t.objs = append(t.objs, cr)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.reportReplicas = 2
				cryostat.Spec.ReportOptions.Replicas = t.reportReplicas
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should configure deployment appropriately", func() {
				t.checkMainDeployment()
				t.checkReportsDeployment()
				t.checkService("cryostat-reports", test.NewReportsService())
			})
		})
		Context("Switching from 2 report sidecars to 1", func() {
			BeforeEach(func() {
				t.reportReplicas = 2
				cr := test.NewCryostat()
				cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
					Replicas: t.reportReplicas,
				}
				t.objs = append(t.objs, cr)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.reportReplicas = 1
				cryostat.Spec.ReportOptions.Replicas = t.reportReplicas
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should configure deployment appropriately", func() {
				t.checkMainDeployment()
				t.checkReportsDeployment()
				t.checkService("cryostat-reports", test.NewReportsService())
			})
		})
		Context("Switching from 1 report sidecar to 0", func() {
			BeforeEach(func() {
				t.reportReplicas = 1
				cr := test.NewCryostat()
				cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
					Replicas: t.reportReplicas,
				}
				t.objs = append(t.objs, cr)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
				t.makeDeploymentAvailable("cryostat-reports")

				cryostat := &operatorv1beta1.Cryostat{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cryostat)
				Expect(err).ToNot(HaveOccurred())

				t.reportReplicas = 0
				cryostat.Spec.ReportOptions.Replicas = t.reportReplicas
				err = t.Client.Status().Update(context.Background(), cryostat)
				Expect(err).ToNot(HaveOccurred())

				req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
				result, err := t.controller.Reconcile(context.Background(), req)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
			It("should configure deployment appropriately", func() {
				t.checkMainDeployment()
				t.expectNoService("cryostat-reports")
				t.expectNoReportsDeployment()
			})
			It("should remove conditions", func() {
				t.checkConditionAbsent(operatorv1beta1.ConditionTypeReportsDeploymentAvailable)
				t.checkConditionAbsent(operatorv1beta1.ConditionTypeReportsDeploymentProgressing)
				t.checkConditionAbsent(operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure)
			})
		})
		Context("Cryostat CR has list of certificate secrets", func() {
			var cr *operatorv1beta1.Cryostat
			BeforeEach(func() {
				cr = test.NewCryostatWithSecrets()
				t.objs = append(t.objs, cr,
					newFakeSecret("testCert1"), newFakeSecret("testCert2"))
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.expectDeploymentHasCertSecrets()
			})
			Context("with cert-manager disabled", func() {
				BeforeEach(func() {
					disable := false
					cr.Spec.EnableCertManager = &disable
					t.TLS = false
				})
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.expectDeploymentHasCertSecrets()
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
				expectedVolumes := test.NewVolumesWithSecrets(t.TLS)
				Expect(volumes).To(ConsistOf(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := test.NewCoreVolumeMounts(t.TLS)
				Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
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
		Context("with custom EmptyDir config", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithDefaultEmptyDir())
			})
			It("should create the EmptyDir with default specs", func() {
				t.expectEmptyDir(test.NewDefaultEmptyDir())
			})
		})
		Context("with custom EmptyDir config with requested spec", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithEmptyDirSpec())
			})
			It("should create the EmptyDir with requested specs", func() {
				t.expectEmptyDir(test.NewEmptyDirWithSpec())
			})
		})
		Context("with overriden image tags", func() {
			var mainDeploy, reportsDeploy *appsv1.Deployment
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithReportsSvc())
				t.reportReplicas = 1
				mainDeploy = &appsv1.Deployment{}
				reportsDeploy = &appsv1.Deployment{}
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, mainDeploy)
				Expect(err).ToNot(HaveOccurred())
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-reports", Namespace: "default"}, reportsDeploy)
				Expect(err).ToNot(HaveOccurred())
			})
			Context("for development", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:1.0.0-SNAPSHOT"
					datasourceImg := "my/datasource-image:1.0.0-BETA25"
					grafanaImg := "my/grafana-image:1.0.0-dev"
					reportsImg := "my/reports-image:1.0.0-SNAPSHOT"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to Always", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullAlways))
					}
					reportContainers := reportsDeploy.Spec.Template.Spec.Containers
					Expect(reportContainers).To(HaveLen(1))
					Expect(reportContainers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
				})
			})
			Context("for release", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:1.0.0"
					datasourceImg := "my/datasource-image:1.0.0"
					grafanaImg := "my/grafana-image:1.0.0"
					reportsImg := "my/reports-image:1.0.0"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to IfNotPresent", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
					}
					reportContainers := reportsDeploy.Spec.Template.Spec.Containers
					Expect(reportContainers).To(HaveLen(1))
					Expect(reportContainers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
				})
			})
			Context("by digest", func() {
				BeforeEach(func() {
					coreImg := "my/core-image@sha256:99b57e9b8880bc5d4d799b508603628c37c3e6a0d4bdd0988e9dc3ad8e04c495"
					datasourceImg := "my/datasource-image@sha256:59ded87392077c2371b26e021aade0409855b597383fa78e549eefafab8fc90c"
					grafanaImg := "my/grafana-image@sha256:e5bc16c2c5b69cd6fd8fdf1381d0a8b6cc9e01d92b9e1bb0a61ed89196563c72"
					reportsImg := "my/reports-image@sha256:8a23ca5e8c8a343789b8c14558a44a49d35ecd130c18e62edf0d1ad9ce88d37d"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to IfNotPresent", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
					}
					reportContainers := reportsDeploy.Spec.Template.Spec.Containers
					Expect(reportContainers).To(HaveLen(1))
					Expect(reportContainers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
				})
			})
			Context("with latest", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:latest"
					datasourceImg := "my/datasource-image:latest"
					grafanaImg := "my/grafana-image:latest"
					reportsImg := "my/reports-image:latest"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
				})
				It("should create deployment with the expected tags", func() {
					t.checkMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to Always", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(3))
					for _, container := range containers {
						Expect(container.ImagePullPolicy).To(Equal(corev1.PullAlways))
					}
					reportContainers := reportsDeploy.Spec.Template.Spec.Containers
					Expect(reportContainers).To(HaveLen(1))
					Expect(reportContainers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
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
				It("should delete Cryostat", func() {
					t.expectNoCryostat()
				})
			})
			Context("ClusterRoleBinding does not exist", func() {
				JustBeforeEach(func() {
					err := t.Client.Delete(context.Background(), test.NewClusterRoleBinding())
					Expect(err).ToNot(HaveOccurred())
					t.reconcileDeletedCryostat()
				})
				It("should delete Cryostat", func() {
					t.expectNoCryostat()
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
					It("should delete Cryostat", func() {
						t.expectNoCryostat()
					})
				})
				Context("ConsoleLink does not exist", func() {
					JustBeforeEach(func() {
						err := t.Client.Delete(context.Background(), test.NewConsoleLink())
						Expect(err).ToNot(HaveOccurred())
						t.reconcileDeletedCryostat()
					})
					It("should delete Cryostat", func() {
						t.expectNoCryostat()
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
			It("should set TLSSetupComplete condition", func() {
				t.reconcileCryostatFully()
				t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"AllCertificatesReady")
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
			It("should set TLSSetupComplete Condition", func() {
				t.reconcileCryostatFully()
				t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"CertManagerDisabled")
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
				t.checkMainDeployment()
			})
			It("should create routes with edge TLS termination", func() {
				t.checkRoutes()
			})
			It("should set TLSSetupComplete Condition", func() {
				t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"CertManagerDisabled")
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
				t.checkMainDeployment()
			})
			It("should create certificates", func() {
				t.checkCertificates()
			})
			It("should create routes with re-encrypt TLS termination", func() {
				t.checkRoutes()
			})
			It("should set TLSSetupComplete condition", func() {
				t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"AllCertificatesReady")
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
				JustBeforeEach(func() {
					req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
					_, err := t.controller.Reconcile(context.Background(), req)
					Expect(err).To(HaveOccurred())
				})
				It("should emit a CertManagerUnavailable Event", func() {
					recorder := t.controller.EventRecorder.(*record.FakeRecorder)
					var eventMsg string
					Expect(recorder.Events).To(Receive(&eventMsg))
					Expect(eventMsg).To(ContainSubstring("CertManagerUnavailable"))
				})
				It("should set TLSSetupComplete Condition", func() {
					t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
						"CertManagerUnavailable")
				})
			})
			Context("and disabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatCertManagerDisabled())
					t.TLS = false
				})
				JustBeforeEach(func() {
					req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
					_, err := t.controller.Reconcile(context.Background(), req)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should not emit a CertManagerUnavailable Event", func() {
					recorder := t.controller.EventRecorder.(*record.FakeRecorder)
					Expect(recorder.Events).ToNot(Receive())
				})
				It("should set TLSSetupComplete Condition", func() {
					t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
						"CertManagerDisabled")
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
			Context("containing reports config", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithReportsSvc())
					t.reportReplicas = 1
				})
				It("should created the service as described", func() {
					t.checkService("cryostat-reports", test.NewCustomizedReportsService())
				})
			})
			Context("and existing services", func() {
				var cr *operatorv1beta1.Cryostat
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostat())
				})
				JustBeforeEach(func() {
					// Fetch the current Cryostat CR
					namespacedName := types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}
					current := &operatorv1beta1.Cryostat{}
					err := t.Client.Get(context.Background(), namespacedName, current)
					Expect(err).ToNot(HaveOccurred())

					// Customize it with service options from the test specs
					current.Spec = cr.Spec
					err = t.Client.Update(context.Background(), current)
					Expect(err).ToNot(HaveOccurred())

					// Reconcile again
					result, err := t.controller.Reconcile(context.Background(), reconcile.Request{NamespacedName: namespacedName})
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(reconcile.Result{}))
				})
				Context("containing core config", func() {
					BeforeEach(func() {
						cr = test.NewCryostatWithCoreSvc()
					})
					It("should created the service as described", func() {
						t.checkService("cryostat", test.NewCustomizedCoreService())
					})
				})
				Context("containing grafana config", func() {
					BeforeEach(func() {
						cr = test.NewCryostatWithGrafanaSvc()
					})
					It("should created the service as described", func() {
						t.checkService("cryostat-grafana", test.NewCustomizedGrafanaService())
					})
				})
				Context("containing reports config", func() {
					BeforeEach(func() {
						cr = test.NewCryostatWithReportsSvc()
						t.reportReplicas = 1
					})
					It("should created the service as described", func() {
						t.checkService("cryostat-reports", test.NewCustomizedReportsService())
					})
				})
			})
		})
		Context("configuring environment variables with non-default spec values", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing MaxWsConnections", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithWsConnectionsSpec())
				})
				It("should set max WebSocket connections", func() {
					t.checkEnvironmentVariables(test.NewWsConnectionsEnv())
				})
			})
			Context("containing SubProcessMaxHeapSize", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithReportSubprocessHeapSpec())
				})
				It("should set report subprocess max heap size", func() {
					t.checkEnvironmentVariables(test.NewReportSubprocessHeapEnv())
				})
			})
			Context("containing JmxCacheOptions", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithJmxCacheOptionsSpec())
				})
				It("should set JMX cache options", func() {
					t.checkEnvironmentVariables(test.NewJmxCacheOptionsEnv())
				})
			})
		})
		Context("with resource requirements", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithResources())
			})
			It("should create expected deployment", func() {
				t.expectDeployment()
			})
		})
		Context("with network options", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing core config", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithCoreNetworkOptions())
				})
				It("should create the route as described", func() {
					t.checkRoute(test.NewCustomCoreRoute(t.TLS))
				})
			})
			Context("containing grafana config", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, test.NewCryostatWithGrafanaNetworkOptions())
				})
				It("should create the route as described", func() {
					t.checkRoute(test.NewCustomGrafanaRoute(t.TLS))
				})
			})
		})
		Context("Cryostat CR has authorization properties", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithAuthProperties(), test.NewAuthPropertiesConfigMap())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.checkDeploymentHasAuthProperties()
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
			It("should create RBAC", func() {
				t.expectRBAC()
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
			It("should create RBAC", func() {
				t.expectRBAC()
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
		Context("with existing Ingresses", func() {
			var cr *operatorv1beta1.Cryostat
			var oldCoreIngress *netv1.Ingress
			var oldGrafanaIngress *netv1.Ingress
			BeforeEach(func() {
				cr = test.NewCryostatWithIngress()
				oldCoreIngress = test.OtherCoreIngress()
				oldGrafanaIngress = test.OtherGrafanaIngress()
				t.objs = append(t.objs, cr, oldCoreIngress, oldGrafanaIngress)
			})
			It("should update the Ingresses", func() {
				t.expectIngresses()
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
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, ingress)
				Expect(err).ToNot(HaveOccurred())
				Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
				Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
				Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))

				err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ingress)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("Cryostat CR has authorization properties", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, test.NewCryostatWithAuthProperties(), test.NewAuthPropertiesConfigMap())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.checkDeploymentHasAuthProperties()
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
	certNames := []string{"cryostat", "cryostat-ca", "cryostat-grafana", "cryostat-reports"}
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
	secretNames := []string{"cryostat-ca", "cryostat-tls", "cryostat-grafana-tls", "cryostat-reports-tls"}
	for _, secretName := range secretNames {
		secret := newFakeSecret(secretName)
		err := t.Client.Create(context.Background(), secret)
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *cryostatTestInput) updateRouteStatus(req reconcile.Request,
	routes ...*openshiftv1.Route) {
	for _, route := range routes {
		result, err := t.controller.Reconcile(context.Background(), req)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

		err = t.Client.Get(context.Background(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, route)
		Expect(err).ToNot(HaveOccurred())
		route.Status.Ingress = append(route.Status.Ingress, openshiftv1.RouteIngress{
			Host: route.Name + ".example.com",
		})
		err = t.Client.Status().Update(context.Background(), route)
		Expect(err).ToNot(HaveOccurred())
	}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{}))
}

func (t *cryostatTestInput) checkRoutes() {
	if !t.minimal {
		t.checkRoute(test.NewGrafanaRoute(t.TLS))
	}
	t.checkRoute(test.NewCoreRoute(t.TLS))
}

func (t *cryostatTestInput) checkRoute(expected *openshiftv1.Route) *openshiftv1.Route {
	route := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, route)
	Expect(err).ToNot(HaveOccurred())

	checkMetadata(route, expected)
	Expect(route.Spec.To).To(Equal(expected.Spec.To))
	Expect(route.Spec.Port).To(Equal(expected.Spec.Port))
	Expect(route.Spec.TLS).To(Equal(expected.Spec.TLS))
	return route
}

func (t *cryostatTestInput) checkConditionPresent(condType operatorv1beta1.CryostatConditionType, status metav1.ConditionStatus, reason string) {
	cr := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())

	condition := meta.FindStatusCondition(cr.Status.Conditions, string(condType))
	Expect(condition).ToNot(BeNil())
	Expect(condition.Status).To(Equal(status))
	Expect(condition.Reason).To(Equal(reason))
}

func (t *cryostatTestInput) checkConditionAbsent(condType operatorv1beta1.CryostatConditionType) {
	cr := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())

	condition := meta.FindStatusCondition(cr.Status.Conditions, string(condType))
	Expect(condition).To(BeNil())
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
		if t.minimal {
			t.updateRouteStatus(req, test.NewCoreRoute(t.TLS))
		} else {
			t.updateRouteStatus(req, test.NewGrafanaRoute(t.TLS), test.NewCoreRoute(t.TLS))
		}
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
	Expect(ownerReferences).To(HaveLen(1))
	Expect(ownerReferences[0].Kind).To(Equal("Cryostat"))
	Expect(ownerReferences[0].Name).To(Equal("cryostat"))
}

func (t *cryostatTestInput) expectNoCryostat() {
	instance := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, instance)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectCertificates() {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	result, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))
	t.checkCertificates()

	// Check TLSSetupComplete condition
	t.checkConditionPresent(operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
		"WaitingForCertificate")
}

func (t *cryostatTestInput) checkCertificates() {
	// Check certificates
	certs := []*certv1.Certificate{test.NewCryostatCert(), test.NewCACert(), test.NewGrafanaCert(), test.NewReportsCert()}
	for _, expected := range certs {
		actual := &certv1.Certificate{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, actual)
		Expect(err).ToNot(HaveOccurred())
		checkMetadata(actual, expected)
		Expect(actual.Spec).To(Equal(expected.Spec))
	}
	// Check issuers as well
	issuers := []*certv1.Issuer{test.NewSelfSignedIssuer(), test.NewCryostatCAIssuer()}
	for _, expected := range issuers {
		actual := &certv1.Issuer{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, actual)
		Expect(err).ToNot(HaveOccurred())
		checkMetadata(actual, expected)
		Expect(actual.Spec).To(Equal(expected.Spec))
	}
}

func (t *cryostatTestInput) expectRBAC() {
	t.reconcileCryostatFully()

	sa := &corev1.ServiceAccount{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, sa)
	Expect(err).ToNot(HaveOccurred())
	expectedSA := test.NewServiceAccount(t.controller.IsOpenShift)
	checkMetadata(sa, expectedSA)
	Expect(sa.Secrets).To(Equal(expectedSA.Secrets))
	Expect(sa.ImagePullSecrets).To(Equal(expectedSA.ImagePullSecrets))
	Expect(sa.AutomountServiceAccountToken).To(Equal(expectedSA.AutomountServiceAccountToken))

	role := &rbacv1.Role{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, role)
	Expect(err).ToNot(HaveOccurred())
	expectedRole := test.NewRole()
	checkMetadata(role, expectedRole)
	Expect(role.Rules).To(Equal(expectedRole.Rules))

	binding := &rbacv1.RoleBinding{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, binding)
	Expect(err).ToNot(HaveOccurred())
	expectedBinding := test.NewRoleBinding()
	checkMetadata(binding, expectedBinding)
	Expect(binding.Subjects).To(Equal(expectedBinding.Subjects))
	Expect(binding.RoleRef).To(Equal(expectedBinding.RoleRef))

	clusterBinding := &rbacv1.ClusterRoleBinding{}
	err = t.Client.Get(context.Background(), types.NamespacedName{
		Name: "cryostat-9ecd5050500c2566765bc593edfcce12434283e5da32a27476bc4a1569304a02"}, clusterBinding)
	Expect(err).ToNot(HaveOccurred())
	expectedClusterBinding := test.NewClusterRoleBinding()
	Expect(clusterBinding.GetName()).To(Equal(expectedClusterBinding.GetName()))
	Expect(clusterBinding.GetNamespace()).To(Equal(expectedClusterBinding.GetNamespace()))
	Expect(clusterBinding.GetLabels()).To(Equal(expectedClusterBinding.GetLabels()))
	Expect(clusterBinding.GetAnnotations()).To(Equal(expectedClusterBinding.GetAnnotations()))
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
	t.reconcileCryostatFully()
	t.checkRoutes()
}

func (t *cryostatTestInput) expectNoRoutes() {
	svc := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) checkIngresses() {
	expectedConfig := test.NewNetworkConfigurationList(t.externalTLS)

	ingress := &netv1.Ingress{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.CoreConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.CoreConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.CoreConfig.IngressSpec))

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-grafana", Namespace: "default"}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expectedConfig.GrafanaConfig.Annotations))
	Expect(ingress.Labels).To(Equal(expectedConfig.GrafanaConfig.Labels))
	Expect(ingress.Spec).To(Equal(*expectedConfig.GrafanaConfig.IngressSpec))
}

func (t *cryostatTestInput) expectIngresses() {
	t.reconcileCryostatFully()
	t.checkIngresses()
}

func (t *cryostatTestInput) expectNoIngresses() {
	ing := &netv1.Ingress{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, ing)
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

func (t *cryostatTestInput) expectEmptyDir(expectedEmptyDir *corev1.EmptyDirVolumeSource) {
	t.reconcileCryostatFully()

	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volume := deployment.Spec.Template.Spec.Volumes[0]
	emptyDir := volume.EmptyDir

	// Compare to desired spec
	Expect(emptyDir.Medium).To(Equal(expectedEmptyDir.Medium))
	Expect(emptyDir.SizeLimit).To(Equal(expectedEmptyDir.SizeLimit))
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

func (t *cryostatTestInput) expectStatusGrafanaSecretName() {
	instance := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, instance)

	t.reconcileCryostatFully()

	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, instance)
	Expect(err).ToNot(HaveOccurred())

	Expect(instance.Status.GrafanaSecret).To(Equal("cryostat-grafana-basic"))
}

func (t *cryostatTestInput) expectDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())

	t.reconcileCryostatFully()
	t.checkMainDeployment()
}

func (t *cryostatTestInput) expectDeploymentHasCertSecrets() {
	t.reconcileCryostatFully()
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := test.NewVolumesWithSecrets(t.TLS)
	Expect(volumes).To(ConsistOf(expectedVolumes))

	volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	expectedVolumeMounts := test.NewCoreVolumeMounts(t.TLS)
	Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
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

func (t *cryostatTestInput) expectNoService(svcName string) {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: svcName, Namespace: "default"}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectNoReportsDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-reports", Namespace: "default"}, deployment)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) makeDeploymentProgress(deployName string) {
	statusTrue := corev1.ConditionTrue
	statusFalse := corev1.ConditionFalse
	t.setDeploymentConditions(deployName, &statusFalse, &statusTrue, nil)
}

func (t *cryostatTestInput) makeDeploymentAvailable(deployName string) {
	statusTrue := corev1.ConditionTrue
	t.setDeploymentConditions(deployName, &statusTrue, &statusTrue, nil)
}

func (t *cryostatTestInput) makeDeploymentFail(deployName string) {
	statusTrue := corev1.ConditionTrue
	statusFalse := corev1.ConditionFalse
	t.setDeploymentConditions(deployName, &statusFalse, &statusFalse, &statusTrue)
}

func (t *cryostatTestInput) setDeploymentConditions(deployName string, available *corev1.ConditionStatus,
	progressing *corev1.ConditionStatus, replicaFailure *corev1.ConditionStatus) {
	// Update Deployment's "Available" Condition
	deploy := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: deployName, Namespace: "default"}, deploy)
	Expect(err).ToNot(HaveOccurred())

	conditions := []appsv1.DeploymentCondition{}
	if available != nil {
		conditions = append(conditions, appsv1.DeploymentCondition{
			Type:    appsv1.DeploymentAvailable,
			Status:  *available,
			Reason:  "TestAvailable",
			Message: "Test made deployment available.",
		})
	}
	if progressing != nil {
		conditions = append(conditions, appsv1.DeploymentCondition{
			Type:    appsv1.DeploymentProgressing,
			Status:  *progressing,
			Reason:  "TestProgressing",
			Message: "Test made deployment progressing.",
		})
	}
	if replicaFailure != nil {
		conditions = append(conditions, appsv1.DeploymentCondition{
			Type:    appsv1.DeploymentReplicaFailure,
			Status:  *replicaFailure,
			Reason:  "TestReplicaFailure",
			Message: "Test made deployment fail to replicate.",
		})
	}
	deploy.Status.Conditions = conditions

	err = t.Client.Status().Update(context.Background(), deploy)
	Expect(err).ToNot(HaveOccurred())

	// Reconcile again
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cryostat", Namespace: "default"}}
	res, err := t.controller.Reconcile(context.Background(), req)
	Expect(err).ToNot(HaveOccurred())
	Expect(res).To(Equal(reconcile.Result{}))
}

func (t *cryostatTestInput) checkMainDeployment() {
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
		"component":              "cryostat",
		"app.kubernetes.io/name": "cryostat",
	}))
	Expect(metav1.IsControlledBy(deployment, cr)).To(BeTrue())
	Expect(deployment.Spec.Selector).To(Equal(test.NewMainDeploymentSelector()))

	// compare Pod template
	t.checkMainPodTemplate(deployment, cr)
}

func (t *cryostatTestInput) checkMainPodTemplate(deployment *appsv1.Deployment, cr *operatorv1beta1.Cryostat) {
	template := deployment.Spec.Template
	Expect(template.Name).To(Equal("cryostat"))
	Expect(template.Namespace).To(Equal("default"))
	Expect(template.Labels).To(Equal(map[string]string{
		"app":       "cryostat",
		"kind":      "cryostat",
		"component": "cryostat",
	}))
	Expect(template.Spec.Volumes).To(ConsistOf(test.NewVolumes(t.minimal, t.TLS)))
	Expect(template.Spec.SecurityContext).To(Equal(test.NewPodSecurityContext()))

	// Check that the networking environment variables are set correctly
	coreContainer := template.Spec.Containers[0]
	port := "10000"
	if cr.Spec.ServiceOptions != nil && cr.Spec.ServiceOptions.ReportsConfig != nil &&
		cr.Spec.ServiceOptions.ReportsConfig.HTTPPort != nil {
		port = strconv.Itoa(int(*cr.Spec.ServiceOptions.ReportsConfig.HTTPPort))
	}
	var reportsUrl string
	if t.reportReplicas == 0 {
		reportsUrl = ""
	} else if t.TLS {
		reportsUrl = "https://cryostat-reports:" + port
	} else {
		reportsUrl = "http://cryostat-reports:" + port
	}

	checkCoreContainer(&coreContainer, t.minimal, t.TLS, t.externalTLS, t.EnvCoreImageTag, t.controller.IsOpenShift, reportsUrl, cr.Spec.Resources.CoreResources)

	if !t.minimal {
		// Check that Grafana is configured properly, depending on the environment
		grafanaContainer := template.Spec.Containers[1]
		checkGrafanaContainer(&grafanaContainer, t.TLS, t.EnvGrafanaImageTag, cr.Spec.Resources.GrafanaResources)

		// Check that JFR Datasource is configured properly
		datasourceContainer := template.Spec.Containers[2]
		checkDatasourceContainer(&datasourceContainer, t.EnvDatasourceImageTag, cr.Spec.Resources.DataSourceResources)
	}

	// Check that the proper Service Account is set
	Expect(template.Spec.ServiceAccountName).To(Equal("cryostat"))
}

func (t *cryostatTestInput) checkReportsDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat-reports", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	cr := &operatorv1beta1.Cryostat{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, cr)
	Expect(err).ToNot(HaveOccurred())

	Expect(deployment.Name).To(Equal("cryostat-reports"))
	Expect(deployment.Namespace).To(Equal("default"))
	Expect(deployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to": "cryostat",
	}))
	Expect(deployment.Labels).To(Equal(map[string]string{
		"app":                    "cryostat",
		"kind":                   "cryostat",
		"component":              "reports",
		"app.kubernetes.io/name": "cryostat-reports",
	}))
	Expect(metav1.IsControlledBy(deployment, cr)).To(BeTrue())
	Expect(deployment.Spec.Selector).To(Equal(test.NewReportsDeploymentSelector()))
	Expect(*deployment.Spec.Replicas).To(Equal(t.reportReplicas))

	// compare Pod template
	template := deployment.Spec.Template
	Expect(template.Name).To(Equal("cryostat-reports"))
	Expect(template.Namespace).To(Equal("default"))
	Expect(template.Labels).To(Equal(map[string]string{
		"app":       "cryostat",
		"kind":      "cryostat",
		"component": "reports",
	}))
	Expect(template.Spec.Volumes).To(ConsistOf(test.NewReportsVolumes(t.TLS)))

	var resources corev1.ResourceRequirements
	if cr.Spec.ReportOptions != nil {
		resources = cr.Spec.ReportOptions.Resources
	}
	checkReportsContainer(&template.Spec.Containers[0], t.TLS, t.EnvReportsImageTag, resources)
	// Check that the proper Service Account is set
	Expect(template.Spec.ServiceAccountName).To(Equal("cryostat"))
}

func (t *cryostatTestInput) checkDeploymentHasTemplates() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := test.NewVolumesWithTemplates(t.TLS)
	Expect(volumes).To(ConsistOf(expectedVolumes))

	volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	expectedVolumeMounts := test.NewVolumeMountsWithTemplates(t.TLS)
	Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
}

func (t *cryostatTestInput) checkDeploymentHasAuthProperties() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := test.NewVolumeWithAuthProperties(t.TLS)
	Expect(volumes).To(ConsistOf(expectedVolumes))

	volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	expectedVolumeMounts := test.NewVolumeMountsWithAuthProperties(t.TLS)
	Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
}

func checkCoreContainer(container *corev1.Container, minimal bool, tls bool, externalTLS bool,
	tag *string, openshift bool, reportsUrl string, resources corev1.ResourceRequirements) {
	Expect(container.Name).To(Equal("cryostat"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewCorePorts()))
	Expect(container.Env).To(ConsistOf(test.NewCoreEnvironmentVariables(minimal, tls, externalTLS, openshift, reportsUrl)))
	Expect(container.EnvFrom).To(ConsistOf(test.NewCoreEnvFromSource(tls)))
	Expect(container.VolumeMounts).To(ConsistOf(test.NewCoreVolumeMounts(tls)))
	Expect(container.LivenessProbe).To(Equal(test.NewCoreLivenessProbe(tls)))
	Expect(container.StartupProbe).To(Equal(test.NewCoreStartupProbe(tls)))
	Expect(container.Resources).To(Equal(resources))
}

func checkGrafanaContainer(container *corev1.Container, tls bool, tag *string, resources corev1.ResourceRequirements) {
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
	Expect(container.Resources).To(Equal(resources))
}

func checkDatasourceContainer(container *corev1.Container, tag *string, resources corev1.ResourceRequirements) {
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
	Expect(container.Resources).To(Equal(resources))
}

func checkReportsContainer(container *corev1.Container, tls bool, tag *string, resources corev1.ResourceRequirements) {
	Expect(container.Name).To(Equal("cryostat-reports"))
	if tag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat-reports:"))
	} else {
		Expect(container.Image).To(Equal(*tag))
	}
	Expect(container.Ports).To(ConsistOf(test.NewReportsPorts()))
	Expect(container.Env).To(ConsistOf(test.NewReportsEnvironmentVariables(tls, resources)))
	Expect(container.VolumeMounts).To(ConsistOf(test.NewReportsVolumeMounts(tls)))
	Expect(container.LivenessProbe).To(Equal(test.NewReportsLivenessProbe(tls)))
	Expect(container.Resources).To(Equal(resources))
}

func (t *cryostatTestInput) checkEnvironmentVariables(expectedEnvVars []corev1.EnvVar) {
	c := &operatorv1beta1.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, c)
	Expect(err).ToNot(HaveOccurred())

	deployment := &appsv1.Deployment{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: "cryostat", Namespace: "default"}, deployment)
	Expect(err).ToNot(HaveOccurred())

	template := deployment.Spec.Template
	coreContainer := template.Spec.Containers[0]

	Expect(coreContainer.Env).To(ContainElements(expectedEnvVars))
}
