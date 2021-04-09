// Copyright (c) 2021 Red Hat, Inc.
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
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainerJFRSpec defines the desired state of ContainerJFR
type ContainerJFRSpec struct {
	// Deploy a pared-down ContainerJFR instance with no Grafana dashboard or jfr-datasource
	// and no web-client UI.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Minimal Deployment",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	Minimal bool `json:"minimal"`
	// List of TLS certificates to trust when connecting to targets
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Trusted TLS Certificates"
	TrustedCertSecrets []CertificateSecret `json:"trustedCertSecrets,omitempty"`
	// Options to customize the storage for Flight Recordings and Templates
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StorageOptions *StorageConfiguration `json:"storageOptions,omitempty"`
	// Ingress specifications when running outside of OpenShift environment
	// +optional
	NetworkOptions *NetworkConfigurationList `json:"networkOptions,omitempty"`
}

// ContainerJFRStatus defines the observed state of ContainerJFR
type ContainerJFRStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status,xDescriptors={"urn:alm:descriptor:org.w3:link"}
	ApplicationURL string `json:"applicationUrl"`
}

// StorageConfiguration provides customization to the storage created by
// the operator to hold Flight Recordings and Recording Templates.
type StorageConfiguration struct {
	// Configuration for the Persistent Volume Claim to be created
	// by the operator.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	PVC *PersistentVolumeClaimConfig `json:"pvc,omitempty"`
}

// NetworkConfiguration provides customization for the corresponding ingress,
// which allows a service to be exposed when running in a Kubernetes environment
type NetworkConfiguration struct {
	// Configuration for an ingress object.
	// Currently subpaths are not supported, so unique hosts must be specified
	// (if a single external IP is being used) to differentiate between ingresses/services
	// +optional
	IngressSpec *netv1.IngressSpec `json:"spec,omitempty"`
	// Annotations to add to the ingress during its creation.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels to add to the ingress during its creation.
	// The label with key "app" is reserved for use by the operator.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// NetworkConfigurationList holds all three NetworkConfiguration objects that specify
// the ingress configurations for the three services created by the operator
type NetworkConfigurationList struct {
	// Specifications for ingress that exposes the containerjfr service
	// (which serves the containerjfr web-client)
	// +optional
	ExporterConfig *NetworkConfiguration `json:"exporterConfig,omitempty"`
	// Specifications for ingress that exposes the containerjfr-command service
	// (which serves the websocket command channel)
	// +optional
	CommandConfig *NetworkConfiguration `json:"commandConfig,omitempty"`
	// Specifications for ingress that exposes the containerjfr-grafana service
	// (which serves the grafana dashboard)
	// +optional
	GrafanaConfig *NetworkConfiguration `json:"grafanaConfig,omitempty"`
}

// PersistentVolumeClaimConfig holds all customization options to
// configure a Persistent Volume Claim to be created and managed
// by the operator.
type PersistentVolumeClaimConfig struct {
	// Annotations to add to the Persistent Volume Claim during its creation.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels to add to the Persistent Volume Claim during its creation.
	// The label with key "app" is reserved for use by the operator.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Spec for a Persistent Volume Claim, whose options will override the
	// defaults used by the operator. Unless overriden, the PVC will be
	// created with the default Storage Class and 500MiB of storage.
	// Once the operator has created the PVC, changes to this field have
	// no effect.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Spec *corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=containerjfrs,scope=Namespaced

// ContainerJFR is the Schema for the containerjfrs API
//+operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1},{Ingress,v1},{PersistentVolumeClaim,v1},{Secret,v1},{Service,v1},{Route,v1},{ConsoleLink,v1}}
type ContainerJFR struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContainerJFRSpec   `json:"spec,omitempty"`
	Status ContainerJFRStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContainerJFRList contains a list of ContainerJFR
type ContainerJFRList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContainerJFR `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ContainerJFR{}, &ContainerJFRList{})
}

// DefaultCertificateKey will be used when looking up the certificate within a secret,
// if a key is not manually specified
const DefaultCertificateKey = corev1.TLSCertKey

type CertificateSecret struct {
	// Name of secret in the local namespace
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	SecretName string `json:"secretName"`
	// Key within secret containing the certificate
	// +optional
	CertificateKey *string `json:"certificateKey,omitempty"`
}
