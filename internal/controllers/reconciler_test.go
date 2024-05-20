// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers_test

import (
	"context"
	"fmt"
	"net/url"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	. "github.com/onsi/ginkgo/v2"
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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type controllerTest struct {
	constructorFunc func(*controllers.ReconcilerConfig) (controllers.CommonReconciler, error)
}

type cryostatTestInput struct {
	controller controllers.CommonReconciler
	objs       []ctrlclient.Object
	test.TestReconcilerConfig
	*test.TestResources
}

func (c *controllerTest) commonBeforeEach() *cryostatTestInput {
	t := &cryostatTestInput{
		TestReconcilerConfig: test.TestReconcilerConfig{
			GeneratedPasswords: []string{"auth_cookie_secret", "connection_key", "encryption_key", "object_storage", "jmx", "keystore"},
		},
		TestResources: &test.TestResources{
			Name:        "cryostat",
			Namespace:   "test",
			TLS:         true,
			ExternalTLS: true,
			OpenShift:   true,
		},
	}
	t.objs = []ctrlclient.Object{
		t.NewNamespace(),
		t.NewApiServer(),
	}
	return t
}

func (c *controllerTest) commonJustBeforeEach(t *cryostatTestInput) {
	s := test.NewTestScheme()

	// Set a CreationTimestamp for created objects to match a real API server
	// TODO When using envtest instead of fake client, this is probably no longer needed
	err := test.SetCreationTimestamp(t.objs...)
	Expect(err).ToNot(HaveOccurred())
	t.Client = fake.NewClientBuilder().WithScheme(s).WithObjects(t.objs...).
		WithStatusSubresource(&operatorv1beta2.Cryostat{}, &certv1.Certificate{}, &openshiftv1.Route{}).Build()
	t.controller, err = c.constructorFunc(t.newReconcilerConfig(s, t.Client))
	Expect(err).ToNot(HaveOccurred())
}

func (c *controllerTest) commonJustAfterEach(t *cryostatTestInput) {
	for _, obj := range t.objs {
		err := ctrlclient.IgnoreNotFound(t.Client.Delete(context.Background(), obj))
		Expect(err).ToNot(HaveOccurred())
	}
}

func (t *cryostatTestInput) newReconcilerConfig(scheme *runtime.Scheme, client ctrlclient.Client) *controllers.ReconcilerConfig {
	logger := zap.New()
	logf.SetLogger(logger)

	// Set InsightsURL in config, if provided
	var insightsURL *url.URL
	if len(t.InsightsURL) > 0 {
		url, err := url.Parse(t.InsightsURL)
		Expect(err).ToNot(HaveOccurred())
		insightsURL = url
	}
	return &controllers.ReconcilerConfig{
		Client:        test.NewClientWithTimestamp(test.NewTestClient(client, t.TestResources)),
		Scheme:        scheme,
		IsOpenShift:   t.OpenShift,
		EventRecorder: record.NewFakeRecorder(1024),
		RESTMapper:    test.NewTESTRESTMapper(),
		Log:           logger,
		ReconcilerTLS: test.NewTestReconcilerTLS(&t.TestReconcilerConfig),
		InsightsProxy: insightsURL,
	}
}

// resourceCheck contains an expectation function that tests the presence
// of an operator-controlled object, along with a human-readable name
// for the resource being tested.
type resourceCheck struct {
	expectFunc   func(*cryostatTestInput)
	resourceName string
}

// Group the expectations that check for successful creation or existence
// of resources in the happy path.
// Meant to be easily reused throughout tests.
func resourceChecks() []resourceCheck {
	return []resourceCheck{
		{(*cryostatTestInput).expectCertificates, "certificates"},
		{(*cryostatTestInput).expectRBAC, "RBAC"},
		{(*cryostatTestInput).expectRoutes, "routes"},
		{func(t *cryostatTestInput) {
			t.expectPVC(t.NewDefaultPVC())
		}, "persistent volume claim"},
		{(*cryostatTestInput).expectDatabaseSecret, "database secret"},
		{(*cryostatTestInput).expectStorageSecret, "object storage secret"},
		{(*cryostatTestInput).expectJMXSecret, "JMX secret"},
		{(*cryostatTestInput).expectCoreService, "core service"},
		{(*cryostatTestInput).expectMainDeployment, "main deployment"},
		{(*cryostatTestInput).expectLockConfigMap, "lock config map"},
	}
}

func expectSuccessful(t **cryostatTestInput) {
	for _, check := range resourceChecks() {
		check := check
		It(fmt.Sprintf("should create %s", check.resourceName), func() {
			check.expectFunc(*t)
		})
	}
	It("should set ApplicationURL in CR Status", func() {
		(*t).expectStatusApplicationURL()
	})
	It("should set Database Secret in CR Status", func() {
		(*t).expectStatusDatabaseSecret()
	})
	It("should set Storage Secret in CR Status", func() {
		(*t).expectStatusStorageSecret()
	})
	It("should set TLSSetupComplete condition", func() {
		(*t).checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
			"AllCertificatesReady")
	})
	Context("deployment is progressing", func() {
		JustBeforeEach(func() {
			(*t).makeDeploymentProgress((*t).Name)
		})
		It("should update conditions", func() {
			(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentAvailable, metav1.ConditionFalse,
				"TestAvailable")
			(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentProgressing, metav1.ConditionTrue,
				"TestProgressing")
			(*t).checkConditionAbsent(operatorv1beta2.ConditionTypeMainDeploymentReplicaFailure)
		})
		Context("then becomes available", func() {
			JustBeforeEach(func() {
				(*t).makeDeploymentAvailable((*t).Name)
			})
			It("should update conditions", func() {
				(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentAvailable, metav1.ConditionTrue,
					"TestAvailable")
				(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentProgressing, metav1.ConditionTrue,
					"TestProgressing")
				(*t).checkConditionAbsent(operatorv1beta2.ConditionTypeMainDeploymentReplicaFailure)
			})
		})
		Context("then fails to roll out", func() {
			JustBeforeEach(func() {
				(*t).makeDeploymentFail((*t).Name)
			})
			It("should update conditions", func() {
				(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentAvailable, metav1.ConditionFalse,
					"TestAvailable")
				(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentProgressing, metav1.ConditionFalse,
					"TestProgressing")
				(*t).checkConditionPresent(operatorv1beta2.ConditionTypeMainDeploymentReplicaFailure, metav1.ConditionTrue,
					"TestReplicaFailure")
			})
		})
	})
}

