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

package insights_test

import (
	"context"

	"github.com/cryostatio/cryostat-operator/internal/controllers/insights"
	insightstest "github.com/cryostatio/cryostat-operator/internal/controllers/insights/test"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type insightsTestInput struct {
	client     ctrlclient.Client
	controller *insights.InsightsReconciler
	objs       []ctrlclient.Object
	*insightstest.TestUtilsConfig
	*insightstest.InsightsTestResources
}

var _ = Describe("InsightsController", func() {
	var t *insightsTestInput

	Describe("setting up", func() {
		var integration *insights.InsightsIntegration

		BeforeEach(func() {
			t = &insightsTestInput{
				TestUtilsConfig: &insightstest.TestUtilsConfig{
					EnvInsightsEnabled:       &[]bool{true}[0],
					EnvInsightsBackendDomain: &[]string{"insights.example.com"}[0],
					EnvInsightsProxyImageTag: &[]string{"example.com/proxy:latest"}[0],
					EnvNamespace:             &[]string{"test"}[0],
				},
				InsightsTestResources: &insightstest.InsightsTestResources{
					TestResources: &test.TestResources{
						Namespace: "test",
					},
				},
			}
			t.objs = []ctrlclient.Object{
				t.NewNamespace(),
				t.NewGlobalPullSecret(),
				t.NewOperatorDeployment(),
			}
		})

		JustBeforeEach(func() {
			s := test.NewTestScheme()
			logger := zap.New()
			logf.SetLogger(logger)

			// Set a CreationTimestamp for created objects to match a real API server
			// TODO When using envtest instead of fake client, this is probably no longer needed
			err := test.SetCreationTimestamp(t.objs...)
			Expect(err).ToNot(HaveOccurred())
			t.client = fake.NewClientBuilder().WithScheme(s).WithObjects(t.objs...).Build()

			manager := insightstest.NewFakeManager(test.NewClientWithTimestamp(test.NewTestClient(t.client, t.TestResources)),
				s, &logger)
			integration = insights.NewInsightsIntegration(manager, &logger)
			integration.OSUtils = insightstest.NewTestOSUtils(t.TestUtilsConfig)
		})

		Context("with defaults", func() {
			It("should return proxy URL", func() {
				result, err := integration.Setup()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.String()).To(Equal("http://insights-proxy.test.svc.cluster.local"))
			})

			It("should create config map", func() {
				_, err := integration.Setup()
				Expect(err).ToNot(HaveOccurred())

				expected := t.NewProxyConfigMap()
				actual := &corev1.ConfigMap{}
				err = t.client.Get(context.Background(), types.NamespacedName{
					Name:      expected.Name,
					Namespace: expected.Namespace,
				}, actual)
				Expect(err).ToNot(HaveOccurred())

				Expect(actual.Labels).To(Equal(expected.Labels))
				Expect(actual.Annotations).To(Equal(expected.Annotations))
				Expect(metav1.IsControlledBy(actual, t.NewOperatorDeployment())).To(BeTrue())
				Expect(actual.Data).To(BeEmpty())
			})
		})

		Context("with Insights disabled", func() {
			BeforeEach(func() {
				t.EnvInsightsEnabled = &[]bool{false}[0]
				t.objs = append(t.objs,
					t.NewProxyConfigMap(),
				)
			})

			It("should return nil", func() {
				result, err := integration.Setup()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeNil())
			})

			It("should delete config map", func() {
				_, err := integration.Setup()
				Expect(err).ToNot(HaveOccurred())

				expected := t.NewProxyConfigMap()
				actual := &corev1.ConfigMap{}
				err = t.client.Get(context.Background(), types.NamespacedName{
					Name:      expected.Name,
					Namespace: expected.Namespace,
				}, actual)
				Expect(err).To(HaveOccurred())
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("when run out-of-cluster", func() {
			BeforeEach(func() {
				t.EnvNamespace = nil
			})

			It("should return nil", func() {
				result, err := integration.Setup()
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeNil())
			})

			It("should not create config map", func() {
				_, err := integration.Setup()
				Expect(err).ToNot(HaveOccurred())

				expected := t.NewProxyConfigMap()
				actual := &corev1.ConfigMap{}
				err = t.client.Get(context.Background(), types.NamespacedName{
					Name:      expected.Name,
					Namespace: expected.Namespace,
				}, actual)
				Expect(err).To(HaveOccurred())
				Expect(kerrors.IsNotFound(err)).To(BeTrue())
			})
		})
	})

	Describe("reconciling a request", func() {

		BeforeEach(func() {
			t = &insightsTestInput{
				TestUtilsConfig: &insightstest.TestUtilsConfig{
					EnvInsightsEnabled:       &[]bool{true}[0],
					EnvInsightsBackendDomain: &[]string{"insights.example.com"}[0],
					EnvInsightsProxyImageTag: &[]string{"example.com/proxy:latest"}[0],
				},
				InsightsTestResources: &insightstest.InsightsTestResources{
					TestResources: &test.TestResources{
						Namespace: "test",
					},
				},
			}
			t.objs = []ctrlclient.Object{
				t.NewNamespace(),
				t.NewGlobalPullSecret(),
				t.NewOperatorDeployment(),
				t.NewProxyConfigMap(),
			}
		})

		JustBeforeEach(func() {
			s := test.NewTestScheme()
			logger := zap.New()
			logf.SetLogger(logger)

			// Set a CreationTimestamp for created objects to match a real API server
			// TODO When using envtest instead of fake client, this is probably no longer needed
			err := test.SetCreationTimestamp(t.objs...)
			Expect(err).ToNot(HaveOccurred())
			t.client = fake.NewClientBuilder().WithScheme(s).WithObjects(t.objs...).Build()

			config := &insights.InsightsReconcilerConfig{
				Client:    test.NewClientWithTimestamp(test.NewTestClient(t.client, t.TestResources)),
				Scheme:    s,
				Log:       logger,
				Namespace: t.Namespace,
				OSUtils:   insightstest.NewTestOSUtils(t.TestUtilsConfig),
			}
			t.controller, err = insights.NewInsightsReconciler(config)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("successfully creates required resources", func() {
			Context("with defaults", func() {
				JustBeforeEach(func() {
					result, err := t.reconcile()
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(reconcile.Result{}))
				})
				It("should create the APICast config secret", func() {
					expected := t.NewInsightsProxySecret()
					actual := &corev1.Secret{}
					err := t.client.Get(context.Background(), types.NamespacedName{
						Name:      expected.Name,
						Namespace: expected.Namespace,
					}, actual)
					Expect(err).ToNot(HaveOccurred())

					Expect(actual.Labels).To(Equal(expected.Labels))
					Expect(actual.Annotations).To(Equal(expected.Annotations))
					Expect(metav1.IsControlledBy(actual, t.NewProxyConfigMap())).To(BeTrue())
					Expect(actual.StringData).To(HaveLen(1))
					Expect(actual.StringData).To(HaveKey("config.json"))
					Expect(actual.StringData["config.json"]).To(MatchJSON(expected.StringData["config.json"]))
				})
				It("should create the proxy deployment", func() {
					expected := t.NewInsightsProxyDeployment()
					actual := &appsv1.Deployment{}
					err := t.client.Get(context.Background(), types.NamespacedName{
						Name:      expected.Name,
						Namespace: expected.Namespace,
					}, actual)
					Expect(err).ToNot(HaveOccurred())

					Expect(actual.Labels).To(Equal(expected.Labels))
					Expect(actual.Annotations).To(Equal(expected.Annotations))
					Expect(metav1.IsControlledBy(actual, t.NewProxyConfigMap())).To(BeTrue())
					Expect(actual.Spec.Selector).To(Equal(expected.Spec.Selector))

					expectedTemplate := expected.Spec.Template
					actualTemplate := actual.Spec.Template
					Expect(actualTemplate.Labels).To(Equal(expectedTemplate.Labels))
					Expect(actualTemplate.Annotations).To(Equal(expectedTemplate.Annotations))
					Expect(actualTemplate.Spec.SecurityContext).To(Equal(expectedTemplate.Spec.SecurityContext))
					Expect(actualTemplate.Spec.Volumes).To(Equal(expectedTemplate.Spec.Volumes))

					Expect(actualTemplate.Spec.Containers).To(HaveLen(1))
					expectedContainer := expectedTemplate.Spec.Containers[0]
					actualContainer := actualTemplate.Spec.Containers[0]
					Expect(actualContainer.Ports).To(ConsistOf(expectedContainer.Ports))
					Expect(actualContainer.Env).To(ConsistOf(expectedContainer.Env))
					Expect(actualContainer.EnvFrom).To(ConsistOf(expectedContainer.EnvFrom))
					Expect(actualContainer.VolumeMounts).To(ConsistOf(expectedContainer.VolumeMounts))
					Expect(actualContainer.LivenessProbe).To(Equal(expectedContainer.LivenessProbe))
					Expect(actualContainer.StartupProbe).To(Equal(expectedContainer.StartupProbe))
					Expect(actualContainer.SecurityContext).To(Equal(expectedContainer.SecurityContext))
					Expect(actualContainer.Resources).To(Equal(expectedContainer.Resources))
				})
				It("should create the proxy service", func() {
					expected := t.NewInsightsProxyService()
					actual := &corev1.Service{}
					err := t.client.Get(context.Background(), types.NamespacedName{
						Name:      expected.Name,
						Namespace: expected.Namespace,
					}, actual)
					Expect(err).ToNot(HaveOccurred())

					Expect(actual.Labels).To(Equal(expected.Labels))
					Expect(actual.Annotations).To(Equal(expected.Annotations))
					Expect(metav1.IsControlledBy(actual, t.NewProxyConfigMap())).To(BeTrue())

					Expect(actual.Spec.Selector).To(Equal(expected.Spec.Selector))
					Expect(actual.Spec.Type).To(Equal(expected.Spec.Type))
					Expect(actual.Spec.Ports).To(ConsistOf(expected.Spec.Ports))
				})
			})
		})
	})
})

func (t *insightsTestInput) reconcile() (reconcile.Result, error) {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "insights-proxy", Namespace: t.Namespace}}
	return t.controller.Reconcile(context.Background(), req)
}
