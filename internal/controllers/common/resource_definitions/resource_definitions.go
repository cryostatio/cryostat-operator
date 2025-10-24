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

package resource_definitions

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	common "github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	appsv1 "k8s.io/api/apps/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ImageTags contains container image tags for each of the images to deploy
type ImageTags struct {
	OAuth2ProxyImageTag         string
	OpenShiftOAuthProxyImageTag string
	CoreImageTag                string
	DatasourceImageTag          string
	GrafanaImageTag             string
	ReportsImageTag             string
	StorageImageTag             string
	DatabaseImageTag            string
	AgentProxyImageTag          string
}

type ServiceSpecs struct {
	AuthProxyURL *url.URL
	CoreURL      *url.URL
	ReportsURL   *url.URL
	InsightsURL  *url.URL
	StorageURL   *url.URL
	DatabaseURL  *url.URL
}

// TLSConfig contains TLS-related information useful when creating other objects
type TLSConfig struct {
	// Name of the TLS secret for Cryostat
	CryostatSecret string
	// Name of the TLS secret for Reports Generator
	ReportsSecret string
	// Name of the TLS secret for Database
	DatabaseSecret string
	// Name of the TLS secret for Storage
	StorageSecret string
	// Name of the TLS secret for the agent proxy
	AgentProxySecret string
	// Name of the secret containing the password for the keystore in CryostatSecret
	KeystorePassSecret string
	// PEM-encoded X.509 certificate for the Cryostat CA
	CACert []byte
}

const (
	defaultAuthProxyCpuRequest        string = "50m"
	defaultAuthProxyMemoryRequest     string = "64Mi"
	defaultAuthProxyCpuLimit          string = "500m"
	defaultAuthProxyMemoryLimit       string = "128Mi"
	defaultCoreCpuRequest             string = "500m"
	defaultCoreMemoryRequest          string = "384Mi"
	defaultCoreCpuLimit               string = "2000m"
	defaultCoreMemoryLimit            string = "1Gi"
	defaultJfrDatasourceCpuRequest    string = "200m"
	defaultJfrDatasourceMemoryRequest string = "200Mi"
	defaultJfrDatasourceCpuLimit      string = "500m"
	defaultJfrDatasourceMemoryLimit   string = "500Mi"
	defaultGrafanaCpuRequest          string = "50m"
	defaultGrafanaMemoryRequest       string = "128Mi"
	defaultGrafanaCpuLimit            string = "500m"
	defaultGrafanaMemoryLimit         string = "256Mi"
	defaultDatabaseCpuRequest         string = "50m"
	defaultDatabaseMemoryRequest      string = "64Mi"
	defaultDatabaseCpuLimit           string = "500m"
	defaultDatabaseMemoryLimit        string = "200Mi"
	defaultStorageCpuRequest          string = "50m"
	defaultStorageMemoryRequest       string = "256Mi"
	defaultStorageCpuLimit            string = "500m"
	defaultStorageMemoryLimit         string = "512Mi"
	defaultReportCpuRequest           string = "500m"
	defaultReportMemoryRequest        string = "512Mi"
	defaultReportCpuLimit             string = "1000m"
	defaultReportMemoryLimit          string = "1Gi"
	defaultAgentProxyCpuRequest       string = "50m"
	defaultAgentProxyMemoryRequest    string = "64Mi"
	defaultAgentProxyCpuLimit         string = "500m"
	defaultAgentProxyMemoryLimit      string = "200Mi"
	OAuth2ConfigFileName              string = "alpha_config.json"
	OAuth2ConfigFilePath              string = "/etc/oauth2_proxy/alpha_config"
	DatabaseName                      string = "cryostat"
	SecretMountPrefix                 string = "/var/run/secrets/operator.cryostat.io"
)

func createMapCopy(in map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range in {
		copy[k] = v
	}
	return copy
}

func createMetadataCopy(in *operatorv1beta2.ResourceMetadata) operatorv1beta2.ResourceMetadata {
	if in == nil {
		return operatorv1beta2.ResourceMetadata{}
	}
	return operatorv1beta2.ResourceMetadata{
		Labels:      createMapCopy(in.Labels),
		Annotations: createMapCopy(in.Annotations),
	}
}

func CorePodLabels(cr *model.CryostatInstance) map[string]string {
	return map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "cryostat",
	}
}