func (c *controllerTest) commonTests() {
	var t *cryostatTestInput

	Describe("reconciling a request in OpenShift", func() {
		BeforeEach(func() {
			t = c.commonBeforeEach()
			t.TargetNamespaces = []string{t.Namespace}
		})

		JustBeforeEach(func() {
			c.commonJustBeforeEach(t)
		})

		JustAfterEach(func() {
			c.commonJustAfterEach(t)
		})
		Context("with a default CR", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostat().Object)
			})

			It("should wait for certificates", func() {
				t.expectWaitingForCertificate()
			})

			Context("successfully creates required resources", func() {
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
				})
				expectSuccessful(&t)
			})
		})
		Context("with multiple namespaces", func() {
			// Use different names as well for cluster-scoped case
			names := []string{"cryostat-one", "cryostat-two"}
			namespaces := []string{"test-one", "test-two"}
			BeforeEach(func() {
				// Sanity check for test
				Expect(names).To(HaveLen(len(namespaces)))
				for i := range namespaces {
					t.Name = names[i]
					t.Namespace = namespaces[i]
					t.TargetNamespaces = []string{t.Namespace}
					t.objs = append(t.objs, t.NewNamespace(), t.NewCryostat().Object)
				}
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})

			for i := range namespaces {
				// capture values for closure
				name := names[i]
				ns := namespaces[i]
				Context(fmt.Sprintf("successfully creates required resources in namespace %s", ns), func() {
					BeforeEach(func() {
						t.Name = name
						t.Namespace = ns
						t.TargetNamespaces = []string{t.Namespace}
					})

					expectSuccessful(&t)
				})
			}
		})
		Context("after cryostat reconciled successfully", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should be idempotent", func() {
				t.expectIdempotence()
			})
		})
		Context("Cryostat does not exist", func() {
			It("should do nothing", func() {
				result, err := t.reconcileWithName("does-not-exist")
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
		Context("with an existing main Deployment", func() {
			var cr *model.CryostatInstance
			var oldDeploy *appsv1.Deployment
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldDeploy = t.OtherDeployment()
				t.objs = append(t.objs, cr.Object, oldDeploy)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the Deployment", func() {
				deploy := &appsv1.Deployment{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deploy)
				Expect(err).ToNot(HaveOccurred())

				Expect(deploy.Annotations).To(Equal(map[string]string{
					"app.openshift.io/connects-to": "cryostat-operator-controller-manager",
					"other":                        "annotation",
				}))
				Expect(deploy.Labels).To(Equal(map[string]string{
					"app":                    t.Name,
					"kind":                   "cryostat",
					"component":              "cryostat",
					"app.kubernetes.io/name": "cryostat",
					"other":                  "label",
				}))
				Expect(metav1.IsControlledBy(deploy, cr.Object)).To(BeTrue())

				t.checkMainPodTemplate(deploy, cr)

				// Deployment Selector is immutable
				Expect(deploy.Spec.Selector).To(Equal(oldDeploy.Spec.Selector))
				Expect(deploy.Spec.Replicas).To(Equal(&[]int32{1}[0]))
				Expect(deploy.Spec.Strategy).To(Equal(t.NewMainDeploymentStrategy()))
			})
			Context("with a different selector", func() {
				BeforeEach(func() {
					selector := metav1.AddLabelToSelector(&metav1.LabelSelector{}, "other", "label")
					oldDeploy.Spec.Selector = selector
				})
				It("should delete and recreate the deployment", func() {
					t.expectMainDeployment()
				})
			})
		})
		Context("with an existing Service Account", func() {
			var cr *model.CryostatInstance
			var oldSA *corev1.ServiceAccount
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldSA = t.OtherServiceAccount()
				t.objs = append(t.objs, cr.Object, oldSA)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the Service Account", func() {
				sa := &corev1.ServiceAccount{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, sa)
				Expect(err).ToNot(HaveOccurred())

				Expect(sa.Annotations).To(Equal(map[string]string{
					"hello": "world",
					"serviceaccounts.openshift.io/oauth-redirectreference.route": fmt.Sprintf(`{"metadata":{"creationTimestamp":null},"reference":{"group":"","kind":"Route","name":"%s"}}`, t.Name),
				}))

				Expect(sa.Labels).To(Equal(map[string]string{
					"app":   t.Name,
					"other": "label",
				}))

				Expect(metav1.IsControlledBy(sa, cr.Object)).To(BeTrue())

				Expect(sa.ImagePullSecrets).To(Equal(oldSA.ImagePullSecrets))
				Expect(sa.Secrets).To(Equal(oldSA.Secrets))
				Expect(sa.AutomountServiceAccountToken).To(Equal(oldSA.AutomountServiceAccountToken))
			})
		})
		Context("with an existing Role", func() {
			var role *rbacv1.Role
			Context("created by the operator", func() {
				BeforeEach(func() {
					cr := t.NewCryostat()
					role = t.NewRole()
					err := controllerutil.SetControllerReference(cr.Object, role, test.NewTestScheme())
					Expect(err).ToNot(HaveOccurred())
					t.objs = append(t.objs, cr.Object, role)
				})
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
				})
				It("should delete the Role", func() {
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, role)
					Expect(err).To(HaveOccurred())
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				})
			})
			Context("not created by the operator", func() {
				BeforeEach(func() {
					role = t.OtherRole()
					t.objs = append(t.objs, t.NewCryostat().Object, role)
				})
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
				})
				It("should not delete the Role", func() {
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, role)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
		Context("with an existing Role Binding", func() {
			var cr *model.CryostatInstance
			var oldBinding *rbacv1.RoleBinding
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldBinding = t.OtherRoleBinding(t.Namespace)
				t.objs = append(t.objs, cr.Object, oldBinding)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the Role Binding", func() {
				expected := t.NewRoleBinding(t.Namespace)
				binding := &rbacv1.RoleBinding{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, binding)
				Expect(err).ToNot(HaveOccurred())

				// Labels are unaffected
				Expect(binding.Labels).To(Equal(oldBinding.Labels))
				Expect(binding.Annotations).To(Equal(oldBinding.Annotations))

				// Subjects and RoleRef should be fully replaced
				Expect(binding.Subjects).To(Equal(expected.Subjects))
				Expect(binding.RoleRef).To(Equal(expected.RoleRef))
			})
			Context("with a different roleRef", func() {
				BeforeEach(func() {
					oldBinding.RoleRef = t.OtherRoleRef()
				})
				It("should delete and re-create the Role Binding", func() {
					t.expectRBAC()
				})
			})
		})
		Context("with an existing Cluster Role Binding", func() {
			var cr *model.CryostatInstance
			var oldBinding *rbacv1.ClusterRoleBinding
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldBinding = t.OtherClusterRoleBinding()
				t.objs = append(t.objs, cr.Object, oldBinding)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the Cluster Role Binding", func() {
				expected := t.NewClusterRoleBinding()
				binding := &rbacv1.ClusterRoleBinding{}
				err := t.Client.Get(context.Background(), types.NamespacedName{
					Name: expected.Name,
				}, binding)
				Expect(err).ToNot(HaveOccurred())

				// Labels and annotations are unaffected
				Expect(binding.Labels).To(Equal(oldBinding.Labels))
				Expect(binding.Annotations).To(Equal(oldBinding.Annotations))

				// Subjects and RoleRef should be fully replaced
				Expect(binding.Subjects).To(Equal(expected.Subjects))
				Expect(binding.RoleRef).To(Equal(expected.RoleRef))
			})
			Context("with a different roleRef", func() {
				BeforeEach(func() {
					oldBinding.RoleRef = t.OtherRoleRef()
				})
				It("should delete and re-create the Cluster Role Binding", func() {
					t.expectRBAC()
				})
			})
		})
		Context("with an existing JMX Secret", func() {
			var cr *model.CryostatInstance
			var oldSecret *corev1.Secret
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldSecret = t.OtherJMXSecret()
				t.objs = append(t.objs, cr.Object, oldSecret)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the username but not password", func() {
				secret := &corev1.Secret{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: oldSecret.Name, Namespace: t.Namespace}, secret)
				Expect(err).ToNot(HaveOccurred())

				Expect(metav1.IsControlledBy(secret, cr.Object)).To(BeTrue())

				// Username should be replaced, but not password
				expected := t.NewJMXSecret()
				Expect(secret.StringData["CRYOSTAT_RJMX_USER"]).To(Equal(expected.StringData["CRYOSTAT_RJMX_USER"]))
				Expect(secret.StringData["CRYOSTAT_RJMX_PASS"]).To(Equal(oldSecret.StringData["CRYOSTAT_RJMX_PASS"]))
			})
		})
		Context("with an existing Database Secret", func() {
			var cr *model.CryostatInstance
			var oldSecret *corev1.Secret
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldSecret = t.OtherDatabaseSecret()
				t.objs = append(t.objs, cr.Object, oldSecret)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should not update password", func() {
				secret := &corev1.Secret{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: oldSecret.Name, Namespace: t.Namespace}, secret)
				Expect(err).ToNot(HaveOccurred())

				Expect(metav1.IsControlledBy(secret, cr.Object)).To(BeTrue())
				Expect(secret.StringData["CONNECTION_KEY"]).To(Equal(oldSecret.StringData["CONNECTION_KEY"]))
			})
		})
		Context("with existing Routes", func() {
			var cr *model.CryostatInstance
			var oldCoreRoute *openshiftv1.Route
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldCoreRoute = t.OtherCoreRoute()
				t.objs = append(t.objs, cr.Object, oldCoreRoute)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the Routes", func() {
				expected := t.NewCoreRoute()
				metav1.SetMetaDataAnnotation(&expected.ObjectMeta, "custom", "annotation")
				metav1.SetMetaDataLabel(&expected.ObjectMeta, "custom", "label")
				t.checkRoute(expected)
			})
		})
		Context("with report generator service", func() {
			var cr *model.CryostatInstance
			BeforeEach(func() {
				t.ReportReplicas = 1
				cr = t.NewCryostat()
				t.objs = append(t.objs, cr.Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("with cert-manager disabled", func() {
				BeforeEach(func() {
					disable := false
					cr.Spec.EnableCertManager = &disable
					t.TLS = false
				})
				It("should configure deployment appropriately", func() {
					t.expectMainDeployment()
					t.checkReportsDeployment()
					t.checkService(t.Name+"-reports", t.NewReportsService())
				})
			})
			Context("with Scheduling options", func() {
				BeforeEach(func() {
					*cr = *t.NewCryostatWithReportsScheduling()
				})
				It("should configure deployment appropriately", func() {
					t.checkReportsDeployment()
				})
			})
			Context("with resource requirements", func() {
				Context("fully specified", func() {
					BeforeEach(func() {
						*cr = *t.NewCryostatWithReportsResources()
					})
					It("should configure deployment appropriately", func() {
						t.expectMainDeployment()
						t.checkReportsDeployment()
						t.checkService(t.Name+"-reports", t.NewReportsService())
					})
				})
				Context("with low limits", func() {
					BeforeEach(func() {
						*cr = *t.NewCryostatWithReportLowResourceLimit()
					})
					It("should configure deployment appropriately", func() {
						t.expectMainDeployment()
						t.checkReportsDeployment()
						t.checkService(t.Name+"-reports", t.NewReportsService())
					})
				})
			})
			Context("deployment is progressing", func() {
				JustBeforeEach(func() {
					t.makeDeploymentProgress(t.Name + "-reports")
				})
				It("should update conditions", func() {
					t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentAvailable, metav1.ConditionFalse,
						"TestAvailable")
					t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentProgressing, metav1.ConditionTrue,
						"TestProgressing")
					t.checkConditionAbsent(operatorv1beta2.ConditionTypeReportsDeploymentReplicaFailure)
				})
				Context("then becomes available", func() {
					JustBeforeEach(func() {
						t.makeDeploymentAvailable(t.Name + "-reports")
					})
					It("should update conditions", func() {
						t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentAvailable, metav1.ConditionTrue,
							"TestAvailable")
						t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentProgressing, metav1.ConditionTrue,
							"TestProgressing")
						t.checkConditionAbsent(operatorv1beta2.ConditionTypeReportsDeploymentReplicaFailure)
					})
				})
				Context("then fails to roll out", func() {
					JustBeforeEach(func() {
						t.makeDeploymentFail(t.Name + "-reports")
					})
					It("should update conditions", func() {
						t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentAvailable, metav1.ConditionFalse,
							"TestAvailable")
						t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentProgressing, metav1.ConditionFalse,
							"TestProgressing")
						t.checkConditionPresent(operatorv1beta2.ConditionTypeReportsDeploymentReplicaFailure, metav1.ConditionTrue,
							"TestReplicaFailure")
					})
				})
			})
		})
		Context("Switching from 0 report sidecars to 1", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := t.getCryostatInstance()

				t.ReportReplicas = 1
				cryostat.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{
					Replicas: t.ReportReplicas,
				}
				t.updateCryostatInstance(cryostat)

				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
				t.checkReportsDeployment()
				t.checkService(t.Name+"-reports", t.NewReportsService())
			})
		})
		Context("Switching from 1 report sidecar to 2", func() {
			BeforeEach(func() {
				t.ReportReplicas = 1
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := t.getCryostatInstance()

				t.ReportReplicas = 2
				cryostat.Spec.ReportOptions.Replicas = t.ReportReplicas
				t.updateCryostatInstance(cryostat)

				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
				t.checkReportsDeployment()
				t.checkService(t.Name+"-reports", t.NewReportsService())
			})
		})
		Context("Switching from 2 report sidecars to 1", func() {
			BeforeEach(func() {
				t.ReportReplicas = 2
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := t.getCryostatInstance()

				t.ReportReplicas = 1
				cryostat.Spec.ReportOptions.Replicas = t.ReportReplicas
				t.updateCryostatInstance(cryostat)

				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
				t.checkReportsDeployment()
				t.checkService(t.Name+"-reports", t.NewReportsService())
			})
		})
		Context("Switching from 1 report sidecar to 0", func() {
			BeforeEach(func() {
				t.ReportReplicas = 1
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
				t.makeDeploymentAvailable(t.Name + "-reports")

				cryostat := t.getCryostatInstance()

				t.ReportReplicas = 0
				cryostat.Spec.ReportOptions.Replicas = t.ReportReplicas
				t.updateCryostatInstance(cryostat)

				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
				t.expectNoService(t.Name + "-reports")
				t.expectNoReportsDeployment()
			})
			It("should remove conditions", func() {
				t.checkConditionAbsent(operatorv1beta2.ConditionTypeReportsDeploymentAvailable)
				t.checkConditionAbsent(operatorv1beta2.ConditionTypeReportsDeploymentProgressing)
				t.checkConditionAbsent(operatorv1beta2.ConditionTypeReportsDeploymentReplicaFailure)
			})
		})
		Context("Cryostat CR has list of certificate secrets", func() {
			var cr *model.CryostatInstance
			BeforeEach(func() {
				cr = t.NewCryostatWithSecrets()
				t.objs = append(t.objs, cr.Object, t.NewTestCertSecret("testCert1"),
					t.NewTestCertSecret("testCert2"))
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
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
				t.objs = append(t.objs, t.NewCryostat().Object, t.NewTestCertSecret("testCert1"),
					t.NewTestCertSecret("testCert2"))
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should update the corresponding deployment", func() {
				// Get Cryostat CR after reconciling
				cr := t.getCryostatInstance()

				// Update it with new TrustedCertSecrets
				cr.Spec.TrustedCertSecrets = t.NewCryostatWithSecrets().Spec.TrustedCertSecrets
				t.updateCryostatInstance(cr)

				t.reconcileCryostatFully()

				deployment := &appsv1.Deployment{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
				Expect(err).ToNot(HaveOccurred())

				volumes := deployment.Spec.Template.Spec.Volumes
				expectedVolumes := t.NewVolumesWithSecrets()
				Expect(volumes).To(ConsistOf(expectedVolumes))

				volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
				expectedVolumeMounts := t.NewCoreVolumeMounts()
				Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
			})
		})
		Context("Cryostat CR has list of event templates", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithTemplates().Object, t.NewTemplateConfigMap(),
					t.NewOtherTemplateConfigMap())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.checkDeploymentHasTemplates()
			})
		})
		Context("Cryostat CR has list of event templates with TLS disabled", func() {
			BeforeEach(func() {
				t.TLS = false
				cr := t.NewCryostatWithTemplates()
				certManager := false
				cr.Spec.EnableCertManager = &certManager
				t.objs = append(t.objs, cr.Object, t.NewTemplateConfigMap(),
					t.NewOtherTemplateConfigMap())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should add volumes and volumeMounts to deployment", func() {
				t.checkDeploymentHasTemplates()
			})
		})
		Context("Adding a template to the EventTemplates list", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostat().Object, t.NewTemplateConfigMap(),
					t.NewOtherTemplateConfigMap())
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("Should update the corresponding deployment", func() {
				// Get Cryostat CR after reconciling
				cr := t.getCryostatInstance()

				// Update it with new EventTemplates
				cr.Spec.EventTemplates = t.NewCryostatWithTemplates().Spec.EventTemplates
				t.updateCryostatInstance(cr)

				t.reconcileCryostatFully()
				t.checkDeploymentHasTemplates()
			})
		})
		Context("with custom PVC spec overriding all defaults", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithPVCSpec().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create the PVC with requested spec", func() {
				t.expectPVC(t.NewCustomPVC())
			})
		})
		Context("with custom PVC spec overriding some defaults", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithPVCSpecSomeDefault().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create the PVC with requested spec", func() {
				t.expectPVC(t.NewCustomPVCSomeDefault())
			})
		})
		Context("with custom PVC config with no spec", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithPVCLabelsOnly().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create the PVC with requested label", func() {
				t.expectPVC(t.NewDefaultPVCWithLabel())
			})
		})
		Context("with an existing PVC", func() {
			var oldPVC *corev1.PersistentVolumeClaim
			BeforeEach(func() {
				oldPVC = t.NewDefaultPVC()
				t.objs = append(t.objs, t.NewCryostatWithPVCSpec().Object, oldPVC)
			})
			Context("that successfully updates", func() {
				BeforeEach(func() {
					// Add some labels and annotations to test merging
					metav1.SetMetaDataLabel(&oldPVC.ObjectMeta, "my", "other-label")
					metav1.SetMetaDataLabel(&oldPVC.ObjectMeta, "another", "label")
					metav1.SetMetaDataAnnotation(&oldPVC.ObjectMeta, "my/custom", "other-annotation")
					metav1.SetMetaDataAnnotation(&oldPVC.ObjectMeta, "another/custom", "annotation")
				})
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
				})
				It("should update metadata and resource requests", func() {
					expected := t.NewDefaultPVC()
					metav1.SetMetaDataLabel(&expected.ObjectMeta, "my", "label")
					metav1.SetMetaDataLabel(&expected.ObjectMeta, "another", "label")
					metav1.SetMetaDataLabel(&expected.ObjectMeta, "app", t.Name)
					metav1.SetMetaDataAnnotation(&expected.ObjectMeta, "my/custom", "annotation")
					metav1.SetMetaDataAnnotation(&expected.ObjectMeta, "another/custom", "annotation")
					expected.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("10Gi")
					t.expectPVC(expected)
				})
			})
			Context("that fails to update", func() {
				JustBeforeEach(func() {
					// Replace client with one that fails to update the PVC
					invalidErr := kerrors.NewInvalid(schema.ParseGroupKind("PersistentVolumeClaim"), oldPVC.Name, field.ErrorList{
						field.Forbidden(field.NewPath("spec"), "test error"),
					})
					t.Client = test.NewClientWithUpdateError(t.Client, oldPVC, invalidErr)
					t.controller.GetConfig().Client = t.Client

					// Expect an Invalid status error after reconciling
					_, err := t.reconcile()
					Expect(err).To(HaveOccurred())
					Expect(kerrors.IsInvalid(err)).To(BeTrue())
				})
				It("should emit a PersistentVolumeClaimInvalid event", func() {
					recorder := t.controller.GetConfig().EventRecorder.(*record.FakeRecorder)
					var eventMsg string
					Expect(recorder.Events).To(Receive(&eventMsg))
					Expect(eventMsg).To(ContainSubstring("PersistentVolumeClaimInvalid"))
				})
			})
		})
		Context("with custom EmptyDir config", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithDefaultEmptyDir().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create the EmptyDir with default specs", func() {
				t.expectEmptyDir(t.NewDefaultEmptyDir())
			})
		})
		Context("with custom EmptyDir config with requested spec", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithEmptyDirSpec().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create the EmptyDir with requested specs", func() {
				t.expectEmptyDir(t.NewEmptyDirWithSpec())
			})
		})
		Context("with overriden image tags", func() {
			var mainDeploy, reportsDeploy *appsv1.Deployment
			BeforeEach(func() {
				t.ReportReplicas = 1
				t.objs = append(t.objs, t.NewCryostatWithReportsSvc().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
				mainDeploy = &appsv1.Deployment{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, mainDeploy)
				Expect(err).ToNot(HaveOccurred())
				reportsDeploy = &appsv1.Deployment{}
				err = t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-reports", Namespace: t.Namespace}, reportsDeploy)
				Expect(err).ToNot(HaveOccurred())
			})
			Context("for development", func() {
				BeforeEach(func() {
					coreImg := "my/core-image:1.0.0-SNAPSHOT"
					datasourceImg := "my/datasource-image:1.0.0-BETA25"
					grafanaImg := "my/grafana-image:1.0.0-dev"
					reportsImg := "my/reports-image:1.0.0-SNAPSHOT"
					storageImg := "my/storage-image:1.0.0-dev"
					databaseImg := "my/database-image:1.0.0-dev"
					oauth2ProxyImg := "my/auth-proxy:1.0.0-dev"
					openshiftAuthProxyImg := "my/openshift-auth-proxy:1.0.0-dev"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
					t.EnvDatabaseImageTag = &databaseImg
					t.EnvStorageImageTag = &storageImg
					t.EnvOAuth2ProxyImageTag = &oauth2ProxyImg
					t.EnvOpenShiftOAuthProxyImageTag = &openshiftAuthProxyImg
				})
				It("should create deployment with the expected tags", func() {
					t.expectMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to Always", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(6))
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
					storageImg := "my/storage-image:1.0.0"
					databaseImg := "my/database-image:1.0.0"
					oauth2ProxyImg := "my/authproxy-image:1.0.0"
					openshiftAuthProxyImg := "my/openshift-authproxy-image:1.0.0"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
					t.EnvDatabaseImageTag = &databaseImg
					t.EnvStorageImageTag = &storageImg
					t.EnvOAuth2ProxyImageTag = &oauth2ProxyImg
					t.EnvOpenShiftOAuthProxyImageTag = &openshiftAuthProxyImg
				})
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
				})
				It("should create deployment with the expected tags", func() {
					t.expectMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to IfNotPresent", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(6))
					for _, container := range containers {
						fmt.Println(container.Image)
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
					storageImg := "my/storage-image@sha256:8b23ca5e8c8a343789b8c14558a44a49d35ecd130c18e62edf0d1ad9ce88d37d"
					databaseImg := "my/database-image@sha256:8c23ca5e8c8a343789b8c14558a44a49d35ecd130c18e62edf0d1ad9ce88d37d"
					oauth2ProxyImage := "my/authproxy-image@sha256:8c23ca5e8c8a343789b8c14558a44a49d35ecd130c18e62edf0d1ad9ce88d37d"
					openshiftAuthProxyImage := "my/openshift-authproxy-image@sha256:8c23ca5e8c8a343789b8c14558a44a49d35ecd130c18e62edf0d1ad9ce88d37d"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
					t.EnvDatabaseImageTag = &databaseImg
					t.EnvStorageImageTag = &storageImg
					t.EnvOAuth2ProxyImageTag = &oauth2ProxyImage
					t.EnvOpenShiftOAuthProxyImageTag = &openshiftAuthProxyImage
				})
				It("should create deployment with the expected tags", func() {
					t.expectMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to IfNotPresent", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(6))
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
					oauth2ProxyImg := "my/authproxy-image:latest"
					openshiftAuthProxyImg := "my/openshift-authproxy-image:latest"
					t.EnvCoreImageTag = &coreImg
					t.EnvDatasourceImageTag = &datasourceImg
					t.EnvGrafanaImageTag = &grafanaImg
					t.EnvReportsImageTag = &reportsImg
					t.EnvOAuth2ProxyImageTag = &oauth2ProxyImg
					t.EnvOpenShiftOAuthProxyImageTag = &openshiftAuthProxyImg
				})
				It("should create deployment with the expected tags", func() {
					t.expectMainDeployment()
					t.checkReportsDeployment()
				})
				It("should set ImagePullPolicy to Always", func() {
					containers := mainDeploy.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(6))
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
				t.objs = append(t.objs, t.NewCryostat().Object)
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
					err := t.Client.Delete(context.Background(), t.NewClusterRoleBinding())
					Expect(err).ToNot(HaveOccurred())
					t.reconcileDeletedCryostat()
				})
				It("should delete Cryostat", func() {
					t.expectNoCryostat()
				})
			})
			Context("RoleBinding exists", func() {
				JustBeforeEach(func() {
					t.reconcileDeletedCryostat()
				})
				It("should delete the RoleBinding", func() {
					t.checkRoleBindingsDeleted()
				})
				It("should delete Cryostat", func() {
					t.expectNoCryostat()
				})
			})
			Context("RoleBinding does not exist", func() {
				JustBeforeEach(func() {
					err := t.Client.Delete(context.Background(), t.NewRoleBinding(t.Namespace))
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
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create ConsoleLink", func() {
				t.expectConsoleLink()
			})
			Context("with an existing ConsoleLink", func() {
				var oldLink *consolev1.ConsoleLink
				BeforeEach(func() {
					oldLink = t.OtherConsoleLink()
					t.objs = append(t.objs, oldLink)
				})
				It("should update the ConsoleLink", func() {
					link := &consolev1.ConsoleLink{}
					expectedLink := t.NewConsoleLink()
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedLink.Name}, link)
					Expect(err).ToNot(HaveOccurred())
					// Existing labels and annotations should remain
					Expect(link.Labels).To(Equal(oldLink.Labels))
					Expect(link.Annotations).To(Equal(oldLink.Annotations))

					// Check managed spec fields
					Expect(link.Spec.Link).To(Equal(expectedLink.Spec.Link))
					Expect(link.Spec.Location).To(Equal(expectedLink.Spec.Location))
					Expect(link.Spec.NamespaceDashboard).To(Equal(expectedLink.Spec.NamespaceDashboard))
				})
			})
			Context("with an existing application url in APIServer AdditionalCORSAllowedOrigins", func() {
				BeforeEach(func() {
					t.objs = []ctrlclient.Object{
						t.NewApiServerWithApplicationURL(),
						t.NewNamespace(),
					}
				})
				It("should remove the application url", func() {
					apiServer := &configv1.APIServer{}
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cluster"}, apiServer)
					Expect(err).ToNot(HaveOccurred())
					Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).ToNot(ContainElement(fmt.Sprintf("https://%s\\.example\\.com", t.Name)))
					Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).To(ContainElement("https://an-existing-user-specified\\.allowed\\.origin\\.com"))
				})
			})
			It("should add the finalizer", func() {
				t.expectCryostatFinalizerPresent()
			})
			Context("with restricted SCC", func() {
				BeforeEach(func() {
					t.objs = []ctrlclient.Object{
						t.NewCryostat().Object, t.NewNamespaceWithSCCSupGroups(), t.NewApiServer(),
					}
				})
				It("should set fsGroup to value derived from namespace", func() {
					deploy := &appsv1.Deployment{}
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deploy)
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
						expectedLink := t.NewConsoleLink()
						err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedLink.Name}, link)
						Expect(kerrors.IsNotFound(err)).To(BeTrue())
					})
					It("should remove the application url from APIServer AdditionalCORSAllowedOrigins", func() {
						apiServer := &configv1.APIServer{}
						err := t.Client.Get(context.Background(), types.NamespacedName{Name: "cluster"}, apiServer)
						Expect(err).ToNot(HaveOccurred())
						Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).ToNot(ContainElement(fmt.Sprintf("https://%s\\.example\\.com", t.Name)))
						Expect(apiServer.Spec.AdditionalCORSAllowedOrigins).To(ContainElement("https://an-existing-user-specified\\.allowed\\.origin\\.com"))
					})
					It("should delete Cryostat", func() {
						t.expectNoCryostat()
					})
				})
				Context("ConsoleLink does not exist", func() {
					JustBeforeEach(func() {
						err := t.Client.Delete(context.Background(), t.NewConsoleLink())
						Expect(err).ToNot(HaveOccurred())
						t.reconcileDeletedCryostat()
					})
					It("should delete Cryostat", func() {
						t.expectNoCryostat()
					})
				})
			})
			Context("with Insights enabled", func() {
				BeforeEach(func() {
					t.InsightsURL = "http://insights-proxy.foo.svc.cluster.local"
				})
				JustBeforeEach(func() {
					t.reconcileCryostatFully()
				})
				It("should create deployment", func() {
					t.expectMainDeployment()
				})
			})
		})
		Context("with cert-manager disabled in CR", func() {
			BeforeEach(func() {
				t.TLS = false
				t.objs = append(t.objs, t.NewCryostatCertManagerDisabled().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create deployment and set owner", func() {
				t.expectMainDeployment()
			})
			It("should not create certificates", func() {
				certs := &certv1.CertificateList{}
				t.Client.List(context.Background(), certs, &ctrlclient.ListOptions{
					Namespace: t.Namespace,
				})
				Expect(certs.Items).To(BeEmpty())
			})
			It("should create routes with edge TLS termination", func() {
				t.expectRoutes()
			})
		})
		Context("with cert-manager not configured in CR", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatCertManagerUndefined().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create deployment and set owner", func() {
				t.expectMainDeployment()
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create routes with re-encrypt TLS termination", func() {
				t.expectRoutes()
			})
			It("should set TLSSetupComplete condition", func() {
				t.reconcileCryostatFully()
				t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"AllCertificatesReady")
			})
		})
		Context("with DISABLE_SERVICE_TLS=true", func() {
			BeforeEach(func() {
				disableTLS := true
				t.EnvDisableTLS = &disableTLS
				t.TLS = false
				t.objs = append(t.objs, t.NewCryostatCertManagerUndefined().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create deployment and set owner", func() {
				t.expectMainDeployment()
			})
			It("should not create certificates", func() {
				certs := &certv1.CertificateList{}
				t.Client.List(context.Background(), certs, &ctrlclient.ListOptions{
					Namespace: t.Namespace,
				})
				Expect(certs.Items).To(BeEmpty())
			})
			It("should create routes with edge TLS termination", func() {
				t.expectRoutes()
			})
			It("should set TLSSetupComplete Condition", func() {
				t.reconcileCryostatFully()
				t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"CertManagerDisabled")
			})
		})
		Context("Disable cert-manager after being enabled", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := t.getCryostatInstance()

				t.TLS = false
				certManager := false
				cryostat.Spec.EnableCertManager = &certManager
				t.updateCryostatInstance(cryostat)

				t.reconcileCryostatFully()
			})
			It("should update the deployment", func() {
				t.expectMainDeployment()
			})
			It("should create routes with edge TLS termination", func() {
				t.expectRoutes()
			})
			It("should set TLSSetupComplete Condition", func() {
				t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"CertManagerDisabled")
			})
		})
		Context("Enable cert-manager after being disabled", func() {
			BeforeEach(func() {
				t.TLS = false
				t.objs = append(t.objs, t.NewCryostatCertManagerDisabled().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()

				cryostat := t.getCryostatInstance()

				t.TLS = true
				certManager := true
				cryostat.Spec.EnableCertManager = &certManager
				t.updateCryostatInstance(cryostat)

				t.reconcileCryostatFully()
			})
			It("should update the deployment", func() {
				t.expectMainDeployment()
			})
			It("should create certificates", func() {
				t.expectCertificates()
			})
			It("should create routes with re-encrypt TLS termination", func() {
				t.expectRoutes()
			})
			It("should set TLSSetupComplete condition", func() {
				t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
					"AllCertificatesReady")
			})
		})
		Context("cert-manager missing", func() {
			JustBeforeEach(func() {
				// Replace with an empty RESTMapper
				t.controller.GetConfig().RESTMapper = meta.NewDefaultRESTMapper([]schema.GroupVersion{})
			})
			Context("and enabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
				})
				JustBeforeEach(func() {
					_, err := t.reconcile()
					Expect(err).To(HaveOccurred())
				})
				It("should emit a CertManagerUnavailable Event", func() {
					recorder := t.controller.GetConfig().EventRecorder.(*record.FakeRecorder)
					var eventMsg string
					Expect(recorder.Events).To(Receive(&eventMsg))
					Expect(eventMsg).To(ContainSubstring("CertManagerUnavailable"))
				})
				It("should set TLSSetupComplete Condition", func() {
					t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
						"CertManagerUnavailable")
				})
			})
			Context("and disabled", func() {
				BeforeEach(func() {
					t.TLS = false
					t.objs = append(t.objs, t.NewCryostatCertManagerDisabled().Object)
				})
				JustBeforeEach(func() {
					_, err := t.reconcile()
					Expect(err).ToNot(HaveOccurred())
				})
				It("should not emit a CertManagerUnavailable Event", func() {
					recorder := t.controller.GetConfig().EventRecorder.(*record.FakeRecorder)
					Expect(recorder.Events).ToNot(Receive())
				})
				It("should set TLSSetupComplete Condition", func() {
					t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
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
					t.objs = append(t.objs, t.NewCryostatWithCoreSvc().Object)
				})
				It("should create the service as described", func() {
					t.checkService(t.Name, t.NewCustomizedCoreService())
				})
			})
			Context("containing reports config", func() {
				BeforeEach(func() {
					t.ReportReplicas = 1
					t.objs = append(t.objs, t.NewCryostatWithReportsSvc().Object)
				})
				It("should create the service as described", func() {
					t.checkService(t.Name+"-reports", t.NewCustomizedReportsService())
				})
			})
			Context("and existing services", func() {
				var cr *model.CryostatInstance
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
				})
				JustBeforeEach(func() {
					// Fetch the current Cryostat CR
					current := t.getCryostatInstance()

					// Customize it with service options from the test specs
					*current.Spec = *cr.Spec
					t.updateCryostatInstance(current)

					t.reconcileCryostatFully()
				})
				Context("containing core config", func() {
					BeforeEach(func() {
						cr = t.NewCryostatWithCoreSvc()
					})
					It("should create the service as described", func() {
						t.checkService(t.Name, t.NewCustomizedCoreService())
					})
				})
				Context("containing reports config", func() {
					BeforeEach(func() {
						t.ReportReplicas = 1
						cr = t.NewCryostatWithReportsSvc()
					})
					It("should create the service as described", func() {
						t.checkService(t.Name+"-reports", t.NewCustomizedReportsService())
					})
				})
			})
		})
		Context("configuring environment variables with non-default spec values", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing JmxCacheOptions", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithJmxCacheOptionsSpec().Object)
				})
				It("should set JMX cache options", func() {
					t.checkCoreHasEnvironmentVariables(t.NewJmxCacheOptionsEnv())
				})
			})
		})
		Context("with resource requirements", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("fully specified", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithResources().Object)
				})
				It("should create expected deployment", func() {
					t.expectMainDeployment()
				})
			})
			Context("with low limits", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithLowResourceLimit().Object)
				})
				It("should create expected deployment", func() {
					t.expectMainDeployment()
				})
			})
		})
		Context("with network options", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing core config", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithCoreNetworkOptions().Object)
				})
				It("should create the route as described", func() {
					t.checkRoute(t.NewCustomCoreRoute())
				})
			})
		})
		Context("with security options", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing Cryostat security options", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithSecurityOptions().Object)
				})
				It("should add security context as described", func() {
					t.expectMainDeployment()
				})
			})
			Context("containing Report security options", func() {
				Context("with 0 report replica", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostatWithReportSecurityOptions().Object)
					})
					It("should add security context as described", func() {
						t.expectNoReportsDeployment()
					})
				})
				Context("with 1 report replicas", func() {
					BeforeEach(func() {
						t.ReportReplicas = 1
						cr := t.NewCryostatWithReportSecurityOptions()
						t.objs = append(t.objs, cr.Object)
					})
					It("should add security context as described", func() {
						t.checkReportsDeployment()
					})
				})

			})
		})
		Context("with Scheduling options", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithScheduling().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
			})

		})
		Context("with target discovery options provided", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("with built-in target discovery mechanism disabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithBuiltInDiscoveryDisabled().Object)
				})
				It("should configure deployment appropriately", func() {
					t.expectMainDeployment()
				})
			})
			Context("with discovery port configurations", func() {
				Context("with built-in values disabled", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostatWithBuiltInPortConfigDisabled().Object)
					})
					It("should configure deployment appropriately", func() {
						t.expectMainDeployment()
					})
				})
				Context("containing non-empty lists", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostatWithDiscoveryPortConfig().Object)
					})
					It("should configure deployment appropriately", func() {
						t.expectMainDeployment()
					})
				})
			})
		})
		Context("with secret provided for database", func() {
			var customSecret *corev1.Secret
			BeforeEach(func() {
				customSecret = t.NewCustomDatabaseSecret()
				t.objs = append(t.objs, t.NewCryostatWithDatabaseSecretProvided().Object, customSecret)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
			})
			It("should set Database Secret in CR Status", func() {
				instance := t.getCryostatInstance()
				Expect(instance.Status.DatabaseSecret).To(Equal(customSecret.Name))
			})
			It("should not generate default secret", func() {
				secret := &corev1.Secret{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-db", Namespace: t.Namespace}, secret)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
			Context("with an existing Database Secret", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewDatabaseSecret())
				})
				It("should not delete the existing Database Secret", func() {
					secret := &corev1.Secret{}
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-db", Namespace: t.Namespace}, secret)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
		Context("with an existing Cryostat CR", func() {
			var otherInput *cryostatTestInput

			BeforeEach(func() {
				otherInput = c.commonBeforeEach()
			})

			JustBeforeEach(func() {
				c.commonJustBeforeEach(otherInput)

				// Controllers need to share client to have shared view of objects
				otherInput.Client = t.Client
				config := otherInput.newReconcilerConfig(otherInput.Client.Scheme(), otherInput.Client)
				controller, err := c.constructorFunc(config)
				Expect(err).ToNot(HaveOccurred())
				otherInput.controller = controller

				// Reconcile conflicting namespaced Cryostat fully
				otherInput.reconcileCryostatFully()
			})

			JustAfterEach(func() {
				c.commonJustAfterEach(otherInput)
			})

			Context("installing into its target namespace", func() {
				BeforeEach(func() {
					// Set up the CRs so the Cryostat's namespace conflicts with the target
					// namespace of the existing Cryostat
					otherNS := "other-test"
					otherInput.Namespace = otherNS
					otherInput.TargetNamespaces = []string{otherNS, t.Namespace}
					t.TargetNamespaces = []string{t.Namespace}

					t.objs = append(t.objs, otherInput.NewNamespace(), t.NewCryostat().Object, otherInput.NewCryostat().Object)
					otherInput.objs = t.objs
				})

				JustBeforeEach(func() {
					// Try reconciling the new Cryostat
					t.reconcileCryostatFully()
				})

				// Existing Cryostat installation should be unaffected
				Context("existing installation", func() {
					expectSuccessful(&otherInput)
				})

				// New installation should also be unaffected
				Context("new installation", func() {
					expectSuccessful(&t)
				})
			})

			Context("with a common target namespace", func() {
				BeforeEach(func() {
					// Set up the CRs so the Cryostat's target namespace conflicts with the target
					// namespace of another Cryostat
					targetNS := "other-test-target"
					otherNS := "other-test"
					otherInput.Namespace = otherNS
					otherInput.TargetNamespaces = []string{otherNS, targetNS}
					t.TargetNamespaces = []string{t.Namespace, targetNS}

					t.objs = append(t.objs, t.NewOtherNamespace(targetNS), otherInput.NewNamespace(), t.NewCryostat().Object, otherInput.NewCryostat().Object)
					otherInput.objs = t.objs
				})

				JustBeforeEach(func() {
					// Try reconciling the new Cryostat
					t.reconcileCryostatFully()
				})

				// Existing Cryostat installation should be unaffected
				Context("existing installation", func() {
					expectSuccessful(&otherInput)
				})

				// New installation should also be unaffected
				Context("new installation", func() {
					expectSuccessful(&t)
				})
			})
		})

		Context("reconciling a multi-namespace request", func() {
			targetNamespaces := []string{"multi-test-one", "multi-test-two"}

			BeforeEach(func() {
				// Create Namespaces
				for _, ns := range targetNamespaces {
					t.objs = append(t.objs, t.NewOtherNamespace(ns))
				}
			})

			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})

			Context("with unchanged target namespaces", func() {
				BeforeEach(func() {
					t.TargetNamespaces = targetNamespaces
					t.objs = append(t.objs, t.NewCryostat().Object)
				})

				It("should create the expected main deployment", func() {
					t.expectMainDeployment()
				})

				It("should create CA Cert secret in each namespace", func() {
					t.expectCertificates()
				})

				It("should create RBAC in each namespace", func() {
					t.expectRBAC()
				})

				It("should update the target namespaces in Status", func() {
					t.expectTargetNamespaces()
				})

				Context("when deleted", func() {
					Context("RoleBindings exist", func() {
						JustBeforeEach(func() {
							t.reconcileDeletedCryostat()
						})
						It("should delete the RoleBindings", func() {
							t.checkRoleBindingsDeleted()
						})
						It("should delete Cryostat", func() {
							t.expectNoCryostat()
						})
					})
					Context("RoleBinding does not exist", func() {
						JustBeforeEach(func() {
							err := t.Client.Delete(context.Background(), t.NewRoleBinding(targetNamespaces[0]))
							Expect(err).ToNot(HaveOccurred())
							t.reconcileDeletedCryostat()
						})
						It("should delete Cryostat", func() {
							t.expectNoCryostat()
						})
					})
				})
			})

			Context("with removed target namespaces", func() {
				BeforeEach(func() {
					// Begin with RBAC set up for two namespaces,
					// and remove the second namespace from the spec
					t.TargetNamespaces = targetNamespaces[:1]
					cr := t.NewCryostat()
					*cr.TargetNamespaceStatus = targetNamespaces
					t.objs = append(t.objs, cr.Object,
						t.NewRoleBinding(targetNamespaces[0]),
						t.NewRoleBinding(targetNamespaces[1]),
						t.NewCACertSecret(targetNamespaces[0]),
						t.NewCACertSecret(targetNamespaces[1]))
				})
				It("should create the expected main deployment", func() {
					t.expectMainDeployment()
				})
				It("leave RBAC for the first namespace", func() {
					t.expectRBAC()
				})
				It("should remove RBAC from the second namespace", func() {
					binding := t.NewRoleBinding(targetNamespaces[1])
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}, binding)
					Expect(err).ToNot(BeNil())
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				})
				It("leave CA Cert secret for the first namespace", func() {
					t.expectCertificates()
				})
				It("should remove CA Cert secret from the second namespace", func() {
					secret := t.NewCACertSecret(targetNamespaces[1])
					err := t.Client.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret)
					Expect(err).ToNot(BeNil())
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				})
				It("should update the target namespaces in Status", func() {
					t.expectTargetNamespaces()
				})
			})

			Context("with no target namespaces", func() {
				BeforeEach(func() {
					t.TargetNamespaces = nil
					t.objs = append(t.objs, t.NewCryostat().Object)
				})
				It("should update the target namespaces in Status", func() {
					t.expectTargetNamespaces()
				})
			})
		})
	})

	Describe("reconciling a request in Kubernetes", func() {
		BeforeEach(func() {
			t = c.commonBeforeEach()
			t.TargetNamespaces = []string{t.Namespace}
			t.OpenShift = false
		})

		JustBeforeEach(func() {
			c.commonJustBeforeEach(t)
		})

		JustAfterEach(func() {
			c.commonJustAfterEach(t)
		})
		Context("with TLS ingress", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostatWithIngress().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create ingresses", func() {
				t.expectIngresses()
			})
			It("should not create routes", func() {
				t.expectNoRoutes()
			})
			It("should create deployment and set owner", func() {
				t.expectMainDeployment()
			})
			It("should create RBAC", func() {
				t.expectRBAC()
			})
		})
		Context("with non-TLS ingress", func() {
			BeforeEach(func() {
				t.ExternalTLS = false
				t.objs = append(t.objs, t.NewCryostatWithIngress().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should create ingresses", func() {
				t.expectIngresses()
			})
			It("should not create routes", func() {
				t.expectNoRoutes()
			})
			It("should create deployment and set owner", func() {
				t.expectMainDeployment()
			})
			It("should create RBAC", func() {
				t.expectRBAC()
			})
		})
		Context("no ingress configuration is provided", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should not create ingresses or routes", func() {
				t.expectNoIngresses()
				t.expectNoRoutes()
			})
		})
		Context("with existing Ingresses", func() {
			var cr *model.CryostatInstance
			var oldCoreIngress *netv1.Ingress
			BeforeEach(func() {
				cr = t.NewCryostatWithIngress()
				oldCoreIngress = t.OtherCoreIngress()
				t.objs = append(t.objs, cr.Object, oldCoreIngress)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should update the Ingresses", func() {
				expected := t.NewCoreIngress()
				metav1.SetMetaDataAnnotation(&expected.ObjectMeta, "other", "annotation")
				metav1.SetMetaDataLabel(&expected.ObjectMeta, "other", "label")
				t.checkIngress(expected)
			})
		})
		Context("networkConfig for one of the services is nil", func() {
			var cr *model.CryostatInstance
			BeforeEach(func() {
				cr = t.NewCryostatWithIngress()
				t.objs = append(t.objs, cr.Object)
			})
			It("should only create specified ingresses", func() {
				c := t.getCryostatInstance()
				c.Spec.NetworkOptions.CoreConfig = nil
				t.updateCryostatInstance(c)

				t.reconcileCryostatFully()

				ingress := &netv1.Ingress{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, ingress)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("ingressSpec for one of the services is nil", func() {
			var cr *model.CryostatInstance
			BeforeEach(func() {
				cr = t.NewCryostatWithIngress()
				t.objs = append(t.objs, cr.Object)
			})
			It("should only create specified ingresses", func() {
				c := t.getCryostatInstance()
				c.Spec.NetworkOptions.CoreConfig.IngressSpec = nil
				t.updateCryostatInstance(c)

				t.reconcileCryostatFully()

				ingress := &netv1.Ingress{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, ingress)
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
		Context("with report generator service", func() {
			BeforeEach(func() {
				t.ReportReplicas = 1
				cr := t.NewCryostatWithIngress()
				t.objs = append(t.objs, cr.Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should configure deployment appropriately", func() {
				t.expectMainDeployment()
				t.checkReportsDeployment()
			})
			It("should create the reports service", func() {
				t.checkService(t.Name+"-reports", t.NewReportsService())
			})
		})
		Context("with security options", func() {
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			Context("containing Cryostat security options", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostatWithSecurityOptions().Object)
				})
				It("should add security context as described", func() {
					t.expectMainDeployment()
				})
			})
			Context("containing Report security options", func() {
				Context("with 0 report replica", func() {
					BeforeEach(func() {
						t.objs = append(t.objs, t.NewCryostatWithReportSecurityOptions().Object)
					})
					It("should add security context as described", func() {
						t.expectNoReportsDeployment()
					})
				})
				Context("with 1 report replicas", func() {
					BeforeEach(func() {
						t.ReportReplicas = 1
						t.objs = append(t.objs, t.NewCryostatWithReportSecurityOptions().Object)
					})
					It("should add security context as described", func() {
						t.checkReportsDeployment()
					})
				})

			})
		})
		Context("with an existing Service Account", func() {
			var cr *model.CryostatInstance
			var oldSA *corev1.ServiceAccount
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldSA = t.OtherServiceAccount()
				t.objs = append(t.objs, cr.Object, oldSA)
			})
			It("should update the Service Account", func() {
				t.reconcileCryostatFully()

				sa := &corev1.ServiceAccount{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, sa)
				Expect(err).ToNot(HaveOccurred())

				Expect(sa.Annotations).To(Equal(map[string]string{
					"hello": "world",
				}))

				Expect(sa.Labels).To(Equal(map[string]string{
					"app":   t.Name,
					"other": "label",
				}))

				Expect(metav1.IsControlledBy(sa, cr.Object)).To(BeTrue())

				Expect(sa.ImagePullSecrets).To(Equal(oldSA.ImagePullSecrets))
				Expect(sa.Secrets).To(Equal(oldSA.Secrets))
				Expect(sa.AutomountServiceAccountToken).To(Equal(oldSA.AutomountServiceAccountToken))
			})
		})
		Context("with an existing Role", func() {
			var role *rbacv1.Role
			Context("created by the operator", func() {
				BeforeEach(func() {
					cr := t.NewCryostat()
					role = t.NewRole()
					err := controllerutil.SetControllerReference(cr.Object, role, test.NewTestScheme())
					Expect(err).ToNot(HaveOccurred())
					t.objs = append(t.objs, cr.Object, role)
				})
				It("should delete the Role", func() {
					t.reconcileCryostatFully()

					err := t.Client.Get(context.Background(), types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, role)
					Expect(err).To(HaveOccurred())
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				})
			})
			Context("not created by the operator", func() {
				BeforeEach(func() {
					role = t.OtherRole()
					t.objs = append(t.objs, t.NewCryostat().Object, role)
				})
				It("should not delete the Role", func() {
					t.reconcileCryostatFully()

					err := t.Client.Get(context.Background(), types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, role)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
		Context("with an existing Role Binding", func() {
			var cr *model.CryostatInstance
			var oldBinding *rbacv1.RoleBinding
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldBinding = t.OtherRoleBinding(t.Namespace)
				t.objs = append(t.objs, cr.Object, oldBinding)
			})
			It("should update the Role Binding", func() {
				t.reconcileCryostatFully()

				expected := t.NewRoleBinding(t.Namespace)
				binding := &rbacv1.RoleBinding{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, binding)
				Expect(err).ToNot(HaveOccurred())

				// Labels are unaffected
				Expect(binding.Labels).To(Equal(oldBinding.Labels))
				Expect(binding.Annotations).To(Equal(oldBinding.Annotations))

				// Subjects and RoleRef should be fully replaced
				Expect(binding.Subjects).To(Equal(expected.Subjects))
				Expect(binding.RoleRef).To(Equal(expected.RoleRef))
			})
		})
		Context("with an existing Cluster Role Binding", func() {
			var cr *model.CryostatInstance
			var oldBinding *rbacv1.ClusterRoleBinding
			BeforeEach(func() {
				cr = t.NewCryostat()
				oldBinding = t.OtherClusterRoleBinding()
				t.objs = append(t.objs, cr.Object, oldBinding)
			})
			It("should update the Cluster Role Binding", func() {
				t.reconcileCryostatFully()

				expected := t.NewClusterRoleBinding()
				binding := &rbacv1.ClusterRoleBinding{}
				err := t.Client.Get(context.Background(), types.NamespacedName{
					Name: expected.Name,
				}, binding)
				Expect(err).ToNot(HaveOccurred())

				// Labels and annotations are unaffected
				Expect(binding.Labels).To(Equal(oldBinding.Labels))
				Expect(binding.Annotations).To(Equal(oldBinding.Annotations))

				// Subjects and RoleRef should be fully replaced
				Expect(binding.Subjects).To(Equal(expected.Subjects))
				Expect(binding.RoleRef).To(Equal(expected.RoleRef))
			})
		})
		Context("with additionnal label and annotation", func() {
			BeforeEach(func() {
				t.ReportReplicas = 1
				t.objs = append(t.objs, t.NewCryostatWithAdditionalMetadata().Object)
			})
			JustBeforeEach(func() {
				t.reconcileCryostatFully()
			})
			It("should have added the extra label and annotation to deployments and pods", func() {
				t.expectMainDeploymentHasExtraMetadata()
				t.expectReportsDeploymentHasExtraMetadata()
			})
		})
	})
}

func (t *cryostatTestInput) expectRoutes() {
	t.checkRoute(t.NewCoreRoute())
}

func (t *cryostatTestInput) checkRoute(expected *openshiftv1.Route) *openshiftv1.Route {
	route := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, route)
	Expect(err).ToNot(HaveOccurred())

	t.checkMetadata(route, expected)
	Expect(route.Spec.To).To(Equal(expected.Spec.To))
	Expect(route.Spec.Port).To(Equal(expected.Spec.Port))
	Expect(route.Spec.TLS).To(Equal(expected.Spec.TLS))
	return route
}

func (t *cryostatTestInput) checkConditionPresent(condType operatorv1beta2.CryostatConditionType, status metav1.ConditionStatus, reason string) {
	cr := t.getCryostatInstance()

	condition := meta.FindStatusCondition(cr.Status.Conditions, string(condType))
	Expect(condition).ToNot(BeNil())
	Expect(condition.Status).To(Equal(status))
	Expect(condition.Reason).To(Equal(reason))
}

func (t *cryostatTestInput) checkConditionAbsent(condType operatorv1beta2.CryostatConditionType) {
	cr := t.getCryostatInstance()

	condition := meta.FindStatusCondition(cr.Status.Conditions, string(condType))
	Expect(condition).To(BeNil())
}

func (t *cryostatTestInput) reconcileCryostatFully() {
	Eventually(func() reconcile.Result {
		result, err := t.reconcile()
		Expect(err).ToNot(HaveOccurred())
		return result
	}).WithTimeout(time.Minute).WithPolling(time.Millisecond).Should(Equal(reconcile.Result{}))
}

func (t *cryostatTestInput) reconcileDeletedCryostat() {
	cr := t.getCryostatInstance()

	// Check that the finalizer is set, then delete
	Expect(controllerutil.ContainsFinalizer(cr.Object, "operator.cryostat.io/cryostat.finalizer"))
	err := t.Client.Delete(context.Background(), cr.Object)
	Expect(err).ToNot(HaveOccurred())

	// Reconcile again
	t.reconcileCryostatFully()
}

func (t *cryostatTestInput) checkMetadata(object metav1.Object, expected metav1.Object) {
	t.checkMetadataNoOwner(object, expected)
	Expect(object.GetOwnerReferences()).To(HaveLen(1))
	Expect(metav1.IsControlledBy(object, t.getCryostatInstance().Object))
}

func (t *cryostatTestInput) checkMetadataNoOwner(object metav1.Object, expected metav1.Object) {
	Expect(object.GetName()).To(Equal(expected.GetName()))
	Expect(object.GetNamespace()).To(Equal(expected.GetNamespace()))
	Expect(object.GetLabels()).To(Equal(expected.GetLabels()))
	Expect(object.GetAnnotations()).To(Equal(expected.GetAnnotations()))
}

func (t *cryostatTestInput) expectNoCryostat() {
	_, err := t.lookupCryostatInstance()
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectWaitingForCertificate() {
	result, err := t.reconcile()
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Second}))

	// Check TLSSetupComplete condition
	t.checkConditionPresent(operatorv1beta2.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
		"WaitingForCertificate")
}

func (t *cryostatTestInput) expectCertificates() {
	// Check certificates
	certs := []*certv1.Certificate{t.NewCryostatCert(), t.NewCACert(), t.NewReportsCert()}
	for _, expected := range certs {
		actual := &certv1.Certificate{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, actual)
		Expect(err).ToNot(HaveOccurred())
		t.checkMetadata(actual, expected)
		Expect(actual.Spec).To(Equal(expected.Spec))
	}
	// Check issuers as well
	issuers := []*certv1.Issuer{t.NewSelfSignedIssuer(), t.NewCryostatCAIssuer()}
	for _, expected := range issuers {
		actual := &certv1.Issuer{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, actual)
		Expect(err).ToNot(HaveOccurred())
		t.checkMetadata(actual, expected)
		Expect(actual.Spec).To(Equal(expected.Spec))
	}
	// Check keystore secret
	expectedSecret := t.NewKeystoreSecret()
	secret := &corev1.Secret{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedSecret.Name, Namespace: expectedSecret.Namespace}, secret)
	Expect(err).ToNot(HaveOccurred())
	t.checkMetadata(secret, expectedSecret)
	Expect(secret.StringData).To(Equal(expectedSecret.StringData))

	// Check CA Cert secrets in each target namespace
	Expect(t.TargetNamespaces).ToNot(BeEmpty())
	for _, ns := range t.TargetNamespaces {
		if ns != t.Namespace {
			expectedSecret := t.NewCACertSecret(ns)
			secret := &corev1.Secret{}
			err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedSecret.Name, Namespace: ns}, secret)
			Expect(err).ToNot(HaveOccurred())
			t.checkMetadataNoOwner(secret, expectedSecret)
			Expect(secret.GetOwnerReferences()).To(BeEmpty())
			Expect(secret.Data).To(Equal(expectedSecret.Data))
			Expect(secret.Type).To(Equal(expectedSecret.Type))
		}
	}
}

