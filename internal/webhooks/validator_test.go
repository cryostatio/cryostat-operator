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

package webhooks_test

import (
	"fmt"
	"strconv"

	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	"github.com/cryostatio/cryostat-operator/internal/webhooks"
	webhooktests "github.com/cryostatio/cryostat-operator/internal/webhooks/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type validatorTestInput struct {
	client ctrlclient.Client
	objs   []ctrlclient.Object
	*webhooktests.WebhookTestResources
}

var _ = Describe("CryostatValidator", func() {
	var t *validatorTestInput
	var otherNS string
	count := 0

	namespaceWithSuffix := func(name string) string {
		return name + "-validator-" + strconv.Itoa(count)
	}

	BeforeEach(func() {
		ns := namespaceWithSuffix("test")
		otherNS = namespaceWithSuffix("other")
		t = &validatorTestInput{
			WebhookTestResources: &webhooktests.WebhookTestResources{
				TestResources: &test.TestResources{
					Name:      "cryostat",
					Namespace: ns,
					TargetNamespaces: []string{
						ns, otherNS,
					},
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

	Context("authorized user", func() {
		var cr *model.CryostatInstance

		BeforeEach(func() {
			cr = t.NewCryostat()
		})

		Context("creates a Cryostat", func() {
			It("should allow the request", func() {
				err := t.client.Create(ctx, cr.Object)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("updates a Cryostat", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, cr.Object)
			})

			It("should allow the request", func() {
				err := t.client.Update(ctx, cr.Object)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("deletes a Cryostat", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, cr.Object)
			})

			It("should allow the request", func() {
				err := t.client.Delete(ctx, cr.Object)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("unauthorized user", func() {
		var cr *model.CryostatInstance
		var sa *corev1.ServiceAccount
		var saClient ctrlclient.Client

		BeforeEach(func() {
			cr = t.NewCryostat()
			sa = t.NewWebhookTestServiceAccount()
			t.objs = append(t.objs,
				sa,
				t.NewWebhookTestRole(t.Namespace),
				t.NewWebhookTestRoleBinding(t.Namespace),
			)
		})

		JustBeforeEach(func() {
			config := rest.CopyConfig(cfg)
			config.Impersonate = rest.ImpersonationConfig{
				UserName: fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name),
			}
			client, err := ctrlclient.New(config, ctrlclient.Options{Scheme: k8sScheme})
			Expect(err).ToNot(HaveOccurred())
			saClient = client
		})

		Context("creates a Cryostat", func() {
			It("should deny the request", func() {
				err := saClient.Create(ctx, cr.Object)
				Expect(err).To((HaveOccurred()))
				expectErrNotPermitted(err, "create", otherNS)
			})
		})

		Context("updates a Cryostat", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, cr.Object)
			})

			It("should deny the request", func() {
				err := saClient.Update(ctx, cr.Object)
				Expect(err).To((HaveOccurred()))
				expectErrNotPermitted(err, "update", otherNS)
			})
		})

		Context("deletes a Cryostat", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, cr.Object)
			})

			It("should allow the request", func() {
				err := saClient.Delete(ctx, cr.Object)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

})

func expectErrNotPermitted(actual error, op string, namespace string) {
	expectedErr := webhooks.NewErrNotPermitted(op, namespace)
	Expect(kerrors.IsForbidden(actual)).To(BeTrue(), "expected Forbidden API error")
	Expect(actual.Error()).To(ContainSubstring(expectedErr.Error()))
}
