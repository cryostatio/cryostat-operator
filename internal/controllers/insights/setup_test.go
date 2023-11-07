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

package insights_test

import (
	"context"

	"github.com/cryostatio/cryostat-operator/internal/controllers/insights"
	insightstest "github.com/cryostatio/cryostat-operator/internal/controllers/insights/test"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("InsightsIntegration", func() {
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
})
