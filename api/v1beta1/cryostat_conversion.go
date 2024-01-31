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
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

var _ conversion.Convertible = &Cryostat{}

// ConvertTo converts this Cryostat to the Hub version (v1beta2).
func (src *Cryostat) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*operatorv1beta2.Cryostat)

	// Copy ObjectMeta as-is
	dst.ObjectMeta = src.ObjectMeta

	// Convert existing Spec fields
	convertSpecTo(&src.Spec, &dst.Spec)

	// Convert existing Status fields
	convertStatusTo(&src.Status, &dst.Status)

	// Maintain the previous behaviour by using the CR's namespace as the sole target namespace
	dst.Spec.TargetNamespaces = []string{src.Namespace}
	dst.Status.TargetNamespaces = []string{src.Namespace}

	return nil
}

func convertSpecTo(src *CryostatSpec, dst *operatorv1beta2.CryostatSpec) {
	dst.Minimal = src.Minimal
	dst.EnableCertManager = src.EnableCertManager
	dst.TrustedCertSecrets = convertCertSecretsTo(src.TrustedCertSecrets)
	dst.EventTemplates = convertEventTemplatesTo(src.EventTemplates)
	dst.StorageOptions = convertStorageOptionsTo(src.StorageOptions)
	dst.ServiceOptions = convertServiceOptionsTo(src.ServiceOptions)
	dst.NetworkOptions = convertNetworkOptionsTo(src.NetworkOptions)
	dst.ReportOptions = convertReportOptionsTo(src.ReportOptions)
	dst.MaxWsConnections = src.MaxWsConnections
	dst.JmxCacheOptions = convertJmxCacheOptionsTo(src.JmxCacheOptions)
	dst.Resources = convertResourceOptionsTo(src.Resources)
	dst.AuthProperties = convertAuthPropertiesTo(src.AuthProperties)
	dst.SecurityOptions = convertSecurityOptionsTo(src.SecurityOptions)
	dst.SchedulingOptions = convertSchedulingOptionsTo(src.SchedulingOptions)
	dst.TargetDiscoveryOptions = convertTargetDiscoveryTo(src.TargetDiscoveryOptions)
	dst.JmxCredentialsDatabaseOptions = convertDatabaseOptionsTo(src.JmxCredentialsDatabaseOptions)
	dst.OperandMetadata = convertOperandMetadataTo(src.OperandMetadata)
}

func convertStatusTo(src *CryostatStatus, dst *operatorv1beta2.CryostatStatus) {
	dst.ApplicationURL = src.ApplicationURL
	dst.Conditions = src.Conditions
	dst.GrafanaSecret = src.GrafanaSecret
}

func convertCertSecretsTo(srcCerts []CertificateSecret) []operatorv1beta2.CertificateSecret {
	var dstCerts []operatorv1beta2.CertificateSecret
	if srcCerts != nil {
		dstCerts = make([]operatorv1beta2.CertificateSecret, 0, len(srcCerts))
		for _, cert := range srcCerts {
			dstCerts = append(dstCerts, operatorv1beta2.CertificateSecret{
				SecretName:     cert.SecretName,
				CertificateKey: cert.CertificateKey,
			})
		}
	}
	return dstCerts
}

func convertEventTemplatesTo(srcTemplates []TemplateConfigMap) []operatorv1beta2.TemplateConfigMap {
	var dstTemplates []operatorv1beta2.TemplateConfigMap
	if srcTemplates != nil {
		dstTemplates = make([]operatorv1beta2.TemplateConfigMap, 0, len(srcTemplates))
		for _, template := range srcTemplates {
			dstTemplates = append(dstTemplates, operatorv1beta2.TemplateConfigMap{
				ConfigMapName: template.ConfigMapName,
				Filename:      template.Filename,
			})
		}
	}
	return dstTemplates
}