func NewDeploymentForCR(cr *model.CryostatInstance, specs *ServiceSpecs, imageTags *ImageTags,
	tls *TLSConfig, fsGroup int64, openshift bool) (*appsv1.Deployment, error) {
	// Force one replica to avoid lock file and PVC contention
	replicas := int32(1)

	defaultDeploymentLabels := map[string]string{
		"app":                    cr.Name,
		"kind":                   "cryostat",
		"component":              "cryostat",
		"app.kubernetes.io/name": "cryostat",
	}
	defaultDeploymentAnnotations := map[string]string{
		"app.openshift.io/connects-to": constants.OperatorDeploymentName,
	}
	defaultPodLabels := CorePodLabels(cr)
	operandMeta := operatorv1beta2.OperandMetadata{
		DeploymentMetadata: &operatorv1beta2.ResourceMetadata{},
		PodMetadata:        &operatorv1beta2.ResourceMetadata{},
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.DeploymentMetadata != nil {
		deploymentCopy := createMetadataCopy(cr.Spec.OperandMetadata.DeploymentMetadata)
		operandMeta.DeploymentMetadata = &deploymentCopy
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.PodMetadata != nil {
		podCopy := createMetadataCopy(cr.Spec.OperandMetadata.PodMetadata)
		operandMeta.PodMetadata = &podCopy
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name,
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.DeploymentMetadata.Labels,
		Annotations: operandMeta.DeploymentMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name,
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.PodMetadata.Labels,
		Annotations: operandMeta.PodMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&podTemplateMeta, defaultPodLabels, nil)

	pod, err := NewPodForCR(cr, specs, imageTags, tls, fsGroup, openshift)
	if err != nil {
		return nil, err
	}
	return &appsv1.Deployment{
		ObjectMeta: deploymentMeta,
		Spec: appsv1.DeploymentSpec{
			// Selector is immutable, avoid modifying if possible
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       cr.Name,
					"kind":      "cryostat",
					"component": "cryostat",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podTemplateMeta,
				Spec:       *pod,
			},
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
	}, nil
}

func DatabasePodLabels(cr *model.CryostatInstance) map[string]string {
	return map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "database",
	}
}

func NewDeploymentForDatabase(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig,
	openshift bool, fsGroup int64) *appsv1.Deployment {
	replicas := int32(1)

	defaultDeploymentLabels := map[string]string{
		"app":                    cr.Name,
		"kind":                   "cryostat",
		"component":              "database",
		"app.kubernetes.io/name": "cryostat-database",
	}
	defaultDeploymentAnnotations := map[string]string{
		"app.openshift.io/connects-to": cr.Name,
	}
	defaultPodLabels := DatabasePodLabels(cr)
	operandMeta := operatorv1beta2.OperandMetadata{
		DeploymentMetadata: &operatorv1beta2.ResourceMetadata{},
		PodMetadata:        &operatorv1beta2.ResourceMetadata{},
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.DeploymentMetadata != nil {
		deploymentCopy := createMetadataCopy(cr.Spec.OperandMetadata.DeploymentMetadata)
		operandMeta.DeploymentMetadata = &deploymentCopy
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.PodMetadata != nil {
		podCopy := createMetadataCopy(cr.Spec.OperandMetadata.PodMetadata)
		operandMeta.PodMetadata = &podCopy
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-database",
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.DeploymentMetadata.Labels,
		Annotations: operandMeta.DeploymentMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-database",
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.PodMetadata.Labels,
		Annotations: operandMeta.PodMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&podTemplateMeta, defaultPodLabels, nil)

	return &appsv1.Deployment{
		ObjectMeta: deploymentMeta,
		Spec: appsv1.DeploymentSpec{
			// Selector is immutable, avoid modifying if possible
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       cr.Name,
					"kind":      "cryostat",
					"component": "database",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podTemplateMeta,
				Spec:       *NewPodForDatabase(cr, imageTags, tls, openshift, fsGroup),
			},
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
	}
}

func StoragePodLabels(cr *model.CryostatInstance) map[string]string {
	return map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "storage",
	}
}

func NewDeploymentForStorage(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig, openshift bool, fsGroup int64) *appsv1.Deployment {
	replicas := int32(1)

	defaultDeploymentLabels := map[string]string{
		"app":                    cr.Name,
		"kind":                   "cryostat",
		"component":              "storage",
		"app.kubernetes.io/name": "cryostat-storage",
	}
	defaultDeploymentAnnotations := map[string]string{
		"app.openshift.io/connects-to": cr.Name,
	}
	defaultPodLabels := StoragePodLabels(cr)
	operandMeta := operatorv1beta2.OperandMetadata{
		DeploymentMetadata: &operatorv1beta2.ResourceMetadata{},
		PodMetadata:        &operatorv1beta2.ResourceMetadata{},
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.DeploymentMetadata != nil {
		deploymentCopy := createMetadataCopy(cr.Spec.OperandMetadata.DeploymentMetadata)
		operandMeta.DeploymentMetadata = &deploymentCopy
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.PodMetadata != nil {
		podCopy := createMetadataCopy(cr.Spec.OperandMetadata.PodMetadata)
		operandMeta.PodMetadata = &podCopy
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-storage",
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.DeploymentMetadata.Labels,
		Annotations: operandMeta.DeploymentMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-storage",
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.PodMetadata.Labels,
		Annotations: operandMeta.PodMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&podTemplateMeta, defaultPodLabels, nil)

	return &appsv1.Deployment{
		ObjectMeta: deploymentMeta,
		Spec: appsv1.DeploymentSpec{
			// Selector is immutable, avoid modifying if possible
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       cr.Name,
					"kind":      "cryostat",
					"component": "storage",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podTemplateMeta,
				Spec:       *NewPodForStorage(cr, imageTags, tls, openshift, fsGroup),
			},
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
	}
}

func ReportsPodLabels(cr *model.CryostatInstance) map[string]string {
	return map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "reports",
	}
}

func NewDeploymentForReports(cr *model.CryostatInstance, imageTags *ImageTags, serviceSpecs *ServiceSpecs, tls *TLSConfig,
	openshift bool) *appsv1.Deployment {
	replicas := int32(0)
	if cr.Spec.ReportOptions != nil {
		replicas = cr.Spec.ReportOptions.Replicas
	}

	defaultDeploymentLabels := map[string]string{
		"app":                    cr.Name,
		"kind":                   "cryostat",
		"component":              "reports",
		"app.kubernetes.io/name": "cryostat-reports",
	}
	defaultDeploymentAnnotations := map[string]string{
		"app.openshift.io/connects-to": cr.Name,
	}
	defaultPodLabels := ReportsPodLabels(cr)
	operandMeta := operatorv1beta2.OperandMetadata{
		DeploymentMetadata: &operatorv1beta2.ResourceMetadata{},
		PodMetadata:        &operatorv1beta2.ResourceMetadata{},
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.DeploymentMetadata != nil {
		deploymentCopy := createMetadataCopy(cr.Spec.OperandMetadata.DeploymentMetadata)
		operandMeta.DeploymentMetadata = &deploymentCopy
	}
	if cr.Spec.OperandMetadata != nil && cr.Spec.OperandMetadata.PodMetadata != nil {
		podCopy := createMetadataCopy(cr.Spec.OperandMetadata.PodMetadata)
		operandMeta.PodMetadata = &podCopy
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-reports",
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.DeploymentMetadata.Labels,
		Annotations: operandMeta.DeploymentMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-reports",
		Namespace:   cr.InstallNamespace,
		Labels:      operandMeta.PodMetadata.Labels,
		Annotations: operandMeta.PodMetadata.Annotations,
	}
	common.MergeLabelsAndAnnotations(&podTemplateMeta, defaultPodLabels, nil)

	return &appsv1.Deployment{
		ObjectMeta: deploymentMeta,
		Spec: appsv1.DeploymentSpec{
			// Selector is immutable, avoid modifying if possible
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       cr.Name,
					"kind":      "cryostat",
					"component": "reports",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podTemplateMeta,
				Spec:       *NewPodForReports(cr, imageTags, serviceSpecs, tls, openshift),
			},
			Replicas: &replicas,
		},
	}
}

func NewPodForCR(cr *model.CryostatInstance, specs *ServiceSpecs, imageTags *ImageTags,
	tls *TLSConfig, fsGroup int64, openshift bool) (*corev1.PodSpec, error) {
	authProxy, err := NewAuthProxyContainer(cr, specs, imageTags.OAuth2ProxyImageTag, imageTags.OpenShiftOAuthProxyImageTag, tls, openshift)
	if err != nil {
		return nil, err
	}
	coreContainer, err := NewCoreContainer(cr, specs, imageTags.CoreImageTag, tls, openshift)
	if err != nil {
		return nil, err
	}
	containers := []corev1.Container{
		*coreContainer,
		NewGrafanaContainer(cr, imageTags.GrafanaImageTag, tls),
		NewJfrDatasourceContainer(cr, imageTags.DatasourceImageTag, specs, tls),
		*authProxy,
		newAgentProxyContainer(cr, imageTags.AgentProxyImageTag, tls),
	}

	volumes := []corev1.Volume{}
	volSources := []corev1.VolumeProjection{}
	readOnlyMode := int32(0440)

	// Add any TrustedCertSecrets as volumes
	for _, secret := range cr.Spec.TrustedCertSecrets {
		var key string
		if secret.CertificateKey != nil {
			key = *secret.CertificateKey
		} else {
			key = operatorv1beta2.DefaultCertificateKey
		}
		source := corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.SecretName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  key,
						Path: fmt.Sprintf("%s_%s", secret.SecretName, key),
						Mode: &readOnlyMode,
					},
				},
			},
		}
		volSources = append(volSources, source)
	}

	if tls != nil {
		volSources = append(volSources, corev1.VolumeProjection{
			// Add Cryostat self-signed CA
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: tls.CryostatSecret,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  constants.CAKey,
						Path: fmt.Sprintf("%s-%s", cr.Name, constants.CAKey),
						Mode: &readOnlyMode,
					},
				},
			},
		})

		volumes = append(volumes,
			corev1.Volume{
				Name: "auth-proxy-tls-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  tls.CryostatSecret,
						DefaultMode: &readOnlyMode,
					},
				},
			},
			corev1.Volume{
				Name: "agent-proxy-tls-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  tls.AgentProxySecret,
						DefaultMode: &readOnlyMode,
					},
				},
			},
			corev1.Volume{
				Name: "keystore",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tls.CryostatSecret,
						Items: []corev1.KeyToPath{
							{
								Key:  "keystore.p12",
								Path: "keystore.p12",
								Mode: &readOnlyMode,
							},
						},
					},
				},
			},
			corev1.Volume{
				Name: "keystore-pass",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  tls.KeystorePassSecret,
						DefaultMode: &readOnlyMode,
					},
				},
			},
		)

		storageMountPrefix := "s3"
		storageSecretVolume := corev1.Volume{
			Name: "storage-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tls.StorageSecret,
					DefaultMode: &readOnlyMode,
					Items: []corev1.KeyToPath{
						{
							Key:  corev1.TLSCertKey,
							Path: path.Join(storageMountPrefix, corev1.TLSCertKey),
							Mode: &readOnlyMode,
						},
						{
							Key:  constants.CAKey,
							Path: path.Join(storageMountPrefix, constants.CAKey),
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}
		volumes = append(volumes, storageSecretVolume)

		dbTlsVolume := corev1.Volume{
			Name: "database-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tls.DatabaseSecret,
					DefaultMode: &readOnlyMode,
					Items: []corev1.KeyToPath{
						{
							Key:  corev1.TLSCertKey,
							Path: corev1.TLSCertKey,
							Mode: &readOnlyMode,
						},
						{
							Key:  constants.CAKey,
							Path: constants.CAKey,
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}
		volumes = append(volumes, dbTlsVolume)
	}

	// Project certificate secrets into deployment
	certVolume := corev1.Volume{
		Name: "cert-secrets",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: volSources,
			},
		},
	}

	// Add agent proxy config map as a volume
	agentProxyVolume := corev1.Volume{
		Name: "agent-proxy-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Name + "-agent-proxy",
				},
				DefaultMode: &readOnlyMode,
			},
		},
	}

	volumes = append(volumes, certVolume, agentProxyVolume)

	if !openshift {
		// if not deploying openshift oauth-proxy then we must be deploying oauth2_proxy instead
		volumes = append(volumes, corev1.Volume{
			Name: cr.Name + "-oauth2-proxy-cfg",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Name + "-oauth2-proxy-cfg",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  OAuth2ConfigFileName,
							Path: OAuth2ConfigFileName,
							Mode: &readOnlyMode,
						},
					},
				},
			},
		})
	}

	if isBasicAuthEnabled(cr) {
		volumes = append(volumes,
			corev1.Volume{
				Name: cr.Name + "-auth-proxy-htpasswd",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: *cr.Spec.AuthorizationOptions.BasicAuth.SecretName,
					},
				},
			},
		)
	}

	// Add any EventTemplates as volumes
	for _, template := range cr.Spec.EventTemplates {
		eventTemplateVolume := corev1.Volume{
			Name: "template-" + template.ConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: template.ConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  template.Filename,
							Path: template.Filename,
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}
		volumes = append(volumes, eventTemplateVolume)
	}

	// Add Automated Rules as volumes
	for _, rule := range cr.Spec.AutomatedRules {
		ruleVolume := corev1.Volume{
			Name: "rule-" + rule.ConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: rule.ConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  rule.Filename,
							Path: rule.Filename,
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}
		volumes = append(volumes, ruleVolume)
	}

	// Add any ProbeTemplates as volumes
	for _, template := range cr.Spec.ProbeTemplates {
		probeTemplateVolume := corev1.Volume{
			Name: "probe-template-" + template.ConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: template.ConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  template.Filename,
							Path: template.Filename,
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}
		volumes = append(volumes, probeTemplateVolume)
	}

	// Add Declarative Credentials as volumes
	for _, credential := range cr.Spec.DeclarativeCredentials {
		volumes = append(volumes,
			corev1.Volume{
				Name: credential.SecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  credential.SecretName,
						DefaultMode: &readOnlyMode,
					},
				},
			},
		)
	}

	var podSc *corev1.PodSecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.PodSecurityContext != nil {
		podSc = cr.Spec.SecurityOptions.PodSecurityContext
	} else {
		nonRoot := true
		podSc = &corev1.PodSecurityContext{
			// Ensure PV mounts are writable
			FSGroup:        &fsGroup,
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: common.SeccompProfile(openshift),
		}
	}

	var nodeSelector map[string]string
	var affinity *corev1.Affinity
	var tolerations []corev1.Toleration

	if cr.Spec.SchedulingOptions != nil {
		nodeSelector = cr.Spec.SchedulingOptions.NodeSelector

		if cr.Spec.SchedulingOptions.Affinity != nil {
			affinity = &corev1.Affinity{
				NodeAffinity:    cr.Spec.SchedulingOptions.Affinity.NodeAffinity,
				PodAffinity:     cr.Spec.SchedulingOptions.Affinity.PodAffinity,
				PodAntiAffinity: cr.Spec.SchedulingOptions.Affinity.PodAntiAffinity,
			}
		}
		tolerations = cr.Spec.SchedulingOptions.Tolerations
	}

	automountSAToken := true
	return &corev1.PodSpec{
		ServiceAccountName:           cr.Name,
		Volumes:                      volumes,
		Containers:                   containers,
		SecurityContext:              podSc,
		AutomountServiceAccountToken: &automountSAToken,
		NodeSelector:                 nodeSelector,
		Affinity:                     affinity,
		Tolerations:                  tolerations,
	}, nil
}

