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

package model

import (
	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CryostatInstance is an abstraction to work with both cluster-scoped
// and namespace-scoped Cryostat CRDs.
type CryostatInstance struct {
	// Name of the Cryostat CR.
	Name string
	// Namespace to install Cryostat into. For Cryostat, this comes from the
	// CR's namespace. For ClusterCryostat, this comes from spec.InstallNamespace.
	InstallNamespace string
	// Namespaces that Cryostat should look for targets. For Cryostat, this is
	// always the CR's namespace. For ClusterCryostat, this comes from spec.TargetNamespaces.
	TargetNamespaces []string
	// Namespaces that the operator has successfully set up RBAC for Cryostat to monitor targets
	// in that namespace. For Cryostat, this is always the CR's namespace.
	// For ClusterCryostat, this is a reference to status.TargetNamespaces.
	TargetNamespaceStatus *[]string
	// Reference to the common Spec properties to both CRD types.
	Spec *operatorv1beta1.CryostatSpec
	// Reference to the common Status properties to both CRD types.
	Status *operatorv1beta1.CryostatStatus
	// The actual CR instance as a generic Kubernetes object.
	Object client.Object
}

// FromCryostat creates a CryostatInstance from a Cryostat CR
func FromCryostat(cr *operatorv1beta1.Cryostat) *CryostatInstance {
	targetNS := []string{cr.Namespace}
	return &CryostatInstance{
		Name:                  cr.Name,
		InstallNamespace:      cr.Namespace,
		TargetNamespaces:      targetNS,
		TargetNamespaceStatus: &targetNS,

		Spec:   &cr.Spec,
		Status: &cr.Status,

		Object: cr,
	}
}

// FromClusterCryostat creates a CryostatInstance from a ClusterCryostat CR
func FromClusterCryostat(cr *operatorv1beta1.ClusterCryostat) *CryostatInstance {
	return &CryostatInstance{
		Name:                  cr.Name,
		InstallNamespace:      cr.Spec.InstallNamespace,
		TargetNamespaces:      cr.Spec.TargetNamespaces,
		TargetNamespaceStatus: &cr.Status.TargetNamespaces,

		Spec:   &cr.Spec.CryostatSpec,
		Status: &cr.Status.CryostatStatus,

		Object: cr,
	}
}