func convertStorageOptionsTo(srcOpts *StorageConfiguration) *operatorv1beta2.StorageConfiguration {
	var dstOpts *operatorv1beta2.StorageConfiguration
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.StorageConfiguration{}
		if srcOpts.PVC != nil {
			dstOpts.PVC = &operatorv1beta2.PersistentVolumeClaimConfig{
				Annotations: srcOpts.PVC.Annotations,
				Labels:      srcOpts.PVC.Labels,
				Spec:        srcOpts.PVC.Spec,
			}
		}
		if srcOpts.EmptyDir != nil {
			dstOpts.EmptyDir = &operatorv1beta2.EmptyDirConfig{
				Enabled:   srcOpts.EmptyDir.Enabled,
				Medium:    srcOpts.EmptyDir.Medium,
				SizeLimit: srcOpts.EmptyDir.SizeLimit,
			}
		}
	}
	return dstOpts
}

func convertServiceOptionsTo(srcOpts *ServiceConfigList) *operatorv1beta2.ServiceConfigList {
	var dstOpts *operatorv1beta2.ServiceConfigList
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.ServiceConfigList{}
		if srcOpts.CoreConfig != nil {
			dstOpts.CoreConfig = &operatorv1beta2.CoreServiceConfig{
				HTTPPort:      srcOpts.CoreConfig.HTTPPort,
				JMXPort:       srcOpts.CoreConfig.JMXPort,
				ServiceConfig: convertServiceConfigTo(srcOpts.CoreConfig.ServiceConfig),
			}
		}
		if srcOpts.GrafanaConfig != nil {
			dstOpts.GrafanaConfig = &operatorv1beta2.GrafanaServiceConfig{
				HTTPPort:      srcOpts.GrafanaConfig.HTTPPort,
				ServiceConfig: convertServiceConfigTo(srcOpts.GrafanaConfig.ServiceConfig),
			}
		}
		if srcOpts.ReportsConfig != nil {
			dstOpts.ReportsConfig = &operatorv1beta2.ReportsServiceConfig{
				HTTPPort:      srcOpts.ReportsConfig.HTTPPort,
				ServiceConfig: convertServiceConfigTo(srcOpts.ReportsConfig.ServiceConfig),
			}
		}
	}
	return dstOpts
}

func convertServiceConfigTo(srcConfig ServiceConfig) operatorv1beta2.ServiceConfig {
	return operatorv1beta2.ServiceConfig{
		ServiceType: srcConfig.ServiceType,
		Annotations: srcConfig.Annotations,
		Labels:      srcConfig.Labels,
	}
}

func convertNetworkOptionsTo(srcOpts *NetworkConfigurationList) *operatorv1beta2.NetworkConfigurationList {
	var dstOpts *operatorv1beta2.NetworkConfigurationList
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.NetworkConfigurationList{
			CoreConfig:    convertNetworkConfigTo(srcOpts.CoreConfig),
			GrafanaConfig: convertNetworkConfigTo(srcOpts.GrafanaConfig),
			CommandConfig: convertNetworkConfigTo(srcOpts.CommandConfig), // TODO Remove this from v1beta2 API
		}
	}
	return dstOpts
}

func convertNetworkConfigTo(srcConfig *NetworkConfiguration) *operatorv1beta2.NetworkConfiguration {
	var dstConfig *operatorv1beta2.NetworkConfiguration
	if srcConfig != nil {
		dstConfig = &operatorv1beta2.NetworkConfiguration{
			IngressSpec: srcConfig.IngressSpec,
			Annotations: srcConfig.Annotations,
			Labels:      srcConfig.Labels,
		}
	}
	return dstConfig
}

func convertReportOptionsTo(srcOpts *ReportConfiguration) *operatorv1beta2.ReportConfiguration {
	var dstOpts *operatorv1beta2.ReportConfiguration
	if srcOpts != nil {
		var dstSecurityOpts *operatorv1beta2.ReportsSecurityOptions
		if srcOpts.SecurityOptions != nil {
			dstSecurityOpts = &operatorv1beta2.ReportsSecurityOptions{
				PodSecurityContext:     srcOpts.SecurityOptions.PodSecurityContext,
				ReportsSecurityContext: srcOpts.SecurityOptions.ReportsSecurityContext,
			}
		}
		dstOpts = &operatorv1beta2.ReportConfiguration{
			Replicas:              srcOpts.Replicas,
			Resources:             srcOpts.Resources,
			SubProcessMaxHeapSize: srcOpts.SubProcessMaxHeapSize,
			SecurityOptions:       dstSecurityOpts,
			SchedulingOptions:     convertSchedulingOptionsTo(srcOpts.SchedulingOptions),
		}
	}
	return dstOpts
}