func (t *cryostatTestInput) expectRBAC() {
	sa := &corev1.ServiceAccount{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, sa)
	Expect(err).ToNot(HaveOccurred())
	expectedSA := t.NewServiceAccount()
	t.checkMetadata(sa, expectedSA)
	Expect(sa.Secrets).To(Equal(expectedSA.Secrets))
	Expect(sa.ImagePullSecrets).To(Equal(expectedSA.ImagePullSecrets))
	Expect(sa.AutomountServiceAccountToken).To(Equal(expectedSA.AutomountServiceAccountToken))

	// Check for Role and RoleBinding in each target namespace
	Expect(t.TargetNamespaces).ToNot(BeEmpty()) // Sanity check for tests
	for _, ns := range t.TargetNamespaces {
		expectedBinding := t.NewRoleBinding(ns)
		binding := &rbacv1.RoleBinding{}
		err = t.Client.Get(context.Background(), types.NamespacedName{Name: expectedBinding.Name, Namespace: expectedBinding.Namespace}, binding)
		Expect(err).ToNot(HaveOccurred())
		t.checkMetadataNoOwner(binding, expectedBinding)
		Expect(binding.GetOwnerReferences()).To(BeEmpty())
		Expect(binding.Subjects).To(Equal(expectedBinding.Subjects))
		Expect(binding.RoleRef).To(Equal(expectedBinding.RoleRef))
	}

	expectedClusterBinding := t.NewClusterRoleBinding()
	clusterBinding := &rbacv1.ClusterRoleBinding{}
	err = t.Client.Get(context.Background(), types.NamespacedName{Name: expectedClusterBinding.Name}, clusterBinding)
	Expect(err).ToNot(HaveOccurred())
	Expect(clusterBinding.GetName()).To(Equal(expectedClusterBinding.GetName()))
	Expect(clusterBinding.GetNamespace()).To(Equal(expectedClusterBinding.GetNamespace()))
	Expect(clusterBinding.GetLabels()).To(Equal(expectedClusterBinding.GetLabels()))
	Expect(clusterBinding.GetAnnotations()).To(Equal(expectedClusterBinding.GetAnnotations()))
	Expect(clusterBinding.Subjects).To(Equal(expectedClusterBinding.Subjects))
	Expect(clusterBinding.RoleRef).To(Equal(expectedClusterBinding.RoleRef))
}