func NewPodForDatabase(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig, openshift bool, fsGroup int64) *corev1.PodSpec {
	container := []corev1.Container{NewDatabaseContainer(cr, imageTags.DatabaseImageTag, tls)}

	volumes := newVolumeForDatabase(cr)

	if tls != nil {
		readOnlyMode := int32(0440)
		secretVolume := corev1.Volume{
			Name: "database-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tls.DatabaseSecret,
					DefaultMode: &readOnlyMode,
				},
			},
		}
		volumes = append(volumes, secretVolume)
	}

	var podSc *corev1.PodSecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.PodSecurityContext != nil {
		podSc = cr.Spec.SecurityOptions.PodSecurityContext
	} else {
		nonRoot := true
		podSc = &corev1.PodSecurityContext{
			// Ensure PV mounts are writable
			FSGroup:        &fsGroup,
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: common.SeccompProfile(openshift),
		}
	}

	var nodeSelector map[string]string
	var affinity *corev1.Affinity
	var tolerations []corev1.Toleration

	if cr.Spec.SchedulingOptions != nil {
		nodeSelector = cr.Spec.SchedulingOptions.NodeSelector

		if cr.Spec.SchedulingOptions.Affinity != nil {
			affinity = &corev1.Affinity{
				NodeAffinity:    cr.Spec.SchedulingOptions.Affinity.NodeAffinity,
				PodAffinity:     cr.Spec.SchedulingOptions.Affinity.PodAffinity,
				PodAntiAffinity: cr.Spec.SchedulingOptions.Affinity.PodAntiAffinity,
			}
		}
		tolerations = cr.Spec.SchedulingOptions.Tolerations
	}

	return &corev1.PodSpec{
		Containers:      container,
		NodeSelector:    nodeSelector,
		Affinity:        affinity,
		Tolerations:     tolerations,
		SecurityContext: podSc,
		Volumes:         volumes,
	}
}

func DeployManagedStorage(cr *model.CryostatInstance) bool {
	return cr.Spec.ObjectStorageOptions == nil ||
		cr.Spec.ObjectStorageOptions.Provider == nil ||
		cr.Spec.ObjectStorageOptions.Provider.URL == nil
}

func NewPodForStorage(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig, openshift bool, fsGroup int64) *corev1.PodSpec {
	container := []corev1.Container{NewStorageContainer(cr, imageTags.StorageImageTag, tls)}

	volumes := newVolumeForStorage(cr)

	readOnlyMode := int32(0440)
	if tls != nil {
		secretVolume := corev1.Volume{
			Name: "storage-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tls.StorageSecret,
					DefaultMode: &readOnlyMode,
				},
			},
		}
		volumes = append(volumes, secretVolume)
	}

	var podSc *corev1.PodSecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.PodSecurityContext != nil {
		podSc = cr.Spec.SecurityOptions.PodSecurityContext
	} else {
		nonRoot := true
		podSc = &corev1.PodSecurityContext{
			// Ensure PV mounts are writable
			FSGroup:        &fsGroup,
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: common.SeccompProfile(openshift),
		}
	}

	var nodeSelector map[string]string
	var affinity *corev1.Affinity
	var tolerations []corev1.Toleration

	if cr.Spec.SchedulingOptions != nil {
		nodeSelector = cr.Spec.SchedulingOptions.NodeSelector

		if cr.Spec.SchedulingOptions.Affinity != nil {
			affinity = &corev1.Affinity{
				NodeAffinity:    cr.Spec.SchedulingOptions.Affinity.NodeAffinity,
				PodAffinity:     cr.Spec.SchedulingOptions.Affinity.PodAffinity,
				PodAntiAffinity: cr.Spec.SchedulingOptions.Affinity.PodAntiAffinity,
			}
		}
		tolerations = cr.Spec.SchedulingOptions.Tolerations
	}

	return &corev1.PodSpec{
		Containers:      container,
		NodeSelector:    nodeSelector,
		Affinity:        affinity,
		Tolerations:     tolerations,
		SecurityContext: podSc,
		Volumes:         volumes,
	}
}

func NewReportContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.ReportOptions != nil {
		resources = cr.Spec.ReportOptions.Resources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultReportCpuRequest, defaultReportMemoryRequest,
		defaultReportCpuLimit, defaultReportMemoryLimit)
	return resources
}