func convertSchedulingOptionsTo(srcOpts *SchedulingConfiguration) *operatorv1beta2.SchedulingConfiguration {
	var dstOpts *operatorv1beta2.SchedulingConfiguration
	if srcOpts != nil {
		var dstAffinity *operatorv1beta2.Affinity
		if srcOpts.Affinity != nil {
			dstAffinity = &operatorv1beta2.Affinity{
				NodeAffinity:    srcOpts.Affinity.NodeAffinity,
				PodAffinity:     srcOpts.Affinity.PodAffinity,
				PodAntiAffinity: srcOpts.Affinity.PodAntiAffinity,
			}
		}
		dstOpts = &operatorv1beta2.SchedulingConfiguration{
			NodeSelector: srcOpts.NodeSelector,
			Affinity:     dstAffinity,
			Tolerations:  srcOpts.Tolerations,
		}
	}
	return dstOpts
}

func convertJmxCacheOptionsTo(srcOpts *JmxCacheOptions) *operatorv1beta2.JmxCacheOptions {
	var dstOpts *operatorv1beta2.JmxCacheOptions
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.JmxCacheOptions{
			TargetCacheSize: srcOpts.TargetCacheSize,
			TargetCacheTTL:  srcOpts.TargetCacheTTL,
		}
	}
	return dstOpts
}

func convertResourceOptionsTo(srcOpts *ResourceConfigList) *operatorv1beta2.ResourceConfigList {
	var dstOpts *operatorv1beta2.ResourceConfigList
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.ResourceConfigList{
			CoreResources:       srcOpts.CoreResources,
			DataSourceResources: srcOpts.DataSourceResources,
			GrafanaResources:    srcOpts.GrafanaResources,
		}
	}
	return dstOpts
}

func convertAuthPropertiesTo(srcProps *AuthorizationProperties) *operatorv1beta2.AuthorizationProperties {
	var dstProps *operatorv1beta2.AuthorizationProperties
	if srcProps != nil {
		dstProps = &operatorv1beta2.AuthorizationProperties{
			ClusterRoleName: srcProps.ClusterRoleName,
			ConfigMapName:   srcProps.ConfigMapName,
			Filename:        srcProps.Filename,
		}
	}
	return dstProps
}

func convertSecurityOptionsTo(srcOpts *SecurityOptions) *operatorv1beta2.SecurityOptions {
	var dstOpts *operatorv1beta2.SecurityOptions
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.SecurityOptions{
			PodSecurityContext:        srcOpts.PodSecurityContext,
			CoreSecurityContext:       srcOpts.CoreSecurityContext,
			DataSourceSecurityContext: srcOpts.DataSourceSecurityContext,
			GrafanaSecurityContext:    srcOpts.GrafanaSecurityContext,
		}
	}
	return dstOpts
}

func convertTargetDiscoveryTo(srcOpts *TargetDiscoveryOptions) *operatorv1beta2.TargetDiscoveryOptions {
	var dstOpts *operatorv1beta2.TargetDiscoveryOptions
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.TargetDiscoveryOptions{
			BuiltInDiscoveryDisabled: srcOpts.BuiltInDiscoveryDisabled,
		}
	}
	return dstOpts
}

func convertDatabaseOptionsTo(srcOpts *JmxCredentialsDatabaseOptions) *operatorv1beta2.JmxCredentialsDatabaseOptions {
	var dstOpts *operatorv1beta2.JmxCredentialsDatabaseOptions
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.JmxCredentialsDatabaseOptions{
			DatabaseSecretName: srcOpts.DatabaseSecretName,
		}
	}
	return dstOpts
}

func convertOperandMetadataTo(srcOpts *OperandMetadata) *operatorv1beta2.OperandMetadata {
	var dstOpts *operatorv1beta2.OperandMetadata
	if srcOpts != nil {
		dstOpts = &operatorv1beta2.OperandMetadata{
			DeploymentMetadata: convertResourceMetadataTo(srcOpts.DeploymentMetadata),
			PodMetadata:        convertResourceMetadataTo(srcOpts.PodMetadata),
		}
	}
	return dstOpts
}

