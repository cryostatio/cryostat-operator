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
	"context"
	"strconv"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	webhooktests "github.com/cryostatio/cryostat-operator/internal/webhooks/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type defaulterTestInput struct {
	client ctrlclient.Client
	objs   []ctrlclient.Object
	*webhooktests.WebhookTestResources
}

var _ = Describe("CryostatDefaulter", func() {
	var t *defaulterTestInput
	var otherNS string
	count := 0

	namespaceWithSuffix := func(name string) string {
		return name + "-defaulter-" + strconv.Itoa(count)
	}

	BeforeEach(func() {
		ns := namespaceWithSuffix("test")
		otherNS = namespaceWithSuffix("other")
		t = &defaulterTestInput{
			WebhookTestResources: &webhooktests.WebhookTestResources{
				TestResources: &test.TestResources{
					Name:      "cryostat",
					Namespace: ns,
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

	Context("without target namespace", func() {
		BeforeEach(func() {
			t.objs = append(t.objs, t.NewCryostat().Object)
		})

		It("should set default target namespace", func() {
			result := t.getCryostatInstance()
			Expect(result.TargetNamespaces).To(ConsistOf(t.Namespace))
		})
	})

	Context("with target namespace", func() {
		BeforeEach(func() {
			t.TargetNamespaces = []string{otherNS}
			t.objs = append(t.objs, t.NewCryostat().Object)
		})

		It("should do nothing", func() {
			result := t.getCryostatInstance()
			Expect(result.TargetNamespaces).To(ConsistOf(otherNS))
		})
	})

})

func (t *defaulterTestInput) getCryostatInstance() *model.CryostatInstance {
	cr := &operatorv1beta2.Cryostat{}
	err := t.client.Get(context.Background(), types.NamespacedName{Name: t.Name, Namespace: t.Namespace}, cr)
	Expect(err).ToNot(HaveOccurred())
	return t.ConvertNamespacedToModel(cr)
}
