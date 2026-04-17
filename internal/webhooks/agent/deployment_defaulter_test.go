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

package agent_test

import (
	"context"
	"strconv"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	webhooktests "github.com/cryostatio/cryostat-operator/internal/webhooks/agent/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type deploymentDefaulterTestInput struct {
	client ctrlclient.Client
	objs   []ctrlclient.Object
	*webhooktests.AgentWebhookTestResources
}

var _ = Describe("PodDefaulter", func() {
	var t *deploymentDefaulterTestInput
	var otherNS string
	count := 0

	namespaceWithSuffix := func(name string) string {
		return name + "-agent-deployment-" + strconv.Itoa(count)
	}

	BeforeEach(func() {
		ns := namespaceWithSuffix("test")
		otherNS = namespaceWithSuffix("other")
		t = &deploymentDefaulterTestInput{
			AgentWebhookTestResources: &webhooktests.AgentWebhookTestResources{
				TestResources: &test.TestResources{
					Name:             "cryostat",
					Namespace:        ns,
					TargetNamespaces: []string{ns},
					TLS:              true,
				},
			},
		}
		t.objs = []ctrlclient.Object{
			t.NewNamespace(), t.NewOtherNamespace(otherNS),
		}
	})

	JustBeforeEach(func() {
		logger := zap.New()
		logf.SetLogger(logger)

		t.client = k8sClient
		for _, obj := range t.objs {
			err := t.client.Create(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	JustAfterEach(func() {
		for _, obj := range t.objs {
			err := ctrlclient.IgnoreNotFound(t.client.Delete(ctx, obj))
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		count++
	})

	Context("Configuring a Deployment", func() {

		var expectedDeployment *appsv1.Deployment
		var originalDeployment *appsv1.Deployment

		ExpectDeployment := func() {
			It("Should propagate autoconfig labels to pod template", func() {
				actual := t.getDeployment(expectedDeployment)
				actualTemplate := actual.Spec.Template
				Expect(actualTemplate.Labels).To(Equal(expectedDeployment.Spec.Template.Labels))
				Expect(actualTemplate.Labels["cryostat.io/namespace"]).To(Equal(expectedDeployment.Labels["cryostat.io/namespace"]))
				Expect(actualTemplate.Labels["cryostat.io/name"]).To(Equal(expectedDeployment.Labels["cryostat.io/name"]))
				Expect(actualTemplate.Labels["cryostat.io/log-level"]).To(Equal(expectedDeployment.Labels["cryostat.io/log-level"]))
				Expect(actualTemplate.Labels["cryostat.io/callback-port"]).To(Equal(expectedDeployment.Labels["cryostat.io/callback-port"]))
				Expect(actualTemplate.Labels["cryostat.io/container"]).To(Equal(expectedDeployment.Labels["cryostat.io/container"]))
				Expect(actualTemplate.Labels["cryostat.io/read-only"]).To(Equal(expectedDeployment.Labels["cryostat.io/read-only"]))
				Expect(actualTemplate.Labels["cryostat.io/java-options-var"]).To(Equal(expectedDeployment.Labels["cryostat.io/java-options-var"]))
				Expect(actualTemplate.Labels["cryostat.io/harvester-template"]).To(Equal(expectedDeployment.Labels["cryostat.io/harvester-template"]))
				Expect(actualTemplate.Labels["cryostat.io/harvester-max-age"]).To(Equal(expectedDeployment.Labels["cryostat.io/harvester-max-age"]))
				Expect(actualTemplate.Labels["cryostat.io/harvester-max-size"]).To(Equal(expectedDeployment.Labels["cryostat.io/harvester-max-size"]))
				Expect(actualTemplate.Labels["cryostat.io/smart-triggers"]).To(Equal(expectedDeployment.Labels["cryostat.io/smart-triggers"]))
				// Non Agent Autoconfig Labels should not be propagated
				_, exists := actualTemplate.Labels["other"]
				Expect(exists).To(Equal(false))
			})
		}

		Context("with a Cryostat CR", func() {
			JustBeforeEach(func() {
				cr := t.getCryostatInstance()
				cr.Status.TargetNamespaces = cr.Spec.TargetNamespaces
				t.updateCryostatInstanceStatus(cr)

				err := t.client.Create(ctx, originalDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("with TLS enabled", func() {
				BeforeEach(func() {
					t.objs = append(t.objs, t.NewCryostat().Object)
					originalDeployment = t.NewDeployment()
					expectedDeployment = t.NewMutatedDeployment()
				})

				ExpectDeployment()
			})
		})
	})
})

func (t *deploymentDefaulterTestInput) getCryostatInstance() *model.CryostatInstance {
	cr := &operatorv1beta2.Cryostat{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, cr)
	Expect(err).ToNot(HaveOccurred())
	return t.ConvertNamespacedToModel(cr)
}

func (t *deploymentDefaulterTestInput) getDeployment(expected *appsv1.Deployment) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, deployment)
	Expect(err).ToNot(HaveOccurred())
	return deployment
}

func (t *deploymentDefaulterTestInput) updateCryostatInstanceStatus(cr *model.CryostatInstance) {
	err := t.client.Status().Update(context.Background(), cr.Object)
	Expect(err).ToNot(HaveOccurred())
}
