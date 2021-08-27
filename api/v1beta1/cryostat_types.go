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
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CryostatSpec defines the desired state of Cryostat
type CryostatSpec struct {
	// Deploy a pared-down Cryostat instance with no Grafana dashboard or jfr-datasource
	// and no web-client UI.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Minimal Deployment",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	Minimal bool `json:"minimal"`
	// List of TLS certificates to trust when connecting to targets
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Trusted TLS Certificates"
	TrustedCertSecrets []CertificateSecret `json:"trustedCertSecrets,omitempty"`
	// List of Flight Recorder Event Templates to preconfigure in Cryostat
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Event Templates"
	EventTemplates []TemplateConfigMap `json:"eventTemplates,omitempty"`
	// Use cert-manager to secure in-cluster communication between Cryostat components.
	// Requires cert-manager to be installed.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable cert-manager integration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	EnableCertManager *bool `json:"enableCertManager"`
	// Options to customize the storage for Flight Recordings and Templates
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StorageOptions *StorageConfiguration `json:"storageOptions,omitempty"`
	// Options to control how the operator exposes the application over a network
	// +optional
	NetworkOptions *NetworkConfigurationList `json:"networkOptions,omitempty"`
}

// CryostatStatus defines the observed state of Cryostat
type CryostatStatus struct {
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
	IngressSpec *netv1.IngressSpec `json:"ingressSpec,omitempty"`
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
	// Specifications for ingress that exposes the cryostat service
	// (which serves the cryostat web-client)
	// +optional
	CoreConfig *NetworkConfiguration `json:"coreConfig,omitempty"`
	// Specifications for ingress that exposes the cryostat-command service
	// (which serves the websocket command channel)
	// +optional
	CommandConfig *NetworkConfiguration `json:"commandConfig,omitempty"`
	// Specifications for ingress that exposes the cryostat-grafana service
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
// +kubebuilder:resource:path=cryostats,scope=Namespaced

// Cryostat is the Schema for the cryostats API
//+operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1},{Ingress,v1},{PersistentVolumeClaim,v1},{Secret,v1},{Service,v1},{Route,v1},{ConsoleLink,v1}}
type Cryostat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CryostatSpec   `json:"spec,omitempty"`
	Status CryostatStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CryostatList contains a list of Cryostat
type CryostatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cryostat `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cryostat{}, &CryostatList{})
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

// A ConfigMap containing a .jfc template file
type TemplateConfigMap struct {
	// Name of config map in the local namespace
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:ConfigMap"}
	ConfigMapName string `json:"configMapName"`
	// Filename within config map containing the template file
	Filename string `json:"filename"`
}
