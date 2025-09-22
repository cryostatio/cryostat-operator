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

package v1beta2

import (
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// CryostatSpec defines the desired state of Cryostat.
type CryostatSpec struct {
	// List of namespaces whose workloads Cryostat should be
	// permitted to access and profile. Defaults to this Cryostat's namespace.
	// Warning: All Cryostat users will be able to create and manage
	// recordings for workloads in the listed namespaces.
	// More details: https://github.com/cryostatio/cryostat-operator/blob/v4.0.0/docs/config.md#data-isolation
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=2
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`
	// List of TLS certificates to trust when connecting to targets.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Trusted TLS Certificates"
	TrustedCertSecrets []CertificateSecret `json:"trustedCertSecrets,omitempty"`
	// List of Flight Recorder Event Templates to preconfigure in Cryostat.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Event Templates"
	EventTemplates []TemplateConfigMap `json:"eventTemplates,omitempty"`
	// List of Stored Credentials to preconfigure in Cryostat.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Stored Credentials"
	DeclarativeCredentials []DeclarativeCredential `json:"declarativeCredentials,omitempty"`
	// Use cert-manager to secure in-cluster communication between Cryostat components.
	// Requires cert-manager to be installed.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,order=3,displayName="Enable cert-manager Integration",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	EnableCertManager *bool `json:"enableCertManager"`
	// Options to customize the storage provisioned for the database and object storage.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StorageOptions *StorageConfigurations `json:"storageOptions,omitempty"`
	// Options to customize the services created for the Cryostat application.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ServiceOptions *ServiceConfigList `json:"serviceOptions,omitempty"`
	// Options to customize the NetworkPolicy objects created for Cryostat's various Services.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	NetworkPolicies *NetworkPoliciesList `json:"networkPolicies,omitempty"`
	// Options to control how the operator exposes the application outside of the cluster,
	// such as using an Ingress or Route.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	NetworkOptions *NetworkConfigurationList `json:"networkOptions,omitempty"`
	// Options to configure Cryostat Automated Report Analysis.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ReportOptions *ReportConfiguration `json:"reportOptions,omitempty"`
	// Options to customize the target connections cache for the Cryostat application.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Target Connection Cache Options"
	TargetConnectionCacheOptions *TargetConnectionCacheOptions `json:"targetConnectionCacheOptions,omitempty"`
	// Resource requirements for the Cryostat deployment. Default resource requests will be added to each
	// container unless specified. Default resource limits will be added to each container if neither
	// resource requests or limits are specified.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Resources *ResourceConfigList `json:"resources,omitempty"`
	// Additional configuration options for the authorization proxy.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Authorization Options",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	AuthorizationOptions *AuthorizationOptions `json:"authorizationOptions,omitempty"`
	// Options to configure the Security Contexts for the Cryostat application.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	SecurityOptions *SecurityOptions `json:"securityOptions,omitempty"`
	// Options to configure scheduling for the Cryostat deployment
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SchedulingOptions *SchedulingConfiguration `json:"schedulingOptions,omitempty"`
	// Options to configure the Cryostat application's target discovery mechanisms.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	TargetDiscoveryOptions *TargetDiscoveryOptions `json:"targetDiscoveryOptions,omitempty"`
	// Options to configure the Cryostat application's database.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Database Options"
	DatabaseOptions *DatabaseOptions `json:"databaseOptions,omitempty"`
	// Options to configure the Cryostat deployments and pods metadata
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Operand metadata"
	OperandMetadata *OperandMetadata `json:"operandMetadata,omitempty"`
	// Options to control how the operator configures Cryostat Agents
	// to communicate with this Cryostat instance.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Agent Options"
	AgentOptions *AgentOptions `json:"agentOptions,omitempty"`
}

type OperandMetadata struct {
	// Options to configure the Cryostat deployments metadata
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Deployments metadata"
	DeploymentMetadata *ResourceMetadata `json:"deploymentMetadata,omitempty"`
	// Options to configure the Cryostat pods metadata
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Pods metadata"
	PodMetadata *ResourceMetadata `json:"podMetadata,omitempty"`
}

// ResourceMetadata contains common metadata options used in several properties.
type ResourceMetadata struct {
	// Annotations to add to the object during its creation.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels to add to the object during its creation.
	// The following label keys are reserved for use by the operator:
	// "app", "component", "app.kubernetes.io/name", "app.kubernetes.io/instance",
	// "app.kubernetes.io/component", and "app.kubernetes.io/part-of".
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Labels map[string]string `json:"labels,omitempty"`
}

type ResourceConfigList struct {
	// Resource requirements for the auth proxy.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	AuthProxyResources corev1.ResourceRequirements `json:"authProxyResources,omitempty"`
	// Resource requirements for the Cryostat application. If specifying a memory limit, at least 384MiB is recommended.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	CoreResources corev1.ResourceRequirements `json:"coreResources,omitempty"`
	// Resource requirements for the JFR Data Source container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	DataSourceResources corev1.ResourceRequirements `json:"dataSourceResources,omitempty"`
	// Resource requirements for the Grafana container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	GrafanaResources corev1.ResourceRequirements `json:"grafanaResources,omitempty"`
	// Resource requirements for the database container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	DatabaseResources corev1.ResourceRequirements `json:"databaseResources,omitempty"`
	// Resource requirements for the object storage container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	ObjectStorageResources corev1.ResourceRequirements `json:"objectStorageResources,omitempty"`
	// Resource requirements for the agent proxy container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	AgentProxyResources corev1.ResourceRequirements `json:"agentProxyResources,omitempty"`
}

// CryostatStatus defines the observed state of Cryostat.
type CryostatStatus struct {
	// List of namespaces that Cryostat has been configured
	// and authorized to access and profile.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,order=3
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`
	// Conditions of the components managed by the Cryostat Operator.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Cryostat Conditions",xDescriptors={"urn:alm:descriptor:io.kubernetes.conditions"}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Address of the deployed Cryostat web application.
	// +operator-sdk:csv:customresourcedefinitions:type=status,order=1,xDescriptors={"urn:alm:descriptor:org.w3:link"}
	ApplicationURL string `json:"applicationUrl"`
	// Name of the Secret containing the Cryostat storage connection key.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,order=2,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	StorageSecret string `json:"storageSecret,omitempty"`
	// Name of the Secret containing the Cryostat database connection and encryption keys.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,order=2,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	DatabaseSecret string `json:"databaseSecret,omitempty"`
}

// CryostatConditionType refers to a Condition type that may be used in status.conditions
type CryostatConditionType string

const (
	// Whether the main Cryostat deployment is available.
	ConditionTypeMainDeploymentAvailable CryostatConditionType = "MainDeploymentAvailable"
	// Whether the main Cryostat deployment is progressing.
	ConditionTypeMainDeploymentProgressing CryostatConditionType = "MainDeploymentProgressing"
	// If pods within the main Cryostat deployment failed to be created or destroyed.
	ConditionTypeMainDeploymentReplicaFailure CryostatConditionType = "MainDeploymentReplicaFailure"
	// If enabled, whether the database deployment is available.
	ConditionTypeDatabaseDeploymentAvailable CryostatConditionType = "DatabaseDeploymentAvailable"
	// If enabled, whether the database deployment is progressing.
	ConditionTypeDatabaseDeploymentProgressing CryostatConditionType = "DatabaseDeploymentProgressing"
	// If enabled, whether pods in the database deployment failed to be created or destroyed.
	ConditionTypeDatabaseDeploymentReplicaFailure CryostatConditionType = "DatabaseDeploymentReplicaFailure"
	// If enabled, whether the storage deployment is available.
	ConditionTypeStorageDeploymentAvailable CryostatConditionType = "StorageDeploymentAvailable"
	// If enabled, whether the storage deployment is progressing.
	ConditionTypeStorageDeploymentProgressing CryostatConditionType = "StorageDeploymentProgressing"
	// If enabled, whether pods in the storage deployment failed to be created or destroyed.
	ConditionTypeStorageDeploymentReplicaFailure CryostatConditionType = "StorageDeploymentReplicaFailure"
	// If enabled, whether the reports deployment is available.
	ConditionTypeReportsDeploymentAvailable CryostatConditionType = "ReportsDeploymentAvailable"
	// If enabled, whether the reports deployment is progressing.
	ConditionTypeReportsDeploymentProgressing CryostatConditionType = "ReportsDeploymentProgressing"
	// If enabled, whether pods in the reports deployment failed to be created or destroyed.
	ConditionTypeReportsDeploymentReplicaFailure CryostatConditionType = "ReportsDeploymentReplicaFailure"
	// If enabled, whether TLS setup is complete for the Cryostat components.
	ConditionTypeTLSSetupComplete CryostatConditionType = "TLSSetupComplete"
)

// StorageConfigurations provides customization to the storage provisioned for
// the database and the object storage.
type StorageConfigurations struct {
	// Configuration for the Persistent Volume Claim to be created by the operator for the database.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Database *StorageConfiguration `json:"database,omitempty"`
	// Configuration for the Persistent Volume Claim to be created by the operator for the object storage.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ObjectStorage              *StorageConfiguration `json:"objectStorage,omitempty"`
	LegacyStorageConfiguration `json:",inline"`
}

// StorageConfiguration provides customization to the storage created by the
// operator to contain persisted data. If no configurations are specified, a
// PVC will be created by default.
type StorageConfiguration struct {
	// Configuration for the Persistent Volume Claim to be created
	// by the operator.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	PVC *PersistentVolumeClaimConfig `json:"pvc,omitempty"`
	// Configuration for an EmptyDir to be created
	// by the operator instead of a PVC.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	EmptyDir *EmptyDirConfig `json:"emptyDir,omitempty"`
}

// LegacyStorageConfiguration provides customization to the storage created by the
// operator to contain persisted data. If no configurations are specified, a
// PVC will be created by default.
// Deprecated: use StorageConfiguration instead.
type LegacyStorageConfiguration struct {
	// Configuration for the Persistent Volume Claim to be created
	// by the operator.
	// Deprecated: use storageOptions.database and storageOptions.objectStorage
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	PVC *PersistentVolumeClaimConfig `json:"pvc,omitempty"`
	// Configuration for an EmptyDir to be created
	// by the operator instead of a PVC.
	// Deprecated: use storageOptions.database and storageOptions.objectStorage
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	EmptyDir *EmptyDirConfig `json:"emptyDir,omitempty"`
}

// ReportConfiguration is used to determine how many replicas of cryostat-reports
// the operator should create and what the resource limits of those containers
// should be. If no replicas are created then Cryostat is configured to use basic
// subprocess report generation. If at least one replica is created then Cryostat
// is configured to use remote report generation, pointed at a load balancer service
// in front of the cryostat-reports replicas.
type ReportConfiguration struct {
	// The number of report sidecar replica containers to deploy.
	// Each replica can service one report generation request at a time.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:podCount"}
	Replicas int32 `json:"replicas,omitempty"`
	// The resources allocated to each sidecar replica.
	// A replica with more resources can handle larger input recordings and will process them faster.
	// Default resource requests will be added to the container unless specified.
	// Default resource limits will be added to each container if neither resource requests or limits are specified.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// When zero report sidecar replicas are requested, SubProcessMaxHeapSize configures
	// the maximum heap size of the basic subprocess report generator in MiB.
	// The default heap size is `200` (MiB).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:number"}
	SubProcessMaxHeapSize int32 `json:"subProcessMaxHeapSize,omitempty"`
	// Options to configure the Security Contexts for the Cryostat report generator.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:advanced"}
	SecurityOptions *ReportsSecurityOptions `json:"securityOptions,omitempty"`
	// Options to configure scheduling for the reports deployment
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	SchedulingOptions *SchedulingConfiguration `json:"schedulingOptions,omitempty"`
}

// SchedulingConfiguration contains multiple choices to control scheduling of Cryostat pods
type SchedulingConfiguration struct {
	// Label selector used to schedule a Cryostat pod to a node. See: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:selector:core:v1:Node"}
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Affinity rules for scheduling Cryostat pods.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Affinity *Affinity `json:"affinity,omitempty"`
	// Tolerations to allow scheduling of Cryostat pods to tainted nodes. See: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// Affinity groups different kinds of affinity configurations for Cryostat pods
type Affinity struct {
	// Node affinity scheduling rules for a Cryostat pod. See: https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#NodeAffinity
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:nodeAffinity"}
	NodeAffinity *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`
	// Pod affinity scheduling rules for a Cryostat pod. See: https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#PodAffinity
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:podAffinity"}
	PodAffinity *corev1.PodAffinity `json:"podAffinity,omitempty"`
	// Pod anti-affinity scheduling rules for a Cryostat pod. See: https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#PodAntiAffinity
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:podAntiAffinity"}
	PodAntiAffinity *corev1.PodAntiAffinity `json:"podAntiAffinity,omitempty"`
}

// ServiceConfig provides customization for a service created
// by the operator.
type ServiceConfig struct {
	// Type of service to create. Defaults to "ClusterIP".
	// +optional
	ServiceType      *corev1.ServiceType `json:"serviceType,omitempty"`
	ResourceMetadata `json:",inline"`
}

// CoreServiceConfig provides customization for the service handling
// traffic for the Cryostat application.
type CoreServiceConfig struct {
	// HTTP port number for the Cryostat application service.
	// Defaults to 8181.
	// +optional
	HTTPPort      *int32 `json:"httpPort,omitempty"`
	ServiceConfig `json:",inline"`
}

// ReportsServiceConfig provides customization for the service handling
// traffic for the cryostat-reports sidecars.
type ReportsServiceConfig struct {
	// HTTP port number for the cryostat-reports service.
	// Defaults to 10000.
	// +optional
	HTTPPort      *int32 `json:"httpPort,omitempty"`
	ServiceConfig `json:",inline"`
}

// DatabaseServiceConfig provides customization for the service handling
// traffic for the cryostat application's database.
type DatabaseServiceConfig struct {
	// DatabasePort number for the cryostat application's database.
	// Defaults to 5432.
	// +optional
	DatabasePort  *int32 `json:"databasePort,omitempty"`
	ServiceConfig `json:",inline"`
}

// DatabaseServiceConfig provides customization for the service handling
// traffic for the storage to be created by the operator.
type StorageServiceConfig struct {
	// HTTP port number for the storage to be created by the operator.
	// Defaults to 8333.
	// +optional
	HTTPPort      *int32 `json:"httpPort,omitempty"`
	ServiceConfig `json:",inline"`
}

// AgentGatewayServiceConfig provides customization for the service handling
// traffic from Cryostat agents to the Cryostat application.
type AgentGatewayServiceConfig struct {
	// HTTP port number for the Cryostat agent API service.
	// Defaults to 8282.
	// +optional
	HTTPPort      *int32 `json:"httpPort,omitempty"`
	ServiceConfig `json:",inline"`
}

// AgentCallbackServiceConfig provides customization for the headless services
// in each target namespace handling traffic from Cryostat to agents in those
// namespaces.
type AgentCallbackServiceConfig struct {
	ResourceMetadata `json:",inline"`
}

// ServiceConfigList holds the service configuration for each
// service created by the operator.
type ServiceConfigList struct {
	// Specification for the service responsible for the Cryostat application.
	// +optional
	CoreConfig *CoreServiceConfig `json:"coreConfig,omitempty"`
	// Specification for the service responsible for the cryostat-reports sidecars.
	// +optional
	ReportsConfig *ReportsServiceConfig `json:"reportsConfig,omitempty"`
	// Specification for the service responsible for the cryostat application's database.
	// +optional
	DatabaseConfig *DatabaseServiceConfig `json:"databaseConfig,omitempty"`
	// Specification for the service responsible for the storage to be created by the operator.
	// +optional
	StorageConfig *StorageServiceConfig `json:"storageConfig,omitempty"`
	// Specification for the service responsible for agents to communicate with Cryostat.
	// +optional
	AgentGatewayConfig *AgentGatewayServiceConfig `json:"agentGatewayConfig,omitempty"`
	// Specification for the headless services in each target namespace that allow Cryostat
	// to communicate with agents in those namespaces.
	// +optional
	AgentCallbackConfig *AgentCallbackServiceConfig `json:"agentCallbackConfig,omitempty"`
}

// NetworkPoliciesList holds the configurations for NetworkPolicy
// objects for each service created by the operator.
type NetworkPoliciesList struct {
	// NetworkPolicy configuration for the Cryostat application service.
	// +optional
	CoreConfig *NetworkPolicyConfig `json:"coreConfig,omitempty"`
	// NetworkPolicy configuration for the cryostat-reports service.
	// +optional
	ReportsConfig *NetworkPolicyConfig `json:"reportsConfig,omitempty"`
	// NetworkPolicy configuration for the database service.
	// +optional
	DatabaseConfig *NetworkPolicyConfig `json:"databaseConfig,omitempty"`
	// NetworkPolicy configuration for the storage service.
	// +optional
	StorageConfig *NetworkPolicyConfig `json:"storageConfig,omitempty"`
}

type NetworkPolicyConfig struct {
	// Disable the NetworkPolicies (Ingress and Egress) for a given service.
	// Deprecated: use IngressDisabled and EgressEnabled instead.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable NetworkPolicy creation",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	Disabled *bool `json:"disabled,omitempty"`
	// Disable the NetworkPolicy for ingress to a given pod. Enabled by default.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable ingress NetworkPolicy creation",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	IngressDisabled *bool `json:"ingressDisabled,omitempty"`
	// Enable the NetworkPolicy for egress from a given pod. Disabled by default.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable egress NetworkPolicy creation",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	EgressEnabled *bool `json:"egressEnabled,omitempty"`
}

// NetworkConfiguration provides customization for how to expose a Cryostat
// service, so that it can be reached from outside the cluster.
// On OpenShift, a Route is created by default. On Kubernetes, an Ingress will
// be created if the IngressSpec is defined within this NetworkConfiguration.
type NetworkConfiguration struct {
	// Externally routable host to be used to reach this
	// Cryostat service. Used to define a Route's host on
	// OpenShift when it is first created.
	// On Kubernetes, define this using "spec.ingressSpec".
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ExternalHost *string `json:"externalHost,omitempty"`
	// Configuration for an Ingress object.
	// Currently subpaths are not supported, so unique hosts must be specified
	// (if a single external IP is being used) to differentiate between ingresses/services.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	IngressSpec      *netv1.IngressSpec `json:"ingressSpec,omitempty"`
	ResourceMetadata `json:",inline"`
}

// NetworkConfigurationList holds NetworkConfiguration objects that specify
// how to expose the services created by the operator for the main Cryostat
// deployment.
type NetworkConfigurationList struct {
	// Specifications for how to expose the Cryostat service,
	// which serves the Cryostat application.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	CoreConfig *NetworkConfiguration `json:"coreConfig,omitempty"`
}

// PersistentVolumeClaimConfig holds all customization options to
// configure a Persistent Volume Claim to be created and managed
// by the operator.
type PersistentVolumeClaimConfig struct {
	// Spec for a Persistent Volume Claim, whose options will override the
	// defaults used by the operator. Unless overriden, the PVC will be
	// created with the default Storage Class and 500MiB of storage.
	// Once the operator has created the PVC, changes to this field have
	// no effect.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Spec             *corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
	ResourceMetadata `json:",inline"`
}

// EmptyDirConfig holds all customization options to
// configure an EmptyDir to be created and managed
// by the operator.
type EmptyDirConfig struct {
	// When enabled, Cryostat will use EmptyDir volumes instead of a Persistent Volume Claim. Any PVC configurations will be ignored.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	Enabled bool `json:"enabled,omitempty"`
	// Unless specified, the emptyDir volume will be mounted on
	// the same storage medium backing the node. Setting this field to
	// "Memory" will mount the emptyDir on a tmpfs (RAM-backed filesystem).
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Medium corev1.StorageMedium `json:"medium,omitempty"`
	// The maximum memory limit for the emptyDir. Default is unbounded.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +kubebuilder:validation:Pattern=^(\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))))?$
	SizeLimit string `json:"sizeLimit,omitempty"`
}

// TargetConnectionCacheOptions provides customization for the target connections
// cache for the Cryostat application.
type TargetConnectionCacheOptions struct {
	// The maximum number of target connections to cache. Use `-1` for an unlimited cache size (TTL expiration only). Defaults to `-1`.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:number"}
	// +kubebuilder:validation:Minimum=-1
	TargetCacheSize int32 `json:"targetCacheSize,omitempty"`
	// The time to live (in seconds) for cached target connections. Defaults to `10`.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:number"}
	// +kubebuilder:validation:Minimum=1
	TargetCacheTTL int32 `json:"targetCacheTTL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=cryostats,scope=Namespaced

// Cryostat allows you to install Cryostat for a single namespace, or multiple namespaces.
// It contains configuration options for controlling the Deployment of the Cryostat
// application and its related components.
// A Cryostat instance must be created to instruct the operator
// to deploy the Cryostat application.
// +operator-sdk:csv:customresourcedefinitions:resources={{Deployment,v1},{Ingress,v1},{PersistentVolumeClaim,v1},{Secret,v1},{Service,v1},{Route,v1},{ConsoleLink,v1}}
// +kubebuilder:printcolumn:name="Application URL",type=string,JSONPath=`.status.applicationUrl`
// +kubebuilder:printcolumn:name="Target Namespaces",type=string,JSONPath=`.status.targetNamespaces`
// +kubebuilder:printcolumn:name="Storage Secret",type=string,JSONPath=`.status.storageSecret`
// +kubebuilder:printcolumn:name="Database Secret",type=string,JSONPath=`.status.databaseSecret`
type Cryostat struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CryostatSpec   `json:"spec,omitempty"`
	Status CryostatStatus `json:"status,omitempty"`
}

// ConvertFrom implements conversion.Convertible.
func (*Cryostat) ConvertFrom(src conversion.Hub) error {
	panic("unimplemented")
}

// ConvertTo implements conversion.Convertible.
func (*Cryostat) ConvertTo(dst conversion.Hub) error {
	panic("unimplemented")
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
// if a key is not manually specified.
const DefaultCertificateKey = corev1.TLSCertKey

type CertificateSecret struct {
	// Name of secret in the local namespace.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	SecretName string `json:"secretName"`
	// Key within secret containing the certificate.
	// +optional
	CertificateKey *string `json:"certificateKey,omitempty"`
}

type DeclarativeCredential struct {
	// Name of secret in the local namespace. The contents of that secret are expected to be a list of json
	// representations of stored credentials in the format
	// { "username": "$USERNAME", "password": "$PASSWORD", "matchExpression": "$MATCHEXPRESSION" }
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	SecretName string `json:"secretName"`
}

// A ConfigMap containing a .jfc template file.
type TemplateConfigMap struct {
	// Name of config map in the local namespace.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:ConfigMap"}
	ConfigMapName string `json:"configMapName"`
	// Filename within config map containing the template file.
	Filename string `json:"filename"`
}

// Authorization options provide additional configurations for the auth proxy.
type AuthorizationOptions struct {
	// Configuration for OpenShift RBAC to define which OpenShift user accounts may access the Cryostat application.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OpenShift SSO"
	OpenShiftSSO *OpenShiftSSOConfig `json:"openShiftSSO,omitempty"`
	// Reference to a secret and file name containing the Basic authentication htpasswd file. If deploying on OpenShift this
	// defines additional user accounts that can access the Cryostat application, on top of the OpenShift user accounts which
	// pass the OpenShift SSO Roles checks. If not on OpenShift then this defines the only user accounts that have access.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	BasicAuth *SecretFile `json:"basicAuth,omitempty"`
}

type OpenShiftSSOConfig struct {
	// Disable OpenShift SSO integration and allow all users to access the application without authentication. This
	// will also bypass the BasicAuth, if specified.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable OpenShift SSO",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	Disable *bool `json:"disable,omitempty"`
	// The SubjectAccessReview or TokenAccessReview that all clients (users visiting the application via web browser as well
	// as CLI utilities and other programs presenting Bearer auth tokens) must pass in order to access the application.
	// If not specified, the default role required is "create pods/exec" in the Cryostat application's installation namespace.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	AccessReview *authzv1.ResourceAttributes `json:"accessReview,omitempty"`
}

type SecretFile struct {
	// Name of the secret to reference.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	SecretName *string `json:"secretName,omitempty"`
	// Name of the file within the secret.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Filename *string `json:"filename,omitempty"`
}

// Authorization properties provide custom permission mapping between Cryostat resources to Kubernetes resources.
// If the mapping is updated, Cryostat must be manually restarted.
type AuthorizationProperties struct {
	// Name of the ClusterRole to use when Cryostat requests a role-scoped OAuth token.
	// This ClusterRole should contain permissions for all Kubernetes objects listed in custom permission mapping.
	// More details: https://docs.openshift.com/container-platform/4.11/authentication/tokens-scoping.html#scoping-tokens-role-scope_configuring-internal-oauth
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="ClusterRole Name",xDescriptors={"urn:alm:descriptor:io.kubernetes:ClusterRole"}
	ClusterRoleName string `json:"clusterRoleName"`
	// Name of config map in the local namespace.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="ConfigMap Name",xDescriptors={"urn:alm:descriptor:io.kubernetes:ConfigMap"}
	ConfigMapName string `json:"configMapName"`
	// Filename within config map containing the resource mapping.
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Filename string `json:"filename"`
}

// SecurityOptions contains Security Context customizations for the
// main Cryostat application at both the pod and container level.
type SecurityOptions struct {
	// Security Context to apply to the Cryostat pod.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// Security Context to apply to the auth proxy container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	AuthProxySecurityContext *corev1.SecurityContext `json:"authProxySecurityContext,omitempty"`
	// Security Context to apply to the Cryostat application container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	CoreSecurityContext *corev1.SecurityContext `json:"coreSecurityContext,omitempty"`
	// Security Context to apply to the JFR Data Source container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	DataSourceSecurityContext *corev1.SecurityContext `json:"dataSourceSecurityContext,omitempty"`
	// Security Context to apply to the Grafana container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	GrafanaSecurityContext *corev1.SecurityContext `json:"grafanaSecurityContext,omitempty"`
	// Security Context to apply to the storage container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StorageSecurityContext *corev1.SecurityContext `json:"storageSecurityContext,omitempty"`
	// Security Context to apply to the database container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	DatabaseSecurityContext *corev1.SecurityContext `json:"databaseSecurityContext,omitempty"`
	// Security Context to apply to the agent proxy container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	AgentProxySecurityContext *corev1.SecurityContext `json:"agentProxySecurityContext,omitempty"`
}

// ReportsSecurityOptions contains Security Context customizations for the
// Cryostat report generator at both the pod and container level.
type ReportsSecurityOptions struct {
	// Security Context to apply to the Cryostat report generator pod.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// Security Context to apply to the Cryostat report generator container.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	ReportsSecurityContext *corev1.SecurityContext `json:"reportsSecurityContext,omitempty"`
}

// TargetDiscoveryOptions provides configuration options to the Cryostat application's target discovery mechanisms.
type TargetDiscoveryOptions struct {
	// When true, the Cryostat application will disable the built-in discovery mechanisms. Defaults to false
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable Built-in Discovery",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	DisableBuiltInDiscovery bool `json:"disableBuiltInDiscovery,omitempty"`
	// When false and discoveryPortNames is empty, the Cryostat application will use the default port name jfr-jmx to look for JMX connectable targets. Defaults to false.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable Built-in Port Names",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	DisableBuiltInPortNames bool `json:"disableBuiltInPortNames,omitempty"`
	// List of port names that the Cryostat application should look for in order to consider a target as JMX connectable.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:fieldDependency:targetDiscoveryOptions.disableBuiltInPortNames:true"}
	DiscoveryPortNames []string `json:"discoveryPortNames,omitempty"`
	// When false and discoveryPortNumbers is empty, the Cryostat application will use the default port number 9091 to look for JMX connectable targets. Defaults to false.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable Built-in Port Numbers",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	DisableBuiltInPortNumbers bool `json:"disableBuiltInPortNumbers,omitempty"`
	// List of port numbers that the Cryostat application should look for in order to consider a target as JMX connectable.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:fieldDependency:targetDiscoveryOptions.disableBuiltInPortNumbers:true"}
	DiscoveryPortNumbers []int32 `json:"discoveryPortNumbers,omitempty"`
}

// DatabaseOptions provides configuration options to the Cryostat application's database.
type DatabaseOptions struct {
	// Name of the secret containing database keys. This secret must contain a CONNECTION_KEY secret which is the
	// database connection password, and an ENCRYPTION_KEY secret which is the key used to encrypt sensitive data
	// stored within the database, such as the target credentials keyring. This field cannot be updated.
	// It is recommended that the secret should be marked as immutable to avoid accidental changes to secret's data.
	// More details: https://kubernetes.io/docs/concepts/configuration/secret/#secret-immutable
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:io.kubernetes:Secret"}
	SecretName *string `json:"secretName,omitempty"`
}

// AgentOptions provides customization for how the operator configures Cryostat Agents.
type AgentOptions struct {
	// Disables hostname verification when Cryostat connects to Agents over TLS.
	// Consider enabling this if the Cryostat Agent fails to determine the hostname of your pod.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Disable Hostname Verification",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	DisableHostnameVerification bool `json:"disableHostnameVerification,omitempty"`
	// Allow insecure (non-TLS) HTTP connections to Cryostat Agents.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Allow Insecure Connections",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:booleanSwitch"}
	AllowInsecure bool `json:"allowInsecure,omitempty"`
	// The resources allocated to the init container used to inject the Cryostat agent,
	// when using the operator's agent auto-configuration feature.
	// Default resource requests will be added to the init container unless specified.
	// Default resource limits will be added to the init container if neither resource requests or limits are specified.
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:resourceRequirements"}
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}
