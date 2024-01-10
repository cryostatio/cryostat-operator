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
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	CryostatSpec `json:",inline"`
}

// ClusterCryostatStatus defines the observed state of ClusterCryostat.
type ClusterCryostatStatus struct {
	// List of namespaces that Cryostat has been configured
	// and authorized to access and profile.
	// +optional
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
type ClusterCryostat struct {
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
	Items           []ClusterCryostat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterCryostat{}, &ClusterCryostatList{})
}