func convertResourceMetadataTo(srcMeta *ResourceMetadata) *operatorv1beta2.ResourceMetadata {
	var dstMeta *operatorv1beta2.ResourceMetadata
	if srcMeta != nil {
		dstMeta = &operatorv1beta2.ResourceMetadata{
			Annotations: srcMeta.Annotations,
			Labels:      srcMeta.Labels,
		}
	}
	return dstMeta
}

// ConvertFrom converts from the Hub version (v1beta2) to this version.
func (dst *Cryostat) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*operatorv1beta2.Cryostat)

	// Copy ObjectMeta as-is
	dst.ObjectMeta = src.ObjectMeta

	// Convert existing Spec fields
	convertSpecFrom(&src.Spec, &dst.Spec)

	// Convert existing Status fields
	convertStatusFrom(&src.Status, &dst.Status)

	return nil
}

func convertSpecFrom(src *operatorv1beta2.CryostatSpec, dst *CryostatSpec) {
	dst.Minimal = src.Minimal
	dst.EnableCertManager = src.EnableCertManager
	dst.TrustedCertSecrets = convertCertSecretsFrom(src.TrustedCertSecrets)
	dst.EventTemplates = convertEventTemplatesFrom(src.EventTemplates)
	dst.StorageOptions = convertStorageOptionsFrom(src.StorageOptions)
	dst.ServiceOptions = convertServiceOptionsFrom(src.ServiceOptions)
	dst.NetworkOptions = convertNetworkOptionsFrom(src.NetworkOptions)
	dst.ReportOptions = convertReportOptionsFrom(src.ReportOptions)
	dst.MaxWsConnections = src.MaxWsConnections
	dst.JmxCacheOptions = convertJmxCacheOptionsFrom(src.JmxCacheOptions)
	dst.Resources = convertResourceOptionsFrom(src.Resources)
	dst.AuthProperties = convertAuthPropertiesFrom(src.AuthProperties)
	dst.SecurityOptions = convertSecurityOptionsFrom(src.SecurityOptions)
	dst.SchedulingOptions = convertSchedulingOptionsFrom(src.SchedulingOptions)
	dst.TargetDiscoveryOptions = convertTargetDiscoveryFrom(src.TargetDiscoveryOptions)
	dst.JmxCredentialsDatabaseOptions = convertDatabaseOptionsFrom(src.JmxCredentialsDatabaseOptions)
	dst.OperandMetadata = convertOperandMetadataFrom(src.OperandMetadata)
}

func convertStatusFrom(src *operatorv1beta2.CryostatStatus, dst *CryostatStatus) {
	dst.ApplicationURL = src.ApplicationURL
	dst.Conditions = src.Conditions
	dst.GrafanaSecret = src.GrafanaSecret
}

func convertCertSecretsFrom(srcCerts []operatorv1beta2.CertificateSecret) []CertificateSecret {
	var dstCerts []CertificateSecret
	if srcCerts != nil {
		dstCerts = make([]CertificateSecret, 0, len(srcCerts))
		for _, cert := range srcCerts {
			dstCerts = append(dstCerts, CertificateSecret{
				SecretName:     cert.SecretName,
				CertificateKey: cert.CertificateKey,
			})
		}
	}
	return dstCerts
}

func convertEventTemplatesFrom(srcTemplates []operatorv1beta2.TemplateConfigMap) []TemplateConfigMap {
	var dstTemplates []TemplateConfigMap
	if srcTemplates != nil {
		dstTemplates = make([]TemplateConfigMap, 0, len(srcTemplates))
		for _, template := range srcTemplates {
			dstTemplates = append(dstTemplates, TemplateConfigMap{
				ConfigMapName: template.ConfigMapName,
				Filename:      template.Filename,
			})
		}
	}
	return dstTemplates
}

func convertStorageOptionsFrom(srcOpts *operatorv1beta2.StorageConfiguration) *StorageConfiguration {
	var dstOpts *StorageConfiguration
	if srcOpts != nil {
		dstOpts = &StorageConfiguration{}
		if srcOpts.PVC != nil {
			dstOpts.PVC = &PersistentVolumeClaimConfig{
				Annotations: srcOpts.PVC.Annotations,
				Labels:      srcOpts.PVC.Labels,
				Spec:        srcOpts.PVC.Spec,
			}
		}
		if srcOpts.EmptyDir != nil {
			dstOpts.EmptyDir = &EmptyDirConfig{
				Enabled:   srcOpts.EmptyDir.Enabled,
				Medium:    srcOpts.EmptyDir.Medium,
				SizeLimit: srcOpts.EmptyDir.SizeLimit,
			}
		}
	}
	return dstOpts
}