func NewPodForReports(cr *model.CryostatInstance, imageTags *ImageTags, serviceSpecs *ServiceSpecs, tls *TLSConfig, openshift bool) *corev1.PodSpec {
	resources := NewReportContainerResource(cr)
	cpus := resources.Requests.Cpu().Value() // Round to 1 if cpu request < 1000m
	if limits := resources.Limits; limits != nil {
		if cpu := limits.Cpu(); cpu != nil {
			cpus = cpu.Value()
		}
	}
	javaOpts := fmt.Sprintf("-XX:+PrintCommandLineFlags -XX:ActiveProcessorCount=%d -Dorg.openjdk.jmc.flightrecorder.parser.singlethreaded=%t", cpus, cpus < 2)

	envs := []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: "0.0.0.0",
		},
	}
	mounts := []corev1.VolumeMount{}
	volumes := []corev1.Volume{}

	// Configure TLS key/cert if enabled
	livenessProbeScheme := corev1.URISchemeHTTP
	if tls != nil {
		readOnlyMode := int32(0440)
		volumes = append(volumes,
			corev1.Volume{
				Name: "storage-tls-truststore",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tls.StorageSecret,
						Items: []corev1.KeyToPath{
							{
								Key:  "ca.crt",
								Path: "ca.crt",
								Mode: &readOnlyMode,
							},
							{
								Key:  "tls.crt",
								Path: "tls.crt",
								Mode: &readOnlyMode,
							},
						},
					},
				},
			},
		)
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "storage-tls-truststore",
			MountPath: path.Join(SecretMountPrefix, tls.StorageSecret),
			ReadOnly:  true,
		})

		tlsEnvs := []corev1.EnvVar{
			{
				Name:  "QUARKUS_HTTP_SSL_PORT",
				Value: strconv.Itoa(int(constants.ReportsContainerPort)),
			},
			{
				Name:  "QUARKUS_HTTP_INSECURE_REQUESTS",
				Value: "disabled",
			},
		}

		// if we are deploying our own managed storage container with a TLS cert that we issued for it,
		// configure that here. Otherwise if we are configured to talk to an external object storage
		// provider, assume that it is using a well-known certificate signed by a root trust.
		// TODO allow additional configuration via the CR to configure TLS for external providers
		if DeployManagedStorage(cr) {
			tlsEnvs = append(tlsEnvs,
				corev1.EnvVar{
					Name:  "CRYOSTAT_STORAGE_TLS_CA_PATH",
					Value: path.Join(SecretMountPrefix, tls.StorageSecret, "ca.crt"),
				},
				corev1.EnvVar{
					Name:  "CRYOSTAT_STORAGE_TLS_CERT_PATH",
					Value: path.Join(SecretMountPrefix, tls.StorageSecret, "tls.crt"),
				},
			)
		}

		tlsConfigName := "https"
		javaOpts += fmt.Sprintf(" -Dquarkus.http.tls-configuration-name=%s", tlsConfigName)
		javaOpts += fmt.Sprintf(" -Dquarkus.tls.%s.reload-period=%s", tlsConfigName, "1h")
		javaOpts += fmt.Sprintf(" -Dquarkus.tls.%s.key-store.pem.0.cert=%s",
			tlsConfigName,
			path.Join(SecretMountPrefix, tls.ReportsSecret, corev1.TLSCertKey),
		)
		javaOpts += fmt.Sprintf(" -Dquarkus.tls.%s.key-store.pem.0.key=%s",
			tlsConfigName,
			path.Join(SecretMountPrefix, tls.ReportsSecret, corev1.TLSPrivateKeyKey),
		)
		tlsSecretMount := corev1.VolumeMount{
			Name:      "reports-tls-secret",
			MountPath: path.Join(SecretMountPrefix, tls.ReportsSecret),
			ReadOnly:  true,
		}

		secretVolume := corev1.Volume{
			Name: "reports-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tls.ReportsSecret,
				},
			},
		}

		envs = append(envs, tlsEnvs...)
		mounts = append(mounts, tlsSecretMount)
		volumes = append(volumes, secretVolume)

		// Use HTTPS for liveness probe
		livenessProbeScheme = corev1.URISchemeHTTPS
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_PORT",
			Value: strconv.Itoa(int(constants.ReportsContainerPort)),
		})
	}

	probeHandler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Scheme: livenessProbeScheme,
			Port:   intstr.IntOrString{IntVal: constants.ReportsContainerPort},
			Path:   "/health",
		},
	}

	var podSc *corev1.PodSecurityContext
	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SecurityOptions != nil && cr.Spec.ReportOptions.SecurityOptions.PodSecurityContext != nil {
		podSc = cr.Spec.ReportOptions.SecurityOptions.PodSecurityContext
	} else {
		nonRoot := true
		podSc = &corev1.PodSecurityContext{
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: common.SeccompProfile(openshift),
		}
	}

	var containerSc *corev1.SecurityContext
	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SecurityOptions != nil && cr.Spec.ReportOptions.SecurityOptions.ReportsSecurityContext != nil {
		containerSc = cr.Spec.ReportOptions.SecurityOptions.ReportsSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	var nodeSelector map[string]string
	var affinity *corev1.Affinity
	var tolerations []corev1.Toleration

	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SchedulingOptions != nil {
		schedulingOptions := cr.Spec.ReportOptions.SchedulingOptions
		nodeSelector = schedulingOptions.NodeSelector
		if schedulingOptions.Affinity != nil {
			affinity = &corev1.Affinity{
				NodeAffinity:    schedulingOptions.Affinity.NodeAffinity,
				PodAffinity:     schedulingOptions.Affinity.PodAffinity,
				PodAntiAffinity: schedulingOptions.Affinity.PodAntiAffinity,
			}
		}
		tolerations = schedulingOptions.Tolerations
	}

	envs = append(envs, corev1.EnvVar{
		Name:  "JAVA_OPTS_APPEND",
		Value: javaOpts,
	})

	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:            cr.Name + "-reports",
				Image:           imageTags.ReportsImageTag,
				ImagePullPolicy: common.GetPullPolicy(imageTags.ReportsImageTag),
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: constants.ReportsContainerPort,
					},
				},
				Env:          envs,
				VolumeMounts: mounts,
				Resources:    *resources,
				LivenessProbe: &corev1.Probe{
					ProbeHandler: probeHandler,
				},
				StartupProbe: &corev1.Probe{
					ProbeHandler: probeHandler,
				},
				SecurityContext: containerSc,
			},
		},
		Volumes:         volumes,
		NodeSelector:    nodeSelector,
		Affinity:        affinity,
		Tolerations:     tolerations,
		SecurityContext: podSc,
	}
}

func NewAuthProxyContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.AuthProxyResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultAuthProxyCpuRequest, defaultAuthProxyMemoryRequest,
		defaultAuthProxyCpuLimit, defaultAuthProxyMemoryLimit)
	return resources
}

func NewAuthProxyContainer(cr *model.CryostatInstance, specs *ServiceSpecs, oauth2ProxyImageTag string, openshiftAuthProxyImageTag string,
	tls *TLSConfig, openshift bool) (*corev1.Container, error) {
	if openshift {
		return NewOpenShiftAuthProxyContainer(cr, specs, openshiftAuthProxyImageTag, tls)
	}
	return NewOAuth2ProxyContainer(cr, specs, oauth2ProxyImageTag, tls)
}

func NewOpenShiftAuthProxyContainer(cr *model.CryostatInstance, specs *ServiceSpecs, imageTag string,
	tls *TLSConfig) (*corev1.Container, error) {
	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.AuthProxySecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.AuthProxySecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	args := []string{
		"--pass-access-token=false",
		"--pass-user-bearer-token=false",
		"--pass-basic-auth=false",
		fmt.Sprintf("--upstream=http://localhost:%d/", constants.CryostatHTTPContainerPort),
		fmt.Sprintf("--upstream=http://localhost:%d/grafana/", constants.GrafanaContainerPort),
		fmt.Sprintf("--openshift-service-account=%s", cr.Name),
		"--proxy-websockets=true",
		"--proxy-prefix=/oauth2",
	}
	if isOpenShiftAuthProxyDisabled(cr) {
		args = append(args, "--bypass-auth-for=.*")
	} else {
		args = append(args, "--bypass-auth-for=^/health(/liveness)?$")
	}

	subjectAccessReviewJson, err := json.Marshal([]authzv1.ResourceAttributes{getOpenShiftAccessReview(cr)})
	if err != nil {
		return nil, err
	}
	args = append(args, fmt.Sprintf("--openshift-sar=%s", string(subjectAccessReviewJson)))

	delegateUrls := make(map[string]authzv1.ResourceAttributes)
	delegateUrls["/"] = getOpenShiftAccessReview(cr)
	tokenReviewJson, err := json.Marshal(delegateUrls)
	if err != nil {
		return nil, err
	}
	args = append(args, fmt.Sprintf("--openshift-delegate-urls=%s", string(tokenReviewJson)))

	volumeMounts := []corev1.VolumeMount{}

	if isBasicAuthEnabled(cr) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      cr.Name + "-auth-proxy-htpasswd",
			MountPath: SecretMountPrefix,
			ReadOnly:  true,
		})
		args = append(args, fmt.Sprintf("--htpasswd-file=%s", path.Join(SecretMountPrefix, *cr.Spec.AuthorizationOptions.BasicAuth.Filename)))
	}
	args = append(args,
		fmt.Sprintf("--skip-provider-button=%t", !isBasicAuthEnabled(cr)),
	)

	livenessProbeScheme := corev1.URISchemeHTTP
	if tls != nil {
		args = append(args,
			fmt.Sprintf("--http-address="),
			fmt.Sprintf("--https-address=0.0.0.0:%d", constants.AuthProxyHttpContainerPort),
			fmt.Sprintf("--tls-cert=%s", path.Join(SecretMountPrefix, tls.CryostatSecret, corev1.TLSCertKey)),
			fmt.Sprintf("--tls-key=%s", path.Join(SecretMountPrefix, tls.CryostatSecret, corev1.TLSPrivateKeyKey)),
		)

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "auth-proxy-tls-secret",
			MountPath: path.Join(SecretMountPrefix, tls.CryostatSecret),
			ReadOnly:  true,
		})

		livenessProbeScheme = corev1.URISchemeHTTPS
	} else {
		args = append(args,
			fmt.Sprintf("--http-address=0.0.0.0:%d", constants.AuthProxyHttpContainerPort),
			"--https-address=",
		)
	}

	cookieOptional := false
	return &corev1.Container{
		Name:            cr.Name + "-auth-proxy",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		VolumeMounts:    volumeMounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.AuthProxyHttpContainerPort,
			},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Name + "-oauth2-cookie",
					},
					Optional: &cookieOptional,
				},
			},
		},
		Resources: *NewAuthProxyContainerResource(cr),
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: constants.AuthProxyHttpContainerPort},
					Path:   "/oauth2/healthz",
					Scheme: livenessProbeScheme,
				},
			},
		},
		SecurityContext: containerSc,
		Args:            args,
	}, nil
}

