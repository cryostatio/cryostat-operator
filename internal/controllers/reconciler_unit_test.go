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

package controllers

import (
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/cryostatio/cryostat-operator/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type cryostatUnitTestInput struct {
	scheme          *runtime.Scheme
	watchNamespaces []string
	*test.TestResources
}

var _ = Describe("Reconciler", func() {
	Describe("filtering requests", func() {
		Context("watches the configured namespace(s)", func() {
			var t *cryostatUnitTestInput
			var filter predicate.Predicate
			var cr *model.CryostatInstance

			BeforeEach(func() {
				resources := &test.TestResources{
					Name:      "cryostat",
					Namespace: "test",
				}
				t = &cryostatUnitTestInput{
					scheme:          test.NewTestScheme(),
					watchNamespaces: []string{resources.Namespace},
					TestResources:   resources,
				}
			})
			JustBeforeEach(func() {
				filter = namespaceEventFilter(t.scheme, t.watchNamespaces)
			})
			Context("creating a CR in the watched namespace", func() {
				BeforeEach(func() {
					cr = t.NewCryostat()
				})
				It("should reconcile the CR", func() {
					result := filter.Create(event.CreateEvent{
						Object: cr.Object,
					})
					Expect(result).To(BeTrue())
				})
			})
			Context("creating a CR in a non-watched namespace", func() {
				BeforeEach(func() {
					t.Namespace = "something-else"
					cr = t.NewCryostat()
				})
				It("should reconcile the CR", func() {
					result := filter.Create(event.CreateEvent{
						Object: cr.Object,
					})
					Expect(result).To(BeFalse())
				})
			})
		})
	})
})
