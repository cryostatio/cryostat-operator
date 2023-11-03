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

package insights

import (
	insightstest "github.com/cryostatio/cryostat-operator/internal/controllers/insights/test"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type insightsUnitTestInput struct {
	client     ctrlclient.Client
	controller *InsightsReconciler
	objs       []ctrlclient.Object
	*insightstest.TestUtilsConfig
	*insightstest.InsightsTestResources
}

var _ = Describe("InsightsController", func() {
	var t *insightsUnitTestInput

	Describe("configuring watches", func() {

		BeforeEach(func() {
			t = &insightsUnitTestInput{
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

			config := &InsightsReconcilerConfig{
				Client:    test.NewClientWithTimestamp(test.NewTestClient(t.client, t.TestResources)),
				Scheme:    s,
				Log:       logger,
				Namespace: t.Namespace,
				OSUtils:   insightstest.NewTestOSUtils(t.TestUtilsConfig),
			}
			t.controller, err = NewInsightsReconciler(config)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("for secrets", func() {
			It("should reconcile global pull secret", func() {
				result := t.controller.isPullSecretOrProxyConfig(t.NewGlobalPullSecret())
				Expect(result).To(ConsistOf(t.deploymentReconcileRequest()))
			})
			It("should reconcile APICast secret", func() {
				result := t.controller.isPullSecretOrProxyConfig(t.NewInsightsProxySecret())
				Expect(result).To(ConsistOf(t.deploymentReconcileRequest()))
			})
			It("should not reconcile a secret in another namespace", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      t.NewGlobalPullSecret().Name,
						Namespace: "other",
					},
				}
				result := t.controller.isPullSecretOrProxyConfig(secret)
				Expect(result).To(BeEmpty())
			})
		})

		Context("for deployments", func() {
			It("should reconcile proxy deployment", func() {
				result := t.controller.isProxyDeployment(t.NewInsightsProxyDeployment())
				Expect(result).To(ConsistOf(t.deploymentReconcileRequest()))
			})
			It("should not reconcile a deployment in another namespace", func() {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      t.NewInsightsProxyDeployment().Name,
						Namespace: "other",
					},
				}
				result := t.controller.isProxyDeployment(deploy)
				Expect(result).To(BeEmpty())
			})
		})

		Context("for services", func() {
			It("should reconcile proxy service", func() {
				result := t.controller.isProxyService(t.NewInsightsProxyService())
				Expect(result).To(ConsistOf(t.deploymentReconcileRequest()))
			})
			It("should not reconcile a service in another namespace", func() {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      t.NewInsightsProxyService().Name,
						Namespace: "other",
					},
				}
				result := t.controller.isProxyService(svc)
				Expect(result).To(BeEmpty())
			})
		})
	})
})

func (t *insightsUnitTestInput) deploymentReconcileRequest() reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: "insights-proxy", Namespace: t.Namespace}}
}