func isOpenShiftAuthProxyDisabled(cr *model.CryostatInstance) bool {
	if cr.Spec.AuthorizationOptions != nil && cr.Spec.AuthorizationOptions.OpenShiftSSO != nil && cr.Spec.AuthorizationOptions.OpenShiftSSO.Disable != nil {
		return *cr.Spec.AuthorizationOptions.OpenShiftSSO.Disable
	}
	return false
}

func getOpenShiftAccessReview(cr *model.CryostatInstance) authzv1.ResourceAttributes {
	if cr.Spec.AuthorizationOptions != nil && cr.Spec.AuthorizationOptions.OpenShiftSSO != nil && cr.Spec.AuthorizationOptions.OpenShiftSSO.AccessReview != nil {
		return *cr.Spec.AuthorizationOptions.OpenShiftSSO.AccessReview
	}
	return getDefaultOpenShiftAccessRole(cr)
}

func getDefaultOpenShiftAccessRole(cr *model.CryostatInstance) authzv1.ResourceAttributes {
	return authzv1.ResourceAttributes{
		Namespace:   cr.InstallNamespace,
		Verb:        "create",
		Group:       "",
		Version:     "",
		Resource:    "pods",
		Subresource: "exec",
		Name:        "",
	}
}

func NewOAuth2ProxyContainer(cr *model.CryostatInstance, specs *ServiceSpecs, imageTag string,
	tls *TLSConfig) (*corev1.Container, error) {
	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.AuthProxySecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.AuthProxySecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	envs := []corev1.EnvVar{
		{
			Name:  "OAUTH2_PROXY_REDIRECT_URL",
			Value: fmt.Sprintf("http://localhost:%d/oauth2/callback", constants.AuthProxyHttpContainerPort),
		},
		{
			Name:  "OAUTH2_PROXY_EMAIL_DOMAINS",
			Value: "*",
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      cr.Name + "-oauth2-proxy-cfg",
			MountPath: OAuth2ConfigFilePath,
			ReadOnly:  true,
		},
	}

	livenessProbeScheme := corev1.URISchemeHTTP
	if tls != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "auth-proxy-tls-secret",
			MountPath: path.Join(SecretMountPrefix, tls.CryostatSecret),
			ReadOnly:  true,
		})

		livenessProbeScheme = corev1.URISchemeHTTPS
	}

	if isBasicAuthEnabled(cr) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      cr.Name + "-auth-proxy-htpasswd",
			MountPath: SecretMountPrefix,
			ReadOnly:  true,
		})
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "OAUTH2_PROXY_HTPASSWD_FILE",
				Value: path.Join(SecretMountPrefix, *cr.Spec.AuthorizationOptions.BasicAuth.Filename),
			},
			{
				Name:  "OAUTH2_PROXY_HTPASSWD_USER_GROUP",
				Value: "write",
			},
			{
				Name:  "OAUTH2_PROXY_SKIP_AUTH_ROUTES",
				Value: "^/health(/liveness)?$",
			},
		}...)
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "OAUTH2_PROXY_SKIP_AUTH_ROUTES",
			Value: ".*",
		})
	}

	cookieOptional := false
	return &corev1.Container{
		Name:            cr.Name + "-auth-proxy",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		VolumeMounts:    volumeMounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.AuthProxyHttpContainerPort,
			},
		},
		Env: envs,
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Name + "-oauth2-cookie",
					},
					Optional: &cookieOptional,
				},
			},
		},
		Resources: *NewAuthProxyContainerResource(cr),
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: constants.AuthProxyHttpContainerPort},
					Path:   "/ping",
					Scheme: livenessProbeScheme,
				},
			},
		},
		SecurityContext: containerSc,
		Args: []string{
			fmt.Sprintf("--alpha-config=%s", path.Join(OAuth2ConfigFilePath, OAuth2ConfigFileName)),
		},
	}, nil
}

func NewCoreContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.CoreResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultCoreCpuRequest, defaultCoreMemoryRequest,
		defaultCoreCpuLimit, defaultCoreMemoryLimit)
	return resources
}