func (t *cryostatTestInput) checkClusterRoleBindingDeleted() {
	clusterBinding := &rbacv1.ClusterRoleBinding{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.NewClusterRoleBinding().Name}, clusterBinding)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) checkRoleBindingsDeleted() {
	for _, ns := range t.TargetNamespaces {
		expected := t.NewRoleBinding(ns)
		binding := &rbacv1.RoleBinding{}
		err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, binding)
		Expect(kerrors.IsNotFound(err)).To(BeTrue())
	}
}

func (t *cryostatTestInput) expectNoRoutes() {
	svc := &openshiftv1.Route{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, svc)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectIngresses() {
	t.checkIngress(t.NewCoreIngress())
}

func (t *cryostatTestInput) checkIngress(expected *netv1.Ingress) {
	ingress := &netv1.Ingress{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, ingress)
	Expect(err).ToNot(HaveOccurred())
	Expect(ingress.Annotations).To(Equal(expected.Annotations))
	Expect(ingress.Labels).To(Equal(expected.Labels))
	Expect(ingress.Spec).To(Equal(expected.Spec))
}

func (t *cryostatTestInput) expectNoIngresses() {
	ing := &netv1.Ingress{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, ing)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectLockConfigMap() {
	expected := t.NewLockConfigMap()
	cm := &corev1.ConfigMap{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, cm)
	Expect(err).ToNot(HaveOccurred())

	t.checkMetadata(cm, expected)
}

func (t *cryostatTestInput) expectPVC(expectedPVC *corev1.PersistentVolumeClaim) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, pvc)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	t.checkMetadata(pvc, expectedPVC)
	Expect(pvc.Spec.AccessModes).To(Equal(expectedPVC.Spec.AccessModes))
	Expect(pvc.Spec.StorageClassName).To(Equal(expectedPVC.Spec.StorageClassName))
	Expect(pvc.Spec.VolumeName).To(Equal(expectedPVC.Spec.VolumeName))
	Expect(pvc.Spec.VolumeMode).To(Equal(expectedPVC.Spec.VolumeMode))
	Expect(pvc.Spec.Selector).To(Equal(expectedPVC.Spec.Selector))
	Expect(pvc.Spec.DataSource).To(Equal(expectedPVC.Spec.DataSource))
	Expect(pvc.Spec.DataSourceRef).To(Equal(expectedPVC.Spec.DataSourceRef))

	pvcStorage := pvc.Spec.Resources.Requests["storage"]
	expectedPVCStorage := expectedPVC.Spec.Resources.Requests["storage"]
	Expect(pvcStorage.Equal(expectedPVCStorage)).To(BeTrue())
}

