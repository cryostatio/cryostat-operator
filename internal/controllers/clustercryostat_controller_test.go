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

package controllers_test

import (
	"context"

	"github.com/cryostatio/cryostat-operator/internal/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ClusterCryostatController", func() {
	c := controllerTest{
		clusterScoped:   true,
		constructorFunc: newClusterCryostatController,
	}

	c.commonTests()

	Context("reconciling a multi-namespace request", func() {
		var t *cryostatTestInput
		targetNamespaces := []string{"multi-test-one", "multi-test-two"}

		BeforeEach(func() {
			t = c.commonBeforeEach()
			t.TargetNamespaces = targetNamespaces
			t.objs = append(t.objs, t.NewCryostat().Object)
			// Create Namespaces
			saveNS := t.Namespace
			for _, ns := range targetNamespaces {
				t.Namespace = ns
				t.objs = append(t.objs, t.NewNamespace())
			}
			t.Namespace = saveNS
		})

		JustBeforeEach(func() {
			c.commonJustBeforeEach(t)
			t.reconcileCryostatFully()
		})

		JustAfterEach(func() {
			c.commonJustAfterEach(t)
		})

		It("should create the expected main deployment", func() {
			t.expectMainDeployment()
		})

		It("should create RBAC in each namespace", func() {
			t.expectRBAC()
		})

		It("should update the target namespaces in Status", func() {
			t.expectTargetNamespaces()
		})

		Context("with removed target namespaces", func() {
			BeforeEach(func() {
				t = c.commonBeforeEach()
				// Begin with RBAC set up for two namespaces,
				// and remove the second namespace from the spec
				t.TargetNamespaces = targetNamespaces[:1]
				cr := t.NewCryostat()
				*cr.TargetNamespaceStatus = targetNamespaces
				t.objs = append(t.objs, cr.Object,
					t.NewRoleBinding(targetNamespaces[0]),
					t.NewRoleBinding(targetNamespaces[1]))
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
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
			It("should update the target namespaces in Status", func() {
				t.expectTargetNamespaces()
			})
		})

		Context("with no target namespaces", func() {
			BeforeEach(func() {
				t = c.commonBeforeEach()
				t.TargetNamespaces = nil
				t.objs = append(t.objs, t.NewCryostat().Object)
			})
			It("should update the target namespaces in Status", func() {
				t.expectTargetNamespaces()
			})
		})
	})
})

func (t *cryostatTestInput) expectTargetNamespaces() {
	cr := t.getCryostatInstance()
	Expect(*cr.TargetNamespaceStatus).To(ConsistOf(t.TargetNamespaces))
}

func newClusterCryostatController(config *controllers.ReconcilerConfig) controllers.CommonReconciler {
	return controllers.NewClusterCryostatReconciler(config)
}