func NewCoreContainer(cr *model.CryostatInstance, specs *ServiceSpecs, imageTag string,
	tls *TLSConfig, openshift bool) (*corev1.Container, error) {
	configPath := "/opt/cryostat.d/conf.d"
	templatesPath := "/opt/cryostat.d/templates.d"
	rulesPath := "/opt/cryostat.d/rules.d"
	probeTemplatesPath := "/opt/cryostat.d/probes.d"
	credentialsPath := "/opt/cryostat.d/credentials.d"

	envs := []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: "localhost",
		},
		{
			Name:  "QUARKUS_HTTP_PORT",
			Value: "8181",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_PROXY_ADDRESS_FORWARDING",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_ALLOW_X_FORWARDED",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_ENABLE_FORWARDED_HOST",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_ENABLE_FORWARDED_PREFIX",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HIBERNATE_ORM_DATABASE_GENERATION",
			Value: "none",
		},
		{
			Name:  "QUARKUS_HIBERNATE_ORM_SQL_LOAD_SCRIPT",
			Value: "no-file",
		},
		{
			Name:  "QUARKUS_DATASOURCE_USERNAME",
			Value: "cryostat",
		},
		{
			Name:  "CRYOSTAT_CONFIG_PATH",
			Value: configPath,
		},
		{
			Name:  "CRYOSTAT_TEMPLATE_PATH",
			Value: templatesPath,
		},
		{
			Name: "QUARKUS_S3_SYNC_CLIENT_TLS_KEY_MANAGERS_PROVIDER_TYPE",
			// do not present a TLS client certificate
			Value: "none",
		},
		{
			Name: "QUARKUS_S3_SYNC_CLIENT_TLS_TRUST_MANAGERS_PROVIDER_TYPE",
			// cryostat's truststore should include the storage certificate, so the S3 client should be able to load it from the system
			Value: "system-property",
		},
		{
			Name:  "QUARKUS_S3_AWS_CREDENTIALS_TYPE",
			Value: "static",
		},
	}

	if DeployManagedStorage(cr) {
		// default environment variable settings for managed/provisioned cryostat-storage instance
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "QUARKUS_S3_ENDPOINT_OVERRIDE",
				Value: specs.StorageURL.String(),
			},
			{
				Name:  "QUARKUS_S3_PATH_STYLE_ACCESS",
				Value: "true",
			},
			{
				Name:  "QUARKUS_S3_AWS_REGION",
				Value: "us-east-1",
			},
			{
				Name:  "STORAGE_PRESIGNED_TRANSFERS_ENABLED",
				Value: "true",
			},
			{
				Name:  "STORAGE_PRESIGNED_DOWNLOADS_ENABLED",
				Value: "false",
			},
		}...)
	} else {
		if cr.Spec.ObjectStorageOptions.Provider.URL == nil {
			return nil, fmt.Errorf("cr.Spec.ObjectStorageOptions was not nil, but cr.Spec.ObjectStorageOptions.Provider.URL was nil")
		}
		if cr.Spec.ObjectStorageOptions.Provider.Region == nil {
			return nil, fmt.Errorf("cr.Spec.ObjectStorageOptions was not nil, but cr.Spec.ObjectStorageOptions.Provider.Region was nil")
		}
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "QUARKUS_S3_ENDPOINT_OVERRIDE",
				Value: *cr.Spec.ObjectStorageOptions.Provider.URL,
			},
			{
				Name:  "QUARKUS_S3_AWS_REGION",
				Value: *cr.Spec.ObjectStorageOptions.Provider.Region,
			},
		}...)

		useVirtualHostAccess := false
		if cr.Spec.ObjectStorageOptions.Provider != nil && cr.Spec.ObjectStorageOptions.Provider.UseVirtualHostAccess != nil {
			useVirtualHostAccess = *cr.Spec.ObjectStorageOptions.Provider.UseVirtualHostAccess
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_S3_PATH_STYLE_ACCESS",
			Value: strconv.FormatBool(!useVirtualHostAccess),
		})

		disablePresignedFileTransfers := false
		if cr.Spec.ObjectStorageOptions.Provider != nil && cr.Spec.ObjectStorageOptions.Provider.DisablePresignedFileTransfers != nil {
			disablePresignedFileTransfers = *cr.Spec.ObjectStorageOptions.Provider.DisablePresignedFileTransfers
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "STORAGE_PRESIGNED_TRANSFERS_ENABLED",
			Value: strconv.FormatBool(!disablePresignedFileTransfers),
		})
		if cr.Spec.ObjectStorageOptions.Provider != nil && cr.Spec.ObjectStorageOptions.Provider.DisablePresignedDownloads != nil {
			envs = append(envs, corev1.EnvVar{
				Name:  "STORAGE_PRESIGNED_DOWNLOADS_ENABLED",
				Value: strconv.FormatBool(!*cr.Spec.ObjectStorageOptions.Provider.DisablePresignedDownloads),
			})
		}

		metadataMode := "tagging"
		if cr.Spec.ObjectStorageOptions.Provider != nil && cr.Spec.ObjectStorageOptions.Provider.MetadataMode != nil {
			metadataMode = *cr.Spec.ObjectStorageOptions.Provider.MetadataMode
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "STORAGE_METADATA_STORAGE_MODE",
			Value: metadataMode,
		})

		if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions != nil {
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.ArchivedRecordings != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "STORAGE_BUCKETS_ARCHIVES_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.ArchivedRecordings,
				})
			}
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.ArchivedReports != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "CRYOSTAT_SERVICES_REPORTS_STORAGE_CACHE_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.ArchivedReports,
				})
			}
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.EventTemplates != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "STORAGE_BUCKETS_EVENT_TEMPLATES_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.EventTemplates,
				})
			}
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.JMCAgentProbeTemplates != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "STORAGE_BUCKETS_PROBE_TEMPLATES_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.JMCAgentProbeTemplates,
				})
			}
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.HeapDumps != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "STORAGE_BUCKETS_HEAP_DUMPS_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.HeapDumps,
				})
			}
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.ThreadDumps != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "STORAGE_BUCKETS_THREAD_DUMPS_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.ThreadDumps,
				})
			}
			if cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.Metadata != nil {
				envs = append(envs, corev1.EnvVar{
					Name:  "STORAGE_BUCKETS_METADATA_NAME",
					Value: *cr.Spec.ObjectStorageOptions.StorageBucketNameOptions.Metadata,
				})
			}
		}

		tlsTrustAll := false
		if cr.Spec.ObjectStorageOptions.Provider != nil && cr.Spec.ObjectStorageOptions.Provider.TLSTrustAll != nil {
			tlsTrustAll = *cr.Spec.ObjectStorageOptions.Provider.TLSTrustAll
		}
		if tlsTrustAll {
			envs = append(envs, corev1.EnvVar{
				Name:  "QUARKUS_S3_SYNC_CLIENT_TLS_TRUST_MANAGERS_PROVIDER_TYPE",
				Value: "trust-all",
			})
		}
	}

	mounts := []corev1.VolumeMount{
		{
			// Mount the CA cert and user certificates in the expected /truststore location
			Name:      "cert-secrets",
			MountPath: "/truststore/operator",
			ReadOnly:  true,
		},
	}
	if tls != nil {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "storage-tls-secret",
				MountPath: "/truststore/storage",
				ReadOnly:  true,
			},
			corev1.VolumeMount{
				Name:      "keystore",
				MountPath: path.Join(SecretMountPrefix, "client-tls", tls.CryostatSecret),
				ReadOnly:  true,
			},
			corev1.VolumeMount{
				Name:      "keystore-pass",
				MountPath: path.Join(SecretMountPrefix, "client-tls", tls.KeystorePassSecret),
				ReadOnly:  true,
			},
		)

		envs = append(envs,
			corev1.EnvVar{
				Name:  "SSL_KEYSTORE",
				Value: path.Join(SecretMountPrefix, "client-tls", tls.CryostatSecret, constants.KeyStoreFile),
			},
			corev1.EnvVar{
				Name:  "SSL_KEYSTORE_PASS_FILE",
				Value: path.Join(SecretMountPrefix, "client-tls", tls.KeystorePassSecret, constants.KeystorePassSecretKey),
			},
		)
	}

	optional := false
	secretName := getDatabaseSecret(cr)
	envs = append(envs, corev1.EnvVar{
		Name: "QUARKUS_DATASOURCE_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      constants.DatabaseSecretConnectionKey,
				Optional: &optional,
			},
		},
	})

	secretName = getStorageSecret(cr)
	envs = append(envs,
		corev1.EnvVar{
			Name: "QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key:      "ACCESS_KEY",
					Optional: &optional,
				},
			},
		},
		corev1.EnvVar{
			Name: "QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key:      "SECRET_KEY",
					Optional: &optional,
				},
			},
		},
	)

	envs = append(envs,
		corev1.EnvVar{
			Name:  "AWS_ACCESS_KEY_ID",
			Value: "$(QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID)",
		},
		corev1.EnvVar{
			Name:  "AWS_SECRET_ACCESS_KEY",
			Value: "$(QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_SECRET_ACCESS_KEY)",
		},
	)

	if specs.ReportsURL != nil {
		reportsEnvs := []corev1.EnvVar{
			{
				Name:  "QUARKUS_REST_CLIENT_REPORTS_URL",
				Value: specs.ReportsURL.String(),
			},
		}
		envs = append(envs, reportsEnvs...)
	}

	// Define INSIGHTS_PROXY URL if Insights integration is enabled
	if specs.InsightsURL != nil {
		insightsEnvs := []corev1.EnvVar{
			{
				Name:  "INSIGHTS_PROXY",
				Value: specs.InsightsURL.String(),
			},
		}
		envs = append(envs, insightsEnvs...)
	}

	targetCacheSize := "-1"
	targetCacheTTL := "10"
	if cr.Spec.TargetConnectionCacheOptions != nil {
		if cr.Spec.TargetConnectionCacheOptions.TargetCacheSize != 0 {
			targetCacheSize = strconv.Itoa(int(cr.Spec.TargetConnectionCacheOptions.TargetCacheSize))
		}

		if cr.Spec.TargetConnectionCacheOptions.TargetCacheTTL != 0 {
			targetCacheTTL = strconv.Itoa(int(cr.Spec.TargetConnectionCacheOptions.TargetCacheTTL))
		}
	}
	connectionCacheEnvs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_CONNECTIONS_MAX_OPEN",
			Value: targetCacheSize,
		},
		{
			Name:  "CRYOSTAT_CONNECTIONS_TTL",
			Value: targetCacheTTL,
		},
	}
	envs = append(envs, connectionCacheEnvs...)

	k8sDiscoveryEnabled := true
	k8sDiscoveryPortNames := "jfr-jmx"
	k8sDiscoveryPortNumbers := "9091"
	if cr.Spec.TargetDiscoveryOptions != nil {
		k8sDiscoveryEnabled = !cr.Spec.TargetDiscoveryOptions.DisableBuiltInDiscovery

		if len(cr.Spec.TargetDiscoveryOptions.DiscoveryPortNames) > 0 {
			k8sDiscoveryPortNames = strings.Join(cr.Spec.TargetDiscoveryOptions.DiscoveryPortNames[:], ",")
		} else if cr.Spec.TargetDiscoveryOptions.DisableBuiltInPortNames {
			k8sDiscoveryPortNames = ""
		}

		if len(cr.Spec.TargetDiscoveryOptions.DiscoveryPortNumbers) > 0 {
			k8sDiscoveryPortNumbers = strings.Trim(strings.ReplaceAll(fmt.Sprint(cr.Spec.TargetDiscoveryOptions.DiscoveryPortNumbers), " ", ","), "[]")
		} else if cr.Spec.TargetDiscoveryOptions.DisableBuiltInPortNumbers {
			k8sDiscoveryPortNumbers = ""
		}
	}
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_ENABLED",
			Value: fmt.Sprintf("%t", k8sDiscoveryEnabled),
		},
		{
			Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_NAMESPACES",
			Value: strings.Join(cr.TargetNamespaces, ","),
		},
		{
			Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NAMES",
			Value: k8sDiscoveryPortNames,
		},
		{
			Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NUMBERS",
			Value: k8sDiscoveryPortNumbers,
		},
	}...,
	)

	grafanaVars := []corev1.EnvVar{
		{
			Name:  "GRAFANA_DATASOURCE_URL",
			Value: datasourceURL,
		},
	}
	if specs.AuthProxyURL != nil {
		grafanaVars = append(grafanaVars,
			corev1.EnvVar{
				Name:  "GRAFANA_DASHBOARD_EXT_URL",
				Value: "/grafana/",
			},
			corev1.EnvVar{
				Name:  "GRAFANA_DASHBOARD_URL",
				Value: getInternalDashboardURL(),
			},
		)
	}
	envs = append(envs, grafanaVars...)

	if cr.Spec.AgentOptions != nil {
		if cr.Spec.AgentOptions.DisableHostnameVerification {
			envs = append(envs,
				corev1.EnvVar{
					// TODO This should eventually be replaced by an agent-specific toggle.
					// See: https://github.com/cryostatio/cryostat/issues/778
					Name:  "QUARKUS_REST_CLIENT_VERIFY_HOST",
					Value: "false",
				})
		}
		if cr.Spec.AgentOptions.AllowInsecure {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "CRYOSTAT_AGENT_TLS_REQUIRED",
					Value: "false",
				},
			)
		}
	}

	// Mount the templates specified in Cryostat CR under /opt/cryostat.d/templates.d
	for _, template := range cr.Spec.EventTemplates {
		mount := corev1.VolumeMount{
			Name:      "template-" + template.ConfigMapName,
			MountPath: path.Join(templatesPath, fmt.Sprintf("%s_%s", template.ConfigMapName, template.Filename)),
			SubPath:   template.Filename,
			ReadOnly:  true,
		}
		mounts = append(mounts, mount)
	}

	// Mount the automated rules specified in Cryostat CR under /opt/cryostat.d/rules.d
	for _, rule := range cr.Spec.AutomatedRules {
		mount := corev1.VolumeMount{
			Name:      "rule-" + rule.ConfigMapName,
			MountPath: path.Join(rulesPath, fmt.Sprintf("%s_%s", rule.ConfigMapName, rule.Filename)),
			SubPath:   rule.Filename,
			ReadOnly:  true,
		}
		mounts = append(mounts, mount)
	}

	// Mount the templates specified in Cryostat CR under /opt/cryostat.d/probes.d
	for _, template := range cr.Spec.ProbeTemplates {
		mount := corev1.VolumeMount{
			Name:      "probe-template-" + template.ConfigMapName,
			MountPath: path.Join(probeTemplatesPath, fmt.Sprintf("%s_%s", template.ConfigMapName, template.Filename)),
			SubPath:   template.Filename,
			ReadOnly:  true,
		}
		mounts = append(mounts, mount)
	}

	// Mount the declarative credentials specified under /opt/cryostat.d/credentials.d
	for _, credential := range cr.Spec.DeclarativeCredentials {
		mount := corev1.VolumeMount{
			Name:      credential.SecretName,
			MountPath: path.Join(credentialsPath, credential.SecretName),
			ReadOnly:  true,
		}
		mounts = append(mounts, mount)
	}

	if tls != nil {
		tlsPath := path.Join(SecretMountPrefix, tls.DatabaseSecret)
		tlsSecretMount := corev1.VolumeMount{
			Name:      "database-tls-secret",
			MountPath: tlsPath,
			ReadOnly:  true,
		}
		mounts = append(mounts, tlsSecretMount)
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_DATASOURCE_JDBC_URL",
			Value: fmt.Sprintf("jdbc:postgresql://%s-database.%s.svc.cluster.local:5432/cryostat?ssl=true&sslmode=verify-full&sslcert=&sslrootcert=%s/%s", cr.Name, cr.InstallNamespace, tlsPath, constants.CAKey),
		})
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_DATASOURCE_JDBC_URL",
			Value: fmt.Sprintf("jdbc:postgresql://%s-database.%s.svc.cluster.local:5432/cryostat", cr.Name, cr.InstallNamespace),
		})
	}

	probeHandler := corev1.ProbeHandler{
		Exec: &corev1.ExecAction{
			Command: []string{
				"curl",
				"--fail",
				fmt.Sprintf("http://localhost:%d/health/liveness", constants.CryostatHTTPContainerPort),
			},
		},
	}

	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.CoreSecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.CoreSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	return &corev1.Container{
		Name:            cr.Name,
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		VolumeMounts:    mounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.CryostatHTTPContainerPort,
			},
		},
		Env:       envs,
		Resources: *NewCoreContainerResource(cr),
		LivenessProbe: &corev1.Probe{
			ProbeHandler: probeHandler,
		},
		// Expect probe to succeed within 3 minutes
		StartupProbe: &corev1.Probe{
			ProbeHandler:     probeHandler,
			FailureThreshold: 18,
		},
		SecurityContext: containerSc,
	}, nil
}

func NewGrafanaContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.GrafanaResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultGrafanaCpuRequest, defaultGrafanaMemoryRequest,
		defaultGrafanaCpuLimit, defaultGrafanaMemoryLimit)
	return resources
}

func NewGrafanaContainer(cr *model.CryostatInstance, imageTag string, tls *TLSConfig) corev1.Container {
	envs := []corev1.EnvVar{
		{
			Name:  "GF_AUTH_ANONYMOUS_ENABLED",
			Value: "true",
		},
		{
			Name:  "GF_SERVER_DOMAIN",
			Value: "localhost",
		},
		{
			Name:  "GF_SERVER_SERVE_FROM_SUB_PATH",
			Value: "true",
		},
		{
			Name:  "JFR_DATASOURCE_URL",
			Value: datasourceURL,
		},
		{
			Name:  "GF_SERVER_ROOT_URL",
			Value: fmt.Sprintf("%s://localhost:%d/grafana/", "http", constants.AuthProxyHttpContainerPort),
		},
	}

	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.GrafanaSecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.GrafanaSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	return corev1.Container{
		Name:            cr.Name + "-grafana",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.GrafanaContainerPort,
			},
		},
		Env: envs,
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: 3000},
					Path:   "/api/health",
					Scheme: corev1.URISchemeHTTP,
				},
			},
			FailureThreshold: 30,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: 3000},
					Path:   "/api/health",
					Scheme: corev1.URISchemeHTTP,
				},
			},
		},
		SecurityContext: containerSc,
		Resources:       *NewGrafanaContainerResource(cr),
	}
}

func NewStorageContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.ObjectStorageResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultStorageCpuRequest, defaultStorageMemoryRequest,
		defaultStorageCpuLimit, defaultStorageMemoryLimit)
	return resources
}

func NewStorageContainer(cr *model.CryostatInstance, imageTag string, tls *TLSConfig) corev1.Container {
	var containerSc *corev1.SecurityContext
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_BUCKETS",
			Value: "archivedrecordings,archivedreports,eventtemplates,probes,heapdumps,threaddumps",
		},
		{
			Name:  "CRYOSTAT_ACCESS_KEY",
			Value: "cryostat",
		},
		{
			Name:  "DATA_DIR",
			Value: "/data",
		},
		{
			Name:  "IP_BIND",
			Value: "0.0.0.0",
		},
		{
			Name:  "REST_ENCRYPTION_ENABLE",
			Value: "1",
		},
	}

	mounts := []corev1.VolumeMount{
		{
			Name:      cr.Name + "-storage",
			MountPath: "/data",
		},
	}

	secretName := cr.Name + "-storage"
	optional := false
	envs = append(envs, corev1.EnvVar{
		Name: "CRYOSTAT_SECRET_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      "SECRET_KEY",
				Optional: &optional,
			},
		},
	})

	livenessProbeScheme := corev1.URISchemeHTTP
	livenessProbePort := constants.StoragePort
	ports := []corev1.ContainerPort{
		{
			ContainerPort: constants.StoragePort,
		},
	}

	args := []string{}
	if tls != nil {
		args = append(args,
			fmt.Sprintf("-s3.port=%d", constants.StoragePort),
			// when TLS key/cert are provided but port number is not, then the HTTP port number is used for HTTPS instead and plain HTTP is disabled
			"-s3.port.https=0",
			fmt.Sprintf("-s3.key.file=%s", path.Join(SecretMountPrefix, tls.StorageSecret, corev1.TLSPrivateKeyKey)),
			fmt.Sprintf("-s3.cert.file=%s", path.Join(SecretMountPrefix, tls.StorageSecret, corev1.TLSCertKey)),
		)

		tlsSecretMount := corev1.VolumeMount{
			Name:      "storage-tls-secret",
			MountPath: path.Join(SecretMountPrefix, tls.StorageSecret),
			ReadOnly:  true,
		}

		mounts = append(mounts, tlsSecretMount)
		livenessProbeScheme = corev1.URISchemeHTTPS
	}

	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.StorageSecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.StorageSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	probeHandler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Port:   intstr.IntOrString{IntVal: livenessProbePort},
			Path:   "/status",
			Scheme: livenessProbeScheme,
		},
	}

	return corev1.Container{
		Name:            cr.Name + "-storage",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		VolumeMounts:    mounts,
		SecurityContext: containerSc,
		Env:             envs,
		Args:            args,
		Ports:           ports,
		LivenessProbe: &corev1.Probe{
			ProbeHandler:     probeHandler,
			FailureThreshold: 2,
		},
		StartupProbe: &corev1.Probe{
			ProbeHandler:     probeHandler,
			FailureThreshold: 13,
		},
		Resources: *NewStorageContainerResource(cr),
	}
}

func NewDatabaseContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.DatabaseResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultDatabaseCpuRequest, defaultDatabaseMemoryRequest,
		defaultDatabaseCpuLimit, defaultDatabaseMemoryLimit)
	return resources
}

