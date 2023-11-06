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
	"github.com/cryostatio/cryostat-operator/internal/controllers"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ = Describe("CryostatController", func() {
	c := &controllerTest{
		clusterScoped:   false,
		constructorFunc: newCryostatController,
	}

	c.commonTests()

	Describe("filtering requests", func() {
		Context("watches the configured namespace(s)", func() {
			var t *cryostatTestInput
			var filter predicate.Predicate
			var cr *model.CryostatInstance

			BeforeEach(func() {
				t = c.commonBeforeEach()
				t.watchNamespaces = []string{t.Namespace}
			})
			JustBeforeEach(func() {
				c.commonJustBeforeEach(t)
				filter = controllers.NamespaceEventFilter(t.Client.Scheme(), t.watchNamespaces)
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

func newCryostatController(config *controllers.ReconcilerConfig) (controllers.CommonReconciler, error) {
	return controllers.NewCryostatReconciler(config)
}