func convertServiceOptionsFrom(srcOpts *operatorv1beta2.ServiceConfigList) *ServiceConfigList {
	var dstOpts *ServiceConfigList
	if srcOpts != nil {
		dstOpts = &ServiceConfigList{}
		if srcOpts.CoreConfig != nil {
			dstOpts.CoreConfig = &CoreServiceConfig{
				HTTPPort:      srcOpts.CoreConfig.HTTPPort,
				JMXPort:       srcOpts.CoreConfig.JMXPort,
				ServiceConfig: convertServiceConfigFrom(srcOpts.CoreConfig.ServiceConfig),
			}
		}
		if srcOpts.GrafanaConfig != nil {
			dstOpts.GrafanaConfig = &GrafanaServiceConfig{
				HTTPPort:      srcOpts.GrafanaConfig.HTTPPort,
				ServiceConfig: convertServiceConfigFrom(srcOpts.GrafanaConfig.ServiceConfig),
			}
		}
		if srcOpts.ReportsConfig != nil {
			dstOpts.ReportsConfig = &ReportsServiceConfig{
				HTTPPort:      srcOpts.ReportsConfig.HTTPPort,
				ServiceConfig: convertServiceConfigFrom(srcOpts.ReportsConfig.ServiceConfig),
			}
		}
	}
	return dstOpts
}

func convertServiceConfigFrom(srcConfig operatorv1beta2.ServiceConfig) ServiceConfig {
	return ServiceConfig{
		ServiceType: srcConfig.ServiceType,
		Annotations: srcConfig.Annotations,
		Labels:      srcConfig.Labels,
	}
}

func convertNetworkOptionsFrom(srcOpts *operatorv1beta2.NetworkConfigurationList) *NetworkConfigurationList {
	var dstOpts *NetworkConfigurationList
	if srcOpts != nil {
		dstOpts = &NetworkConfigurationList{
			CoreConfig:    convertNetworkConfigFrom(srcOpts.CoreConfig),
			GrafanaConfig: convertNetworkConfigFrom(srcOpts.GrafanaConfig),
			CommandConfig: convertNetworkConfigFrom(srcOpts.CommandConfig), // TODO Remove this from v1beta2 API
		}
	}
	return dstOpts
}

func convertNetworkConfigFrom(srcConfig *operatorv1beta2.NetworkConfiguration) *NetworkConfiguration {
	var dstConfig *NetworkConfiguration
	if srcConfig != nil {
		dstConfig = &NetworkConfiguration{
			IngressSpec: srcConfig.IngressSpec,
			Annotations: srcConfig.Annotations,
			Labels:      srcConfig.Labels,
		}
	}
	return dstConfig
}

func convertReportOptionsFrom(srcOpts *operatorv1beta2.ReportConfiguration) *ReportConfiguration {
	var dstOpts *ReportConfiguration
	if srcOpts != nil {
		var dstSecurityOpts *ReportsSecurityOptions
		if srcOpts.SecurityOptions != nil {
			dstSecurityOpts = &ReportsSecurityOptions{
				PodSecurityContext:     srcOpts.SecurityOptions.PodSecurityContext,
				ReportsSecurityContext: srcOpts.SecurityOptions.ReportsSecurityContext,
			}
		}
		dstOpts = &ReportConfiguration{
			Replicas:              srcOpts.Replicas,
			Resources:             srcOpts.Resources,
			SubProcessMaxHeapSize: srcOpts.SubProcessMaxHeapSize,
			SecurityOptions:       dstSecurityOpts,
			SchedulingOptions:     convertSchedulingOptionsFrom(srcOpts.SchedulingOptions),
		}
	}
	return dstOpts
}

