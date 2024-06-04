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
	"regexp"
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
	// Name of the secret containing the password for the keystore in CryostatSecret
	KeystorePassSecret string
	// PEM-encoded X.509 certificate for the Cryostat CA
	CACert []byte
}

const (
	defaultAuthProxyCpuRequest        string = "25m"
	defaultAuthProxyMemoryRequest     string = "64Mi"
	defaultCoreCpuRequest             string = "500m"
	defaultCoreMemoryRequest          string = "384Mi"
	defaultJfrDatasourceCpuRequest    string = "200m"
	defaultJfrDatasourceMemoryRequest string = "200Mi"
	defaultGrafanaCpuRequest          string = "25m"
	defaultGrafanaMemoryRequest       string = "80Mi"
	defaultDatabaseCpuRequest         string = "25m"
	defaultDatabaseMemoryRequest      string = "64Mi"
	defaultStorageCpuRequest          string = "50m"
	defaultStorageMemoryRequest       string = "256Mi"
	defaultReportCpuRequest           string = "500m"
	defaultReportMemoryRequest        string = "512Mi"
	OAuth2ConfigFileName              string = "alpha_config.json"
	OAuth2ConfigFilePath              string = "/etc/oauth2_proxy/alpha_config"
)

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
		"app.openshift.io/connects-to": "cryostat-operator-controller-manager",
	}
	defaultPodLabels := map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "cryostat",
	}
	userDefinedDeploymentLabels := make(map[string]string)
	userDefinedDeploymentAnnotations := make(map[string]string)
	userDefinedPodTemplateLabels := make(map[string]string)
	userDefinedPodTemplateAnnotations := make(map[string]string)
	if cr.Spec.OperandMetadata != nil {
		if cr.Spec.OperandMetadata.DeploymentMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Labels {
				userDefinedDeploymentLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Annotations {
				userDefinedDeploymentAnnotations[k] = v
			}
		}
		if cr.Spec.OperandMetadata.PodMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Labels {
				userDefinedPodTemplateLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Annotations {
				userDefinedPodTemplateAnnotations[k] = v
			}
		}
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name,
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedDeploymentLabels,
		Annotations: userDefinedDeploymentAnnotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name,
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedPodTemplateLabels,
		Annotations: userDefinedPodTemplateAnnotations,
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

func NewDeploymentForDatabase(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig,
	openshift bool) *appsv1.Deployment {
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
	defaultPodLabels := map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "database",
	}
	userDefinedDeploymentLabels := make(map[string]string)
	userDefinedDeploymentAnnotations := make(map[string]string)
	userDefinedPodTemplateLabels := make(map[string]string)
	userDefinedPodTemplateAnnotations := make(map[string]string)
	if cr.Spec.OperandMetadata != nil {
		if cr.Spec.OperandMetadata.DeploymentMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Labels {
				userDefinedDeploymentLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Annotations {
				userDefinedDeploymentAnnotations[k] = v
			}
		}
		if cr.Spec.OperandMetadata.PodMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Labels {
				userDefinedPodTemplateLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Annotations {
				userDefinedPodTemplateAnnotations[k] = v
			}
		}
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-database",
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedDeploymentLabels,
		Annotations: userDefinedDeploymentAnnotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-database",
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedPodTemplateLabels,
		Annotations: userDefinedPodTemplateAnnotations,
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
				Spec:       *NewPodForDatabase(cr, imageTags, openshift),
			},
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
	}
}

func NewDeploymentForStorage(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig,
	openshift bool) *appsv1.Deployment {
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
	defaultPodLabels := map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "storage",
	}
	userDefinedDeploymentLabels := make(map[string]string)
	userDefinedDeploymentAnnotations := make(map[string]string)
	userDefinedPodTemplateLabels := make(map[string]string)
	userDefinedPodTemplateAnnotations := make(map[string]string)
	if cr.Spec.OperandMetadata != nil {
		if cr.Spec.OperandMetadata.DeploymentMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Labels {
				userDefinedDeploymentLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Annotations {
				userDefinedDeploymentAnnotations[k] = v
			}
		}
		if cr.Spec.OperandMetadata.PodMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Labels {
				userDefinedPodTemplateLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Annotations {
				userDefinedPodTemplateAnnotations[k] = v
			}
		}
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-storage",
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedDeploymentLabels,
		Annotations: userDefinedDeploymentAnnotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-storage",
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedPodTemplateLabels,
		Annotations: userDefinedPodTemplateAnnotations,
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
				Spec:       *NewPodForStorage(cr, imageTags, tls, openshift),
			},
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
		},
	}
}

