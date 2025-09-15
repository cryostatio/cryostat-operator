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

package fips_test

import (
	"strconv"

	"github.com/cryostatio/cryostat-operator/internal/fips"
	fipstests "github.com/cryostatio/cryostat-operator/internal/fips/test"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type fipsTestInput struct {
	client ctrlclient.Client
	objs   []ctrlclient.Object
	*fipstests.FIPSTestResources
}

var _ = Describe("FIPS", func() {
	var t *fipsTestInput
	count := 0

	namespaceWithSuffix := func(name string) string {
		return name + "-fips-" + strconv.Itoa(count)
	}

	BeforeEach(func() {
		ns := namespaceWithSuffix("test")
		t = &fipsTestInput{
			FIPSTestResources: &fipstests.FIPSTestResources{
				TestResources: &test.TestResources{
					Name:             "cryostat",
					Namespace:        ns,
					TargetNamespaces: []string{ns},
					TLS:              true,
				},
			},
		}
		t.objs = []ctrlclient.Object{
			t.NewNamespace(),
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

	Context("detecting FIPS mode", func() {
		Context("on FIPS cluster", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewClusterConfigFIPS())
			})
			It("should return true", func() {
				result, err := fips.IsFIPS(k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("on non-FIPS cluster", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewClusterConfigNoFIPS())
			})
			It("should return false", func() {
				result, err := fips.IsFIPS(k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
		Context("with missing config map", func() {
			It("should return an error", func() {
				_, err := fips.IsFIPS(k8sClient)
				Expect(err).To(HaveOccurred())
			})
		})
		Context("with missing install-config", func() {
			BeforeEach(func() {
				t.objs = append(t.objs, t.NewClusterConfigBad())
			})
			It("should return an error", func() {
				_, err := fips.IsFIPS(k8sClient)
				Expect(err).To(HaveOccurred())
			})
		})
	})

})
