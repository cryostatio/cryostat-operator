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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterCryostatSpec defines the desired state of ClusterCryostat.
type ClusterCryostatSpec struct {
	// Namespace where Cryostat should be installed.
	// On multi-tenant clusters, we strongly suggest installing Cryostat into
	// its own namespace.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=1,xDescriptors={"urn:alm:descriptor:io.kubernetes:Namespace"}
	InstallNamespace string `json:"installNamespace"`
	// List of namespaces whose workloads Cryostat should be
	// permitted to access and profile. Defaults to `spec.installNamespace`.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=2
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	CryostatSpec `json:",inline"`
}

// ClusterCryostatStatus defines the observed state of ClusterCryostat.
type ClusterCryostatStatus struct { // FIXME only conditions are showing in console
	// List of namespaces that Cryostat has been configured
	// and authorized to access and profile.
	// +operator-sdk:csv:customresourcedefinitions:type=status,order=3
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=status
	CryostatStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=clustercryostats,scope=Cluster

// ClusterCryostat allows you to install Cryostat for multiple namespaces or cluster-wide.
// It contains configuration options for controlling the Deployment of the Cryostat
// application and its related components.
// A ClusterCryostat or Cryostat instance must be created to instruct the operator
// to deploy the Cryostat application.
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1},{Ingress,v1},{PersistentVolumeClaim,v1},{Secret,v1},{Service,v1},{Route,v1},{ConsoleLink,v1}}
// +kubebuilder:printcolumn:name="Application URL",type=string,JSONPath=`.status.applicationUrl`
// +kubebuilder:printcolumn:name="Grafana Secret",type=string,JSONPath=`.status.grafanaSecret`
type ClusterCryostat struct { // TODO add cluster-wide API support
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterCryostatSpec   `json:"spec,omitempty"`
	Status ClusterCryostatStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterCryostatList contains a list of ClusterCryostat
type ClusterCryostatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cryostat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterCryostat{}, &ClusterCryostatList{})
}
