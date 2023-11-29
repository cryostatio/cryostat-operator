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
	"context"

	"github.com/cryostatio/cryostat-operator/internal/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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

		It("should create CA Cert secret in each namespace", func() {
			t.expectCertificates()
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
					t.NewRoleBinding(targetNamespaces[1]),
					t.NewCACertSecret(targetNamespaces[0]),
					t.NewCACertSecret(targetNamespaces[1]))
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
			It("leave CA Cert secret for the first namespace", func() {
				t.expectCertificates()
			})
			It("should remove CA Cert secret from the second namespace", func() {
				caCert := t.NewCACert()
				secret := &corev1.Secret{}
				err := t.Client.Get(context.Background(), types.NamespacedName{Name: caCert.Name, Namespace: targetNamespaces[1]}, secret)
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

func newClusterCryostatController(config *controllers.ReconcilerConfig) (controllers.CommonReconciler, error) {
	return controllers.NewClusterCryostatReconciler(config)
}