func (t *cryostatTestInput) expectEmptyDir(expectedEmptyDir *corev1.EmptyDirVolumeSource) {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volume := deployment.Spec.Template.Spec.Volumes[0]
	emptyDir := volume.EmptyDir

	// Compare to desired spec
	Expect(emptyDir.Medium).To(Equal(expectedEmptyDir.Medium))
	Expect(emptyDir.SizeLimit).To(Equal(expectedEmptyDir.SizeLimit))
}

func (t *cryostatTestInput) expectDatabaseSecret() {
	secret := &corev1.Secret{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-db", Namespace: t.Namespace}, secret)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	expectedSecret := t.NewDatabaseSecret()
	t.checkMetadata(secret, expectedSecret)
	Expect(secret.StringData).To(Equal(expectedSecret.StringData))
}

func (t *cryostatTestInput) expectStorageSecret() {
	secret := &corev1.Secret{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-storage-secret-key", Namespace: t.Namespace}, secret)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	expectedSecret := t.NewStorageSecret()
	t.checkMetadata(secret, expectedSecret)
	Expect(secret.StringData).To(Equal(expectedSecret.StringData))
}

func (t *cryostatTestInput) expectJMXSecret() {
	secret := &corev1.Secret{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-jmx-auth", Namespace: t.Namespace}, secret)
	Expect(err).ToNot(HaveOccurred())

	// Compare to desired spec
	expectedSecret := t.NewJMXSecret()
	t.checkMetadata(secret, expectedSecret)
	Expect(secret.StringData).To(Equal(expectedSecret.StringData))
}

func (t *cryostatTestInput) expectCoreService() {
	t.checkService(t.Name, t.NewCryostatService())
}

func (t *cryostatTestInput) expectStatusApplicationURL() {
	instance := t.getCryostatInstance()
	Expect(instance.Status.ApplicationURL).To(Equal(fmt.Sprintf("https://%s.example.com", t.Name)))
}

func (t *cryostatTestInput) expectStatusDatabaseSecret() {
	instance := t.getCryostatInstance()
	Expect(instance.Status.DatabaseSecret).To(Equal(fmt.Sprintf("%s-db", t.Name)))
}

func (t *cryostatTestInput) expectStatusStorageSecret() {
	instance := t.getCryostatInstance()
	Expect(instance.Status.StorageSecret).To(Equal(fmt.Sprintf("%s-storage-secret-key", t.Name)))
}

func (t *cryostatTestInput) expectDeploymentHasCertSecrets() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := t.NewVolumesWithSecrets()
	Expect(volumes).To(ConsistOf(expectedVolumes))

	volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	expectedVolumeMounts := t.NewCoreVolumeMounts()
	Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
}

func (t *cryostatTestInput) expectIdempotence() {
	obj := t.getCryostatInstance()

	// Reconcile again
	t.reconcileCryostatFully()

	obj2 := t.getCryostatInstance()
	Expect(obj2.Status).To(Equal(obj.Status))
	Expect(obj2.Spec).To(Equal(obj.Spec))
}

func (t *cryostatTestInput) expectCryostatFinalizerPresent() {
	cr := t.getCryostatInstance()
	Expect(cr.Object.GetFinalizers()).To(ContainElement("operator.cryostat.io/cryostat.finalizer"))
}

func (t *cryostatTestInput) checkService(svcName string, expected *corev1.Service) {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: svcName, Namespace: t.Namespace}, service)
	Expect(err).ToNot(HaveOccurred())

	t.checkMetadata(service, expected)
	Expect(service.Spec.Type).To(Equal(expected.Spec.Type))
	Expect(service.Spec.Selector).To(Equal(expected.Spec.Selector))
	Expect(service.Spec.Ports).To(Equal(expected.Spec.Ports))
}