func NewDeploymentForReports(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig,
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
	defaultPodLabels := map[string]string{
		"app":       cr.Name,
		"kind":      "cryostat",
		"component": "reports",
	}
	userDefinedDeploymentLabels := make(map[string]string)
	userDefinedDeploymentAnnotations := make(map[string]string)
	userDefinedPodTemplateLabels := make(map[string]string)
	userDefinedPodTemplateAnnotations := make(map[string]string)
	if cr.Spec.OperandMetadata != nil {
		if cr.Spec.OperandMetadata.DeploymentMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Labels {
				userDefinedDeploymentLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.DeploymentMetadata.Annotations {
				userDefinedDeploymentAnnotations[k] = v
			}
		}
		if cr.Spec.OperandMetadata.PodMetadata != nil {
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Labels {
				userDefinedPodTemplateLabels[k] = v
			}
			for k, v := range cr.Spec.OperandMetadata.PodMetadata.Annotations {
				userDefinedPodTemplateAnnotations[k] = v
			}
		}
	}

	// First set the user defined labels and annotation in the meta, so that the default ones can override them
	deploymentMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-reports",
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedDeploymentLabels,
		Annotations: userDefinedDeploymentAnnotations,
	}
	common.MergeLabelsAndAnnotations(&deploymentMeta, defaultDeploymentLabels, defaultDeploymentAnnotations)

	podTemplateMeta := metav1.ObjectMeta{
		Name:        cr.Name + "-reports",
		Namespace:   cr.InstallNamespace,
		Labels:      userDefinedPodTemplateLabels,
		Annotations: userDefinedPodTemplateAnnotations,
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
				Spec:       *NewPodForReports(cr, imageTags, tls, openshift),
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
	containers := []corev1.Container{
		NewCoreContainer(cr, specs, imageTags.CoreImageTag, tls, openshift),
		NewGrafanaContainer(cr, imageTags.GrafanaImageTag, tls),
		NewJfrDatasourceContainer(cr, imageTags.DatasourceImageTag),
		*authProxy,
	}

	volumes := newVolumeForCR(cr)
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
						Path: cr.Name + "-ca.crt",
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
						SecretName: tls.CryostatSecret,
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
		)
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
	volumes = append(volumes, certVolume)

	if !openshift {
		// if not deploying openshift-oauth-proxy then we must be deploying oauth2_proxy instead
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

func NewPodForDatabase(cr *model.CryostatInstance, imageTags *ImageTags, openshift bool) *corev1.PodSpec {
	container := []corev1.Container{NewDatabaseContainer(cr, imageTags.DatabaseImageTag)}

	var podSc *corev1.PodSecurityContext
	if cr.Spec.DatabaseOptions != nil && cr.Spec.DatabaseOptions.SecurityOptions != nil && cr.Spec.DatabaseOptions.SecurityOptions.PodSecurityContext != nil {
		podSc = cr.Spec.DatabaseOptions.SecurityOptions.PodSecurityContext
	} else {
		nonRoot := true
		podSc = &corev1.PodSecurityContext{
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: common.SeccompProfile(openshift),
		}
	}

	var nodeSelector map[string]string
	var affinity *corev1.Affinity
	var tolerations []corev1.Toleration

	if cr.Spec.DatabaseOptions != nil {
		schedulingOptions := cr.Spec.SchedulingOptions
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

	return &corev1.PodSpec{
		Containers:      container,
		NodeSelector:    nodeSelector,
		Affinity:        affinity,
		Tolerations:     tolerations,
		SecurityContext: podSc,
	}
}

func NewPodForStorage(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig, openshift bool) *corev1.PodSpec {
	container := []corev1.Container{NewStorageContainer(cr, imageTags.StorageImageTag)}

	var podSc *corev1.PodSecurityContext
	if cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.SecurityOptions != nil && cr.Spec.StorageOptions.SecurityOptions.StorageSecurityContext != nil {
		podSc = cr.Spec.StorageOptions.SecurityOptions.PodSecurityContext
	} else {
		nonRoot := true
		podSc = &corev1.PodSecurityContext{
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
	}
}

func NewReportContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.ReportOptions != nil {
		resources = cr.Spec.ReportOptions.Resources.DeepCopy()
	}
	populateResourceRequest(resources, defaultReportCpuRequest, defaultReportMemoryRequest)
	return resources
}

func NewPodForReports(cr *model.CryostatInstance, imageTags *ImageTags, tls *TLSConfig, openshift bool) *corev1.PodSpec {
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
		{
			Name:  "JAVA_OPTS",
			Value: javaOpts,
		},
	}
	mounts := []corev1.VolumeMount{}
	volumes := []corev1.Volume{}

	// Configure TLS key/cert if enabled
	livenessProbeScheme := corev1.URISchemeHTTP
	if tls != nil {
		tlsEnvs := []corev1.EnvVar{
			{
				Name:  "QUARKUS_HTTP_SSL_PORT",
				Value: strconv.Itoa(int(constants.ReportsContainerPort)),
			},
			{
				Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_KEY_FILES",
				Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.ReportsSecret, corev1.TLSPrivateKeyKey),
			},
			{
				Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_FILES",
				Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.ReportsSecret, corev1.TLSCertKey),
			},
			{
				Name:  "QUARKUS_HTTP_INSECURE_REQUESTS",
				Value: "disabled",
			},
		}

		tlsSecretMount := corev1.VolumeMount{
			Name:      "reports-tls-secret",
			MountPath: "/var/run/secrets/operator.cryostat.io/" + tls.ReportsSecret,
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

	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:            cr.Name + "-reports",
				Image:           imageTags.ReportsImageTag,
				ImagePullPolicy: getPullPolicy(imageTags.ReportsImageTag),
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
	populateResourceRequest(resources, defaultAuthProxyCpuRequest, defaultAuthProxyMemoryRequest)
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
		fmt.Sprintf("--upstream=http://localhost:%d/storage/", constants.StoragePort),
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
		mountPath := "/var/run/secrets/operator.cryostat.io"
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      cr.Name + "-auth-proxy-htpasswd",
			MountPath: mountPath,
			ReadOnly:  true,
		})
		args = append(args, fmt.Sprintf("--htpasswd-file=%s/%s", mountPath, *cr.Spec.AuthorizationOptions.BasicAuth.Filename))
	}
	args = append(args,
		fmt.Sprintf("--skip-provider-button=%t", !isBasicAuthEnabled(cr)),
	)

	livenessProbeScheme := corev1.URISchemeHTTP
	if tls != nil {
		args = append(args,
			fmt.Sprintf("--http-address="),
			fmt.Sprintf("--https-address=0.0.0.0:%d", constants.AuthProxyHttpContainerPort),
			fmt.Sprintf("--tls-cert=/var/run/secrets/operator.cryostat.io/%s/%s", tls.CryostatSecret, corev1.TLSCertKey),
			fmt.Sprintf("--tls-key=/var/run/secrets/operator.cryostat.io/%s/%s", tls.CryostatSecret, corev1.TLSPrivateKeyKey),
		)

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "auth-proxy-tls-secret",
			MountPath: "/var/run/secrets/operator.cryostat.io/" + tls.CryostatSecret,
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
		ImagePullPolicy: getPullPolicy(imageTag),
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
			MountPath: "/var/run/secrets/operator.cryostat.io/" + tls.CryostatSecret,
			ReadOnly:  true,
		})

		livenessProbeScheme = corev1.URISchemeHTTPS
	}

	if isBasicAuthEnabled(cr) {
		mountPath := "/var/run/secrets/operator.cryostat.io"
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      cr.Name + "-auth-proxy-htpasswd",
			MountPath: mountPath,
			ReadOnly:  true,
		})
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "OAUTH2_PROXY_HTPASSWD_FILE",
				Value: mountPath + "/" + *cr.Spec.AuthorizationOptions.BasicAuth.Filename,
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
		ImagePullPolicy: getPullPolicy(imageTag),
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
			fmt.Sprintf("--alpha-config=%s/%s", OAuth2ConfigFilePath, OAuth2ConfigFileName),
		},
	}, nil
}

func NewCoreContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.CoreResources.DeepCopy()
	}
	populateResourceRequest(resources, defaultCoreCpuRequest, defaultCoreMemoryRequest)
	return resources
}

func NewCoreContainer(cr *model.CryostatInstance, specs *ServiceSpecs, imageTag string,
	tls *TLSConfig, openshift bool) corev1.Container {
	configPath := "/opt/cryostat.d/conf.d"
	templatesPath := "/opt/cryostat.d/templates.d"

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
			Value: "drop-and-create",
		},
		{
			Name:  "QUARKUS_DATASOURCE_USERNAME",
			Value: "cryostat",
		},
		{
			Name:  "QUARKUS_DATASOURCE_JDBC_URL",
			Value: "jdbc:postgresql://localhost:5432/cryostat",
		},
		{
			Name:  "STORAGE_BUCKETS_ARCHIVE_NAME",
			Value: "archivedrecordings",
		},
		{
			Name:  "QUARKUS_S3_ENDPOINT_OVERRIDE",
			Value: "http://localhost:8333",
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
			Name:  "QUARKUS_S3_AWS_CREDENTIALS_TYPE",
			Value: "static",
		},
		{
			Name:  "QUARKUS_S3_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID",
			Value: "cryostat",
		},
		{
			Name:  "AWS_ACCESS_KEY_ID",
			Value: "$(QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID)",
		},
		{
			Name:  "CRYOSTAT_CONFIG_PATH",
			Value: configPath,
		},
		{
			Name:  "CRYOSTAT_TEMPLATE_PATH",
			Value: templatesPath,
		},
	}

	mounts := []corev1.VolumeMount{
		{
			Name:      cr.Name,
			MountPath: configPath,
			SubPath:   "config",
		},
		{
			Name:      cr.Name,
			MountPath: templatesPath,
			SubPath:   "templates",
		},
		{
			Name:      cr.Name,
			MountPath: "truststore",
			SubPath:   "truststore",
		},
		{
			// Mount the CA cert and user certificates in the expected /truststore location
			Name:      "cert-secrets",
			MountPath: "/truststore/operator",
			ReadOnly:  true,
		},
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

	envs = append(envs, corev1.EnvVar{
		Name:  "QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID",
		Value: "cryostat",
	})

	secretName = cr.Name + "-storage"
	envs = append(envs, corev1.EnvVar{
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
	})

	envs = append(envs, corev1.EnvVar{
		Name:  "AWS_SECRET_ACCESS_KEY",
		Value: "$(QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_SECRET_ACCESS_KEY)",
	})

	if specs.ReportsURL != nil {
		reportsEnvs := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_SERVICES_REPORTS_URL",
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

	// Mount the templates specified in Cryostat CR under /opt/cryostat.d/templates.d
	for _, template := range cr.Spec.EventTemplates {
		mount := corev1.VolumeMount{
			Name:      "template-" + template.ConfigMapName,
			MountPath: fmt.Sprintf("%s/%s_%s", templatesPath, template.ConfigMapName, template.Filename),
			SubPath:   template.Filename,
			ReadOnly:  true,
		}
		mounts = append(mounts, mount)
	}

	probeHandler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Port:   intstr.IntOrString{IntVal: constants.CryostatHTTPContainerPort},
			Path:   "/health/liveness",
			Scheme: corev1.URISchemeHTTP,
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

	return corev1.Container{
		Name:            cr.Name,
		Image:           imageTag,
		ImagePullPolicy: getPullPolicy(imageTag),
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
	}
}

func NewGrafanaContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.Resources != nil {
		resources = cr.Spec.Resources.GrafanaResources.DeepCopy()
	}
	populateResourceRequest(resources, defaultGrafanaCpuRequest, defaultGrafanaMemoryRequest)
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
		ImagePullPolicy: getPullPolicy(imageTag),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.GrafanaContainerPort,
			},
		},
		Env: envs,
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
	populateResourceRequest(resources, defaultStorageCpuRequest, defaultStorageMemoryRequest)
	return resources
}