func convertSchedulingOptionsFrom(srcOpts *operatorv1beta2.SchedulingConfiguration) *SchedulingConfiguration {
	var dstOpts *SchedulingConfiguration
	if srcOpts != nil {
		var dstAffinity *Affinity
		if srcOpts.Affinity != nil {
			dstAffinity = &Affinity{
				NodeAffinity:    srcOpts.Affinity.NodeAffinity,
				PodAffinity:     srcOpts.Affinity.PodAffinity,
				PodAntiAffinity: srcOpts.Affinity.PodAntiAffinity,
			}
		}
		dstOpts = &SchedulingConfiguration{
			NodeSelector: srcOpts.NodeSelector,
			Affinity:     dstAffinity,
			Tolerations:  srcOpts.Tolerations,
		}
	}
	return dstOpts
}

func convertJmxCacheOptionsFrom(srcOpts *operatorv1beta2.JmxCacheOptions) *JmxCacheOptions {
	var dstOpts *JmxCacheOptions
	if srcOpts != nil {
		dstOpts = &JmxCacheOptions{
			TargetCacheSize: srcOpts.TargetCacheSize,
			TargetCacheTTL:  srcOpts.TargetCacheTTL,
		}
	}
	return dstOpts
}

func convertResourceOptionsFrom(srcOpts *operatorv1beta2.ResourceConfigList) *ResourceConfigList {
	var dstOpts *ResourceConfigList
	if srcOpts != nil {
		dstOpts = &ResourceConfigList{
			CoreResources:       srcOpts.CoreResources,
			DataSourceResources: srcOpts.DataSourceResources,
			GrafanaResources:    srcOpts.GrafanaResources,
		}
	}
	return dstOpts
}

func convertAuthPropertiesFrom(srcProps *operatorv1beta2.AuthorizationProperties) *AuthorizationProperties {
	var dstProps *AuthorizationProperties
	if srcProps != nil {
		dstProps = &AuthorizationProperties{
			ClusterRoleName: srcProps.ClusterRoleName,
			ConfigMapName:   srcProps.ConfigMapName,
			Filename:        srcProps.Filename,
		}
	}
	return dstProps
}

func convertSecurityOptionsFrom(srcOpts *operatorv1beta2.SecurityOptions) *SecurityOptions {
	var dstOpts *SecurityOptions
	if srcOpts != nil {
		dstOpts = &SecurityOptions{
			PodSecurityContext:        srcOpts.PodSecurityContext,
			CoreSecurityContext:       srcOpts.CoreSecurityContext,
			DataSourceSecurityContext: srcOpts.DataSourceSecurityContext,
			GrafanaSecurityContext:    srcOpts.GrafanaSecurityContext,
		}
	}
	return dstOpts
}

func convertTargetDiscoveryFrom(srcOpts *operatorv1beta2.TargetDiscoveryOptions) *TargetDiscoveryOptions {
	var dstOpts *TargetDiscoveryOptions
	if srcOpts != nil {
		dstOpts = &TargetDiscoveryOptions{
			BuiltInDiscoveryDisabled: srcOpts.BuiltInDiscoveryDisabled,
		}
	}
	return dstOpts
}

func convertDatabaseOptionsFrom(srcOpts *operatorv1beta2.JmxCredentialsDatabaseOptions) *JmxCredentialsDatabaseOptions {
	var dstOpts *JmxCredentialsDatabaseOptions
	if srcOpts != nil {
		dstOpts = &JmxCredentialsDatabaseOptions{
			DatabaseSecretName: srcOpts.DatabaseSecretName,
		}
	}
	return dstOpts
}

func convertOperandMetadataFrom(srcOpts *operatorv1beta2.OperandMetadata) *OperandMetadata {
	var dstOpts *OperandMetadata
	if srcOpts != nil {
		dstOpts = &OperandMetadata{
			DeploymentMetadata: convertResourceMetadataFrom(srcOpts.DeploymentMetadata),
			PodMetadata:        convertResourceMetadataFrom(srcOpts.PodMetadata),
		}
	}
	return dstOpts
}

func convertResourceMetadataFrom(srcMeta *operatorv1beta2.ResourceMetadata) *ResourceMetadata {
	var dstMeta *ResourceMetadata
	if srcMeta != nil {
		dstMeta = &ResourceMetadata{
			Annotations: srcMeta.Annotations,
			Labels:      srcMeta.Labels,
		}
	}
	return dstMeta
}