func (t *cryostatTestInput) expectNoService(svcName string) {
	service := &corev1.Service{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: svcName, Namespace: t.Namespace}, service)
	Expect(kerrors.IsNotFound(err)).To(BeTrue())
}

func (t *cryostatTestInput) expectNoReportsDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-reports", Namespace: t.Namespace}, deployment)
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
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: deployName, Namespace: t.Namespace}, deploy)
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
	t.reconcileCryostatFully()
}

func (t *cryostatTestInput) expectMainDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	cr := t.getCryostatInstance()

	Expect(deployment.Name).To(Equal(t.Name))
	Expect(deployment.Namespace).To(Equal(t.Namespace))
	Expect(deployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to": "cryostat-operator-controller-manager",
	}))
	Expect(deployment.Labels).To(Equal(map[string]string{
		"app":                    t.Name,
		"kind":                   "cryostat",
		"component":              "cryostat",
		"app.kubernetes.io/name": "cryostat",
	}))
	Expect(metav1.IsControlledBy(deployment, cr.Object)).To(BeTrue())
	Expect(deployment.Spec.Selector).To(Equal(t.NewMainDeploymentSelector()))
	Expect(deployment.Spec.Replicas).ToNot(BeNil())
	Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
	Expect(deployment.Spec.Strategy).To(Equal(t.NewMainDeploymentStrategy()))

	// compare Pod template
	t.checkMainPodTemplate(deployment, cr)
}

