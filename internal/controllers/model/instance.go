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