func NewDatabaseContainer(cr *model.CryostatInstance, imageTag string, tls *TLSConfig) corev1.Container {
	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.DatabaseSecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.DatabaseSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	mountPath := "/var/lib/pgsql"
	dataDir := path.Join(mountPath, "data")
	envs := []corev1.EnvVar{
		{
			Name:  "PGDATA",
			Value: dataDir,
		},
		{
			Name:  "POSTGRESQL_USER",
			Value: "cryostat",
		},
		{
			Name:  "POSTGRESQL_DATABASE",
			Value: "cryostat",
		},
	}

	optional := false
	secretName := getDatabaseSecret(cr)
	envs = append(envs, corev1.EnvVar{
		Name: "POSTGRESQL_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      constants.DatabaseSecretConnectionKey,
				Optional: &optional,
			},
		},
	})

	envs = append(envs, corev1.EnvVar{
		Name: "PG_ENCRYPT_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      constants.DatabaseSecretEncryptionKey,
				Optional: &optional,
			},
		},
	})

	mounts := []corev1.VolumeMount{
		{
			Name:      cr.Name + "-database",
			MountPath: mountPath,
		},
	}

	args := []string{}

	if tls != nil {
		tlsPath := path.Join(SecretMountPrefix, tls.DatabaseSecret)
		tlsSecretMount := corev1.VolumeMount{
			Name:      "database-tls-secret",
			MountPath: tlsPath,
			ReadOnly:  true,
		}
		mounts = append(mounts, tlsSecretMount)

		args = append(args,
			"-c", "ssl=on",
			"-c", fmt.Sprintf("ssl_cert_file=%s", path.Join(tlsPath, corev1.TLSCertKey)),
			"-c", fmt.Sprintf("ssl_key_file=%s", path.Join(tlsPath, corev1.TLSPrivateKeyKey)),
		)
	}

	return corev1.Container{
		Name:            cr.Name + "-db",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		VolumeMounts:    mounts,
		SecurityContext: containerSc,
		Env:             envs,
		Args:            args,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.DatabasePort,
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"pg_isready", "-U", "cryostat", "-d", "cryostat"},
				},
			},
		},
		Resources: *NewDatabaseContainerResource(cr),
	}
}

// datasourceURL contains the fixed URL to jfr-datasource's web server
var datasourceURL = "http://" + constants.LoopbackAddress + ":" + strconv.Itoa(int(constants.DatasourceContainerPort))

func NewJfrDatasourceContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.DataSourceResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultJfrDatasourceCpuRequest, defaultJfrDatasourceMemoryRequest,
		defaultJfrDatasourceCpuLimit, defaultJfrDatasourceMemoryLimit)
	return resources
}

func NewJfrDatasourceContainer(cr *model.CryostatInstance, imageTag string, serviceSpecs *ServiceSpecs, tls *TLSConfig) corev1.Container {
	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.DataSourceSecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.DataSourceSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	envs := []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: constants.LoopbackAddress,
		},
		{
			Name:  "QUARKUS_HTTP_PORT",
			Value: strconv.Itoa(int(constants.DatasourceContainerPort)),
		},
	}

	mounts := []corev1.VolumeMount{}
	if tls != nil {
		tlsPath := path.Join(SecretMountPrefix, tls.StorageSecret)
		tlsSecretMount := corev1.VolumeMount{
			Name:      "storage-tls-secret",
			MountPath: tlsPath,
			ReadOnly:  true,
		}
		mounts = append(mounts, tlsSecretMount)

		// if we are deploying our own managed storage container with a TLS cert that we issued for it,
		// configure that here. Otherwise if we are configured to talk to an external object storage
		// provider, assume that it is using a well-known certificate signed by a root trust.
		// TODO allow additional configuration via the CR to configure TLS for external providers
		if DeployManagedStorage(cr) {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "CRYOSTAT_STORAGE_TLS_CA_PATH",
					Value: path.Join(SecretMountPrefix, tls.StorageSecret, "s3", "ca.crt"),
				},
				corev1.EnvVar{
					Name:  "CRYOSTAT_STORAGE_TLS_CERT_PATH",
					Value: path.Join(SecretMountPrefix, tls.StorageSecret, "s3", "tls.crt"),
				},
			)
		}
	}

	return corev1.Container{
		Name:            cr.Name + "-jfr-datasource",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.DatasourceContainerPort,
			},
		},
		Env:          envs,
		VolumeMounts: mounts,
		// Can't use HTTP probe since the port is not exposed over the network
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"curl", "--fail", datasourceURL},
				},
			},
		},
		SecurityContext: containerSc,
		Resources:       *NewJfrDatasourceContainerResource(cr),
	}
}

func newAgentProxyContainer(cr *model.CryostatInstance, imageTag string, tls *TLSConfig) corev1.Container {
	var securityContext *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.AgentProxySecurityContext != nil {
		securityContext = cr.Spec.SecurityOptions.AgentProxySecurityContext
	} else {
		privEscalation := false
		securityContext = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	// Mount the config map containing the nginx.conf (and DH params if TLS is enabled)
	mounts := []corev1.VolumeMount{
		{
			Name:      "agent-proxy-config",
			MountPath: constants.AgentProxyConfigFilePath,
			ReadOnly:  true,
		},
	}
	if tls != nil {
		// Mount the TLS secret for the agent proxy
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "agent-proxy-tls-secret",
			MountPath: path.Join(SecretMountPrefix, tls.AgentProxySecret),
			ReadOnly:  true,
		})
	}

	return corev1.Container{
		Name:            cr.Name + "-agent-proxy",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.AgentProxyContainerPort,
			},
			{
				ContainerPort: constants.AgentProxyHealthPort,
			},
		},
		// Override the command to run nginx pointed at our config file. See:
		// https://github.com/sclorg/nginx-container/blob/e7d8db9bc5299a4c4e254f8a82e917c7c136468b/1.24/README.md#direct-usage-with-a-mounted-directory
		Command: []string{
			"nginx",
			"-c", path.Join(constants.AgentProxyConfigFilePath, constants.AgentProxyConfigFileName),
			"-g", "daemon off;",
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/healthz",
					Port:   intstr.FromInt32(constants.AgentProxyHealthPort),
					Scheme: corev1.URISchemeHTTP,
				},
			},
		},
		SecurityContext: securityContext,
		Resources:       *newAgentProxyContainerResource(cr),
		VolumeMounts:    mounts,
	}
}

func newAgentProxyContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.AgentProxyResources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, defaultAgentProxyCpuRequest, defaultAgentProxyMemoryRequest,
		defaultAgentProxyCpuLimit, defaultAgentProxyMemoryLimit)
	return resources
}

func getInternalDashboardURL() string {
	return fmt.Sprintf("http://localhost:%d", constants.GrafanaContainerPort)
}

func newVolumeForDatabase(cr *model.CryostatInstance) []corev1.Volume {
	var volumeSource corev1.VolumeSource

	var emptyDir *operatorv1beta2.EmptyDirConfig
	if cr.Spec.StorageOptions != nil {
		cfg := cr.Spec.StorageOptions.Database
		if cfg != nil {
			emptyDir = cfg.EmptyDir
		} else {
			emptyDir = cr.Spec.StorageOptions.EmptyDir
		}
	}
	if emptyDir != nil && emptyDir.Enabled {
		sizeLimit, err := resource.ParseQuantity(emptyDir.SizeLimit)
		if err != nil {
			sizeLimit = *resource.NewQuantity(0, resource.BinarySI)
		}

		volumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    emptyDir.Medium,
				SizeLimit: &sizeLimit,
			},
		}
	} else {
		volumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: cr.Name + "-database",
			},
		}
	}

	return []corev1.Volume{
		{
			Name:         cr.Name + "-database",
			VolumeSource: volumeSource,
		},
	}
}

func newVolumeForStorage(cr *model.CryostatInstance) []corev1.Volume {
	var volumeSource corev1.VolumeSource

	var emptyDir *operatorv1beta2.EmptyDirConfig
	if cr.Spec.StorageOptions != nil {
		cfg := cr.Spec.StorageOptions.ObjectStorage
		if cfg != nil {
			emptyDir = cfg.EmptyDir
		} else {
			emptyDir = cr.Spec.StorageOptions.EmptyDir
		}
	}
	if emptyDir != nil && emptyDir.Enabled {
		sizeLimit, err := resource.ParseQuantity(emptyDir.SizeLimit)
		if err != nil {
			sizeLimit = *resource.NewQuantity(0, resource.BinarySI)
		}

		volumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    emptyDir.Medium,
				SizeLimit: &sizeLimit,
			},
		}
	} else {
		volumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: cr.Name + "-storage",
			},
		}
	}

	return []corev1.Volume{
		{
			Name:         cr.Name + "-storage",
			VolumeSource: volumeSource,
		},
	}
}

func isBasicAuthEnabled(cr *model.CryostatInstance) bool {
	return cr.Spec.AuthorizationOptions != nil && cr.Spec.AuthorizationOptions.BasicAuth != nil && cr.Spec.AuthorizationOptions.BasicAuth.SecretName != nil && cr.Spec.AuthorizationOptions.BasicAuth.Filename != nil
}

func getDatabaseSecret(cr *model.CryostatInstance) string {
	if cr.Spec.DatabaseOptions != nil && cr.Spec.DatabaseOptions.SecretName != nil {
		return *cr.Spec.DatabaseOptions.SecretName
	}
	return cr.Name + "-db"
}

func getStorageSecret(cr *model.CryostatInstance) string {
	if cr.Spec.ObjectStorageOptions != nil && cr.Spec.ObjectStorageOptions.SecretName != nil {
		return *cr.Spec.ObjectStorageOptions.SecretName
	}
	return cr.Name + "-storage"
}