func (t *cryostatTestInput) expectMainDeploymentHasExtraMetadata() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	cr := t.getCryostatInstance()

	Expect(deployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to":      "cryostat-operator-controller-manager",
		"myDeploymentExtraAnnotation":       "myDeploymentAnnotation",
		"mySecondDeploymentExtraAnnotation": "mySecondDeploymentAnnotation",
	}))
	Expect(deployment.Labels).To(Equal(map[string]string{
		"app":                          t.Name,
		"kind":                         "cryostat",
		"component":                    "cryostat",
		"app.kubernetes.io/name":       "cryostat",
		"myDeploymentExtraLabel":       "myDeploymentLabel",
		"mySecondDeploymentExtraLabel": "mySecondDeploymentLabel",
	}))
	// compare Pod template
	t.expectMainPodTemplateHasExtraMetadata(deployment, cr)
}

func (t *cryostatTestInput) checkMainPodTemplate(deployment *appsv1.Deployment, cr *model.CryostatInstance) {
	template := deployment.Spec.Template
	Expect(template.Name).To(Equal(t.Name))
	Expect(template.Namespace).To(Equal(t.Namespace))
	Expect(template.Labels).To(Equal(map[string]string{
		"app":       t.Name,
		"kind":      "cryostat",
		"component": "cryostat",
	}))
	Expect(template.Spec.Volumes).To(ConsistOf(t.NewVolumes()))
	Expect(template.Spec.SecurityContext).To(Equal(t.NewPodSecurityContext(cr)))

	// Check that the networking environment variables are set correctly
	coreContainer := template.Spec.Containers[0]
	port := int32(10000)
	if cr.Spec.ServiceOptions != nil && cr.Spec.ServiceOptions.ReportsConfig != nil &&
		cr.Spec.ServiceOptions.ReportsConfig.HTTPPort != nil {
		port = *cr.Spec.ServiceOptions.ReportsConfig.HTTPPort
	}
	var reportsUrl string
	if t.ReportReplicas == 0 {
		reportsUrl = ""
	} else if t.TLS {
		reportsUrl = fmt.Sprintf("https://%s-reports:%d", t.Name, port)
	} else {
		reportsUrl = fmt.Sprintf("http://%s-reports:%d", t.Name, port)
	}
	ingress := !t.OpenShift &&
		cr.Spec.NetworkOptions != nil && cr.Spec.NetworkOptions.CoreConfig != nil && cr.Spec.NetworkOptions.CoreConfig.IngressSpec != nil
	emptyDir := cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.EmptyDir != nil && cr.Spec.StorageOptions.EmptyDir.Enabled
	hasPortConfig := cr.Spec.TargetDiscoveryOptions != nil &&
		len(cr.Spec.TargetDiscoveryOptions.DiscoveryPortNames) > 0 &&
		len(cr.Spec.TargetDiscoveryOptions.DiscoveryPortNumbers) > 0
	builtInDiscoveryDisabled := cr.Spec.TargetDiscoveryOptions != nil && cr.Spec.TargetDiscoveryOptions.BuiltInDiscoveryDisabled
	builtInPortConfigDisabled := cr.Spec.TargetDiscoveryOptions != nil &&
		cr.Spec.TargetDiscoveryOptions.DisableBuiltInPortNames &&
		cr.Spec.TargetDiscoveryOptions.DisableBuiltInPortNumbers
	dbSecretProvided := cr.Spec.DatabaseOptions != nil && cr.Spec.DatabaseOptions.SecretName != nil

	t.checkCoreContainer(&coreContainer, ingress, reportsUrl,
		emptyDir,
		hasPortConfig,
		builtInDiscoveryDisabled,
		builtInPortConfigDisabled,
		dbSecretProvided,
		t.NewCoreContainerResource(cr), t.NewCoreSecurityContext(cr))

	// Check that Grafana is configured properly, depending on the environment
	grafanaContainer := template.Spec.Containers[1]
	t.checkGrafanaContainer(&grafanaContainer, t.NewGrafanaContainerResource(cr), t.NewGrafanaSecurityContext(cr))

	// Check that JFR Datasource is configured properly
	datasourceContainer := template.Spec.Containers[2]
	t.checkDatasourceContainer(&datasourceContainer, t.NewDatasourceContainerResource(cr), t.NewDatasourceSecurityContext(cr))

	// Check that Storage is configured properly
	storageContainer := template.Spec.Containers[3]
	t.checkStorageContainer(&storageContainer, t.NewStorageContainerResource(cr), t.NewStorageSecurityContext(cr))

	// Check that Database is configured properly
	databaseContainer := template.Spec.Containers[4]
	t.checkDatabaseContainer(&databaseContainer, nil, t.NewDatabaseSecurityContext(cr))

	// Check that Auth Proxy is configured properly
	authProxyContainer := template.Spec.Containers[5]
	t.checkAuthProxyContainer(&authProxyContainer, nil, t.NewAuthProxySecurityContext(cr))

	// Check that the proper Service Account is set
	Expect(template.Spec.ServiceAccountName).To(Equal(t.Name))

	if cr.Spec.SchedulingOptions != nil {
		scheduling := cr.Spec.SchedulingOptions
		Expect(template.Spec.NodeSelector).To(Equal(scheduling.NodeSelector))
		if scheduling.Affinity != nil {
			Expect(template.Spec.Affinity.PodAffinity).To(Equal(scheduling.Affinity.PodAffinity))
			Expect(template.Spec.Affinity.PodAntiAffinity).To(Equal(scheduling.Affinity.PodAntiAffinity))
			Expect(template.Spec.Affinity.NodeAffinity).To(Equal(scheduling.Affinity.NodeAffinity))
		}
		Expect(template.Spec.Tolerations).To(Equal(scheduling.Tolerations))
	}
}

func (t *cryostatTestInput) expectMainPodTemplateHasExtraMetadata(deployment *appsv1.Deployment, cr *model.CryostatInstance) {
	template := deployment.Spec.Template
	Expect(template.Labels).To(Equal(map[string]string{
		"app":                   t.Name,
		"kind":                  "cryostat",
		"component":             "cryostat",
		"myPodExtraLabel":       "myPodLabel",
		"myPodSecondExtraLabel": "myPodSecondLabel",
	}))
	Expect(template.Annotations).To(Equal(map[string]string{
		"myPodExtraAnnotation":       "myPodAnnotation",
		"mySecondPodExtraAnnotation": "mySecondPodAnnotation",
	}))
}