func NewStorageContainer(cr *model.CryostatInstance, imageTag string) corev1.Container {
	var containerSc *corev1.SecurityContext
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_BUCKETS",
			Value: "archivedrecordings,archivedreports,eventtemplates,probes",
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
	}

	mounts := []corev1.VolumeMount{
		{
			Name:      cr.Name,
			MountPath: "/data",
			SubPath:   "seaweed",
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

	if cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.SecurityOptions != nil && cr.Spec.StorageOptions.SecurityOptions.StorageSecurityContext != nil {
		containerSc = cr.Spec.StorageOptions.SecurityOptions.StorageSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{constants.CapabilityAll},
			},
		}
	}

	livenessProbeScheme := corev1.URISchemeHTTP
	probeHandler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Port:   intstr.IntOrString{IntVal: 8333},
			Path:   "/status",
			Scheme: livenessProbeScheme,
		},
	}

	return corev1.Container{
		Name:            cr.Name + "-storage",
		Image:           imageTag,
		ImagePullPolicy: getPullPolicy(imageTag),
		VolumeMounts:    mounts,
		SecurityContext: containerSc,
		Env:             envs,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.StoragePort,
			},
		},
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
	populateResourceRequest(resources, defaultDatabaseCpuRequest, defaultDatabaseMemoryRequest)
	return resources
}