func (t *cryostatTestInput) checkReportsDeployment() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-reports", Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	cr := t.getCryostatInstance()

	Expect(deployment.Name).To(Equal(t.Name + "-reports"))
	Expect(deployment.Namespace).To(Equal(t.Namespace))
	Expect(deployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to": t.Name,
	}))
	Expect(deployment.Labels).To(Equal(map[string]string{
		"app":                    t.Name,
		"kind":                   "cryostat",
		"component":              "reports",
		"app.kubernetes.io/name": "cryostat-reports",
	}))
	Expect(metav1.IsControlledBy(deployment, cr.Object)).To(BeTrue())
	Expect(deployment.Spec.Selector).To(Equal(t.NewReportsDeploymentSelector()))
	Expect(deployment.Spec.Replicas).ToNot(BeNil())
	Expect(*deployment.Spec.Replicas).To(Equal(t.ReportReplicas))
	Expect(deployment.Spec.Strategy).To(BeZero())

	// compare Pod template
	template := deployment.Spec.Template
	Expect(template.Name).To(Equal(t.Name + "-reports"))
	Expect(template.Namespace).To(Equal(t.Namespace))
	Expect(template.Labels).To(Equal(map[string]string{
		"app":       t.Name,
		"kind":      "cryostat",
		"component": "reports",
	}))
	Expect(template.Spec.Volumes).To(ConsistOf(t.NewReportsVolumes()))
	Expect(template.Spec.SecurityContext).To(Equal(t.NewReportPodSecurityContext(cr)))

	t.checkReportsContainer(&template.Spec.Containers[0], t.NewReportContainerResource(cr), t.NewReportSecurityContext(cr))

	// Check that the default Service Account is used
	Expect(template.Spec.ServiceAccountName).To(BeEmpty())
	Expect(template.Spec.AutomountServiceAccountToken).To(BeNil())

	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SchedulingOptions != nil {
		scheduling := cr.Spec.ReportOptions.SchedulingOptions
		Expect(template.Spec.NodeSelector).To(Equal(scheduling.NodeSelector))
		if scheduling.Affinity != nil {
			Expect(template.Spec.Affinity.PodAffinity).To(Equal(scheduling.Affinity.PodAffinity))
			Expect(template.Spec.Affinity.PodAntiAffinity).To(Equal(scheduling.Affinity.PodAntiAffinity))
			Expect(template.Spec.Affinity.NodeAffinity).To(Equal(scheduling.Affinity.NodeAffinity))
		}
		Expect(template.Spec.Tolerations).To(Equal(scheduling.Tolerations))
	}
}

func (t *cryostatTestInput) expectReportsDeploymentHasExtraMetadata() {
	reportDeployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name + "-reports", Namespace: t.Namespace}, reportDeployment)
	Expect(err).ToNot(HaveOccurred())
	Expect(reportDeployment.Annotations).To(Equal(map[string]string{
		"app.openshift.io/connects-to":      t.Name,
		"myDeploymentExtraAnnotation":       "myDeploymentAnnotation",
		"mySecondDeploymentExtraAnnotation": "mySecondDeploymentAnnotation",
	}))
	Expect(reportDeployment.Labels).To(Equal(map[string]string{
		"app":                          t.Name,
		"kind":                         "cryostat",
		"component":                    "reports",
		"app.kubernetes.io/name":       "cryostat-reports",
		"myDeploymentExtraLabel":       "myDeploymentLabel",
		"mySecondDeploymentExtraLabel": "mySecondDeploymentLabel",
	}))
	template := reportDeployment.Spec.Template
	Expect(template.Labels).To(Equal(map[string]string{
		"app":                   t.Name,
		"kind":                  "cryostat",
		"component":             "reports",
		"myPodExtraLabel":       "myPodLabel",
		"myPodSecondExtraLabel": "myPodSecondLabel",
	}))
	Expect(template.Annotations).To(Equal(map[string]string{
		"myPodExtraAnnotation":       "myPodAnnotation",
		"mySecondPodExtraAnnotation": "mySecondPodAnnotation",
	}))
}

func (t *cryostatTestInput) checkDeploymentHasTemplates() {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := t.NewVolumesWithTemplates()
	Expect(volumes).To(ConsistOf(expectedVolumes))

	volumeMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	expectedVolumeMounts := t.NewVolumeMountsWithTemplates()
	Expect(volumeMounts).To(ConsistOf(expectedVolumeMounts))
}

func (t *cryostatTestInput) checkCoreContainer(container *corev1.Container, ingress bool,
	reportsUrl string,
	emptyDir bool,
	hasPortConfig bool, builtInDiscoveryDisabled bool, builtInPortConfigDisabled bool,
	dbSecretProvided bool,
	resources *corev1.ResourceRequirements,
	securityContext *corev1.SecurityContext) {
	Expect(container.Name).To(Equal(t.Name))
	if t.EnvCoreImageTag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat:"))
	} else {
		Expect(container.Image).To(Equal(*t.EnvCoreImageTag))
	}
	Expect(container.Ports).To(ConsistOf(t.NewCorePorts()))
	Expect(container.Env).To(ConsistOf(t.NewCoreEnvironmentVariables(reportsUrl, ingress, emptyDir, hasPortConfig, builtInDiscoveryDisabled, builtInPortConfigDisabled, dbSecretProvided)))
	Expect(container.EnvFrom).To(ConsistOf(t.NewCoreEnvFromSource()))
	Expect(container.VolumeMounts).To(ConsistOf(t.NewCoreVolumeMounts()))
	Expect(container.LivenessProbe).To(Equal(t.NewCoreLivenessProbe()))
	Expect(container.StartupProbe).To(Equal(t.NewCoreStartupProbe()))
	Expect(container.SecurityContext).To(Equal(securityContext))

	test.ExpectResourceRequirements(&container.Resources, resources)
}

func (t *cryostatTestInput) checkGrafanaContainer(container *corev1.Container, resources *corev1.ResourceRequirements, securityContext *corev1.SecurityContext) {
	Expect(container.Name).To(Equal(t.Name + "-grafana"))
	if t.EnvGrafanaImageTag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat-grafana-dashboard:"))
	} else {
		Expect(container.Image).To(Equal(*t.EnvGrafanaImageTag))
	}
	Expect(container.Ports).To(ConsistOf(t.NewGrafanaPorts()))
	Expect(container.Env).To(ConsistOf(t.NewGrafanaEnvironmentVariables()))
	Expect(container.VolumeMounts).To(BeEmpty())
	Expect(container.LivenessProbe).To(Equal(t.NewGrafanaLivenessProbe()))
	Expect(container.SecurityContext).To(Equal(securityContext))

	test.ExpectResourceRequirements(&container.Resources, resources)
}

func (t *cryostatTestInput) checkDatasourceContainer(container *corev1.Container, resources *corev1.ResourceRequirements, securityContext *corev1.SecurityContext) {
	Expect(container.Name).To(Equal(t.Name + "-jfr-datasource"))
	if t.EnvDatasourceImageTag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/jfr-datasource:"))
	} else {
		Expect(container.Image).To(Equal(*t.EnvDatasourceImageTag))
	}
	Expect(container.Ports).To(ConsistOf(t.NewDatasourcePorts()))
	Expect(container.Env).To(ConsistOf(t.NewDatasourceEnvironmentVariables()))
	Expect(container.EnvFrom).To(BeEmpty())
	Expect(container.VolumeMounts).To(BeEmpty())
	Expect(container.LivenessProbe).To(Equal(t.NewDatasourceLivenessProbe()))
	Expect(container.SecurityContext).To(Equal(securityContext))

	test.ExpectResourceRequirements(&container.Resources, resources)
}

func (t *cryostatTestInput) checkStorageContainer(container *corev1.Container, resources *corev1.ResourceRequirements, securityContext *corev1.SecurityContext) {
	Expect(container.Name).To(Equal(t.Name + "-storage"))
	if t.EnvStorageImageTag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat-storage:"))
	} else {
		Expect(container.Image).To(Equal(*t.EnvStorageImageTag))
	}
	Expect(container.Ports).To(ConsistOf(t.NewStoragePorts()))
	Expect(container.Env).To(ConsistOf(t.NewStorageEnvironmentVariables()))
	Expect(container.EnvFrom).To(BeEmpty())
	Expect(container.VolumeMounts).To(ConsistOf(t.NewStorageVolumeMounts()))
	Expect(container.LivenessProbe).To(Equal(t.NewStorageLivenessProbe()))
	Expect(container.SecurityContext).To(Equal(securityContext))

	test.ExpectResourceRequirements(&container.Resources, resources)
}

func (t *cryostatTestInput) checkDatabaseContainer(container *corev1.Container, resources *corev1.ResourceRequirements, securityContext *corev1.SecurityContext) {
	// TODO: Check Database Container
}

func (t *cryostatTestInput) checkAuthProxyContainer(container *corev1.Container, resources *corev1.ResourceRequirements, securityContext *corev1.SecurityContext) {
	// TODO: Check Auth Proxy container
}

func (t *cryostatTestInput) checkReportsContainer(container *corev1.Container, resources *corev1.ResourceRequirements, securityContext *corev1.SecurityContext) {
	Expect(container.Name).To(Equal(t.Name + "-reports"))
	if t.EnvReportsImageTag == nil {
		Expect(container.Image).To(HavePrefix("quay.io/cryostat/cryostat-reports:"))
	} else {
		Expect(container.Image).To(Equal(*t.EnvReportsImageTag))
	}
	Expect(container.Ports).To(ConsistOf(t.NewReportsPorts()))
	Expect(container.Env).To(ConsistOf(t.NewReportsEnvironmentVariables(resources)))
	Expect(container.VolumeMounts).To(ConsistOf(t.NewReportsVolumeMounts()))
	Expect(container.LivenessProbe).To(Equal(t.NewReportsLivenessProbe()))
	Expect(container.SecurityContext).To(Equal(securityContext))

	test.ExpectResourceRequirements(&container.Resources, resources)
}

func (t *cryostatTestInput) checkCoreHasEnvironmentVariables(expectedEnvVars []corev1.EnvVar) {
	deployment := &appsv1.Deployment{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())

	template := deployment.Spec.Template
	coreContainer := template.Spec.Containers[0]

	Expect(coreContainer.Env).To(ContainElements(expectedEnvVars))
}

func (t *cryostatTestInput) getCryostatInstance() *model.CryostatInstance {
	cr, err := t.lookupCryostatInstance()
	Expect(err).ToNot(HaveOccurred())
	return cr
}

func (t *cryostatTestInput) lookupCryostatInstance() (*model.CryostatInstance, error) {
	cr := &operatorv1beta2.Cryostat{}
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, cr)
	if err != nil {
		return nil, err
	}
	return t.ConvertNamespacedToModel(cr), nil
}

func (t *cryostatTestInput) updateCryostatInstance(cr *model.CryostatInstance) {
	err := t.Client.Update(context.Background(), cr.Object)
	Expect(err).ToNot(HaveOccurred())
}

func (t *cryostatTestInput) reconcile() (reconcile.Result, error) {
	return t.reconcileWithName(t.Name)
}

func (t *cryostatTestInput) reconcileWithName(name string) (reconcile.Result, error) {
	nsName := types.NamespacedName{Name: name, Namespace: t.Namespace}
	req := reconcile.Request{NamespacedName: nsName}
	return t.controller.Reconcile(context.Background(), req)
}

func (t *cryostatTestInput) expectConsoleLink() {
	link := &consolev1.ConsoleLink{}
	expectedLink := t.NewConsoleLink()
	err := t.Client.Get(context.Background(), types.NamespacedName{Name: expectedLink.Name}, link)
	Expect(err).ToNot(HaveOccurred())
	Expect(link.Spec).To(Equal(expectedLink.Spec))
}

func (t *cryostatTestInput) expectTargetNamespaces() {
	cr := t.getCryostatInstance()
	Expect(*cr.TargetNamespaceStatus).To(ConsistOf(t.TargetNamespaces))
}