func NewDatabaseContainer(cr *model.CryostatInstance, imageTag string) corev1.Container {
	var containerSc *corev1.SecurityContext
	if cr.Spec.DatabaseOptions != nil && cr.Spec.DatabaseOptions.SecurityOptions != nil && cr.Spec.DatabaseOptions.SecurityOptions.DatabaseSecurityContext != nil {
		containerSc = cr.Spec.DatabaseOptions.SecurityOptions.DatabaseSecurityContext
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
			Name:      cr.Name,
			MountPath: "/data",
			SubPath:   "postgres",
		},
	}

	return corev1.Container{
		Name:            cr.Name + "-db",
		Image:           imageTag,
		ImagePullPolicy: getPullPolicy(imageTag),
		VolumeMounts:    mounts,
		SecurityContext: containerSc,
		Env:             envs,
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
	populateResourceRequest(resources, defaultJfrDatasourceCpuRequest, defaultJfrDatasourceMemoryRequest)
	return resources
}

func NewJfrDatasourceContainer(cr *model.CryostatInstance, imageTag string) corev1.Container {
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

	return corev1.Container{
		Name:            cr.Name + "-jfr-datasource",
		Image:           imageTag,
		ImagePullPolicy: getPullPolicy(imageTag),
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.DatasourceContainerPort,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "QUARKUS_HTTP_HOST",
				Value: constants.LoopbackAddress,
			},
			{
				Name:  "QUARKUS_HTTP_PORT",
				Value: strconv.Itoa(int(constants.DatasourceContainerPort)),
			},
		},
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

func getPort(url *url.URL) string {
	// Return port if already defined in URL
	port := url.Port()
	if len(port) > 0 {
		return port
	}
	// Otherwise use default HTTP(S) ports
	if url.Scheme == "https" {
		return "443"
	}
	return "80"
}

func getInternalDashboardURL() string {
	return fmt.Sprintf("http://localhost:%d", constants.GrafanaContainerPort)
}

// Matches image tags of the form "major.minor.patch"
var develVerRegexp = regexp.MustCompile(`(?i)(:latest|SNAPSHOT|dev|BETA\d+)$`)

func getPullPolicy(imageTag string) corev1.PullPolicy {
	// Use Always for tags that have a known development suffix
	if develVerRegexp.MatchString(imageTag) {
		return corev1.PullAlways
	}
	// Likely a release, use IfNotPresent
	return corev1.PullIfNotPresent
}

func newVolumeForCR(cr *model.CryostatInstance) []corev1.Volume {
	var volumeSource corev1.VolumeSource
	if useEmptyDir(cr) {
		emptyDir := cr.Spec.StorageOptions.EmptyDir

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
				ClaimName: cr.Name,
			},
		}
	}

	return []corev1.Volume{
		{
			Name:         cr.Name,
			VolumeSource: volumeSource,
		},
	}
}

func useEmptyDir(cr *model.CryostatInstance) bool {
	return cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.EmptyDir != nil && cr.Spec.StorageOptions.EmptyDir.Enabled
}

func isBasicAuthEnabled(cr *model.CryostatInstance) bool {
	return cr.Spec.AuthorizationOptions != nil && cr.Spec.AuthorizationOptions.BasicAuth != nil && cr.Spec.AuthorizationOptions.BasicAuth.SecretName != nil && cr.Spec.AuthorizationOptions.BasicAuth.Filename != nil
}

func checkResourceRequestWithLimit(requests, limits corev1.ResourceList) {
	if limits != nil {
		if limitCpu, found := limits[corev1.ResourceCPU]; found && limitCpu.Cmp(*requests.Cpu()) < 0 {
			requests[corev1.ResourceCPU] = limitCpu.DeepCopy()
		}
		if limitMemory, found := limits[corev1.ResourceMemory]; found && limitMemory.Cmp(*requests.Memory()) < 0 {
			requests[corev1.ResourceMemory] = limitMemory.DeepCopy()
		}
	}
}

func populateResourceRequest(resources *corev1.ResourceRequirements, defaultCpu, defaultMemory string) {
	if resources.Requests == nil {
		resources.Requests = corev1.ResourceList{}
	}
	requests := resources.Requests
	if _, found := requests[corev1.ResourceCPU]; !found {
		requests[corev1.ResourceCPU] = resource.MustParse(defaultCpu)
	}
	if _, found := requests[corev1.ResourceMemory]; !found {
		requests[corev1.ResourceMemory] = resource.MustParse(defaultMemory)
	}
	checkResourceRequestWithLimit(requests, resources.Limits)
}

func getDatabaseSecret(cr *model.CryostatInstance) string {
	if cr.Spec.DatabaseOptions != nil && cr.Spec.DatabaseOptions.SecretName != nil {
		return *cr.Spec.DatabaseOptions.SecretName
	}
	return cr.Name + "-db"
}
