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

package resource_definitions

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ImageTags contains container image tags for each of the images to deploy
type ImageTags struct {
	CoreImageTag       string
	DatasourceImageTag string
	GrafanaImageTag    string
	ReportsImageTag    string
}

type ServiceSpecs struct {
	CoreURL    *url.URL
	GrafanaURL *url.URL
	ReportsURL *url.URL
}

// TLSConfig contains TLS-related information useful when creating other objects
type TLSConfig struct {
	// Name of the TLS secret for Cryostat
	CryostatSecret string
	// Name of the TLS secret for Grafana
	GrafanaSecret string
	// Name of the TLS secret for Reports Generator
	ReportsSecret string
	// Name of the secret containing the password for the keystore in CryostatSecret
	KeystorePassSecret string
	// PEM-encoded X.509 certificate for the Cryostat CA
	CACert []byte
}

const (
	defaultCoreCpuRequest             string = "100m"
	defaultCoreMemoryRequest          string = "384Mi"
	defaultJfrDatasourceCpuRequest    string = "100m"
	defaultJfrDatasourceMemoryRequest string = "512Mi"
	defaultGrafanaCpuRequest          string = "100m"
	defaultGrafanaMemoryRequest       string = "256Mi"
	defaultReportCpuRequest           string = "128m"
	defaultReportMemoryRequest        string = "256Mi"
)

func NewDeploymentForCR(cr *model.CryostatInstance, specs *ServiceSpecs, imageTags *ImageTags,
	tls *TLSConfig, fsGroup int64, openshift bool) *appsv1.Deployment {
	// Force one replica to avoid lock file and PVC contention
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
			Labels: map[string]string{
				"app":                    cr.Name,
				"kind":                   "cryostat",
				"component":              "cryostat",
				"app.kubernetes.io/name": "cryostat",
			},
			Annotations: map[string]string{
				"app.openshift.io/connects-to": "cryostat-operator-controller-manager",
			},
		},
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name,
					Namespace: cr.InstallNamespace,
					Labels: map[string]string{
						"app":       cr.Name,
						"kind":      "cryostat",
						"component": "cryostat",
					},
				},
				Spec: *NewPodForCR(cr, specs, imageTags, tls, fsGroup, openshift),
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
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-reports",
			Namespace: cr.InstallNamespace,
			Labels: map[string]string{
				"app":                    cr.Name,
				"kind":                   "cryostat",
				"component":              "reports",
				"app.kubernetes.io/name": "cryostat-reports",
			},
			Annotations: map[string]string{
				"app.openshift.io/connects-to": cr.Name,
			},
		},
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name + "-reports",
					Namespace: cr.InstallNamespace,
					Labels: map[string]string{
						"app":       cr.Name,
						"kind":      "cryostat",
						"component": "reports",
					},
				},
				Spec: *NewPodForReports(cr, imageTags, tls, openshift),
			},
			Replicas: &replicas,
		},
	}
}

func NewPodForCR(cr *model.CryostatInstance, specs *ServiceSpecs, imageTags *ImageTags,
	tls *TLSConfig, fsGroup int64, openshift bool) *corev1.PodSpec {
	var containers []corev1.Container
	if cr.Spec.Minimal {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, imageTags.CoreImageTag, tls, openshift),
		}
	} else {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, imageTags.CoreImageTag, tls, openshift),
			NewGrafanaContainer(cr, imageTags.GrafanaImageTag, tls),
			NewJfrDatasourceContainer(cr, imageTags.DatasourceImageTag),
		}
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
			key = operatorv1beta1.DefaultCertificateKey
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

		keyVolume := corev1.Volume{
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
		}

		volumes = append(volumes, keyVolume)

		if !cr.Spec.Minimal {
			grafanaSecretVolume := corev1.Volume{
				Name: "grafana-tls-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tls.GrafanaSecret,
					},
				},
			}
			volumes = append(volumes, grafanaSecretVolume)
		}
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

	// Add Auth properties as a volume if specified (on Openshift)
	if openshift && cr.Spec.AuthProperties != nil {
		authResourceVolume := corev1.Volume{
			Name: "auth-properties-" + cr.Spec.AuthProperties.ConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Spec.AuthProperties.ConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  cr.Spec.AuthProperties.Filename,
							Path: "OpenShiftAuthManager.properties",
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}
		volumes = append(volumes, authResourceVolume)
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
			SeccompProfile: seccompProfile(openshift),
		}
	}

	// Use HostAlias for loopback address to allow health checks to
	// work over HTTPS with hostname added as a SubjectAltName
	hostAliases := []corev1.HostAlias{
		{
			IP: constants.LoopbackAddress,
			Hostnames: []string{
				constants.HealthCheckHostname,
			},
		},
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
		HostAliases:                  hostAliases,
		AutomountServiceAccountToken: &automountSAToken,
		NodeSelector:                 nodeSelector,
		Affinity:                     affinity,
		Tolerations:                  tolerations,
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

// ALL capability to drop for restricted pod security. See:
// https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
const capabilityAll corev1.Capability = "ALL"

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
				Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_KEY_FILE",
				Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.ReportsSecret, corev1.TLSPrivateKeyKey),
			},
			{
				Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_FILE",
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
			SeccompProfile: seccompProfile(openshift),
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
				Drop: []corev1.Capability{capabilityAll},
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
	archivePath := "/opt/cryostat.d/recordings.d"
	templatesPath := "/opt/cryostat.d/templates.d"
	clientlibPath := "/opt/cryostat.d/clientlib.d"
	probesPath := "/opt/cryostat.d/probes.d"
	authPropertiesPath := "/app/resources/io/cryostat/net/openshift/OpenShiftAuthManager.properties"

	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_WEB_PORT",
			Value: strconv.Itoa(int(constants.CryostatHTTPContainerPort)),
		},
		{
			Name:  "CRYOSTAT_CONFIG_PATH",
			Value: configPath,
		},
		{
			Name:  "CRYOSTAT_ARCHIVE_PATH",
			Value: archivePath,
		},
		{
			Name:  "CRYOSTAT_TEMPLATE_PATH",
			Value: templatesPath,
		},
		{
			Name:  "CRYOSTAT_CLIENTLIB_PATH",
			Value: clientlibPath,
		},
		{
			Name:  "CRYOSTAT_PROBE_TEMPLATE_PATH",
			Value: probesPath,
		},
		{
			Name:  "CRYOSTAT_ENABLE_JDP_BROADCAST",
			Value: "false",
		},
		{
			Name:  "CRYOSTAT_K8S_NAMESPACES",
			Value: strings.Join(cr.TargetNamespaces, ","),
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
			MountPath: archivePath,
			SubPath:   "flightrecordings",
		},
		{
			Name:      cr.Name,
			MountPath: templatesPath,
			SubPath:   "templates",
		},
		{
			Name:      cr.Name,
			MountPath: clientlibPath,
			SubPath:   "clientlib",
		},
		{
			Name:      cr.Name,
			MountPath: probesPath,
			SubPath:   "probes",
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

	if specs.CoreURL != nil {
		coreEnvs := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_EXT_WEB_PORT",
				Value: getPort(specs.CoreURL),
			},
			{
				Name:  "CRYOSTAT_WEB_HOST",
				Value: specs.CoreURL.Hostname(),
			},
		}
		envs = append(envs, coreEnvs...)
	}
	if specs.ReportsURL != nil {
		reportsEnvs := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_REPORT_GENERATOR",
				Value: specs.ReportsURL.String(),
			},
		}
		envs = append(envs, reportsEnvs...)
	} else {
		subProcessMaxHeapSize := "200"
		if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SubProcessMaxHeapSize != 0 {
			subProcessMaxHeapSize = strconv.Itoa(int(cr.Spec.ReportOptions.SubProcessMaxHeapSize))
		}
		subprocessReportHeapEnv := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_REPORT_GENERATION_MAX_HEAP",
				Value: subProcessMaxHeapSize,
			},
		}
		envs = append(envs, subprocessReportHeapEnv...)
	}

	if cr.Spec.MaxWsConnections != 0 {
		maxWsConnections := strconv.Itoa(int(cr.Spec.MaxWsConnections))
		maxWsConnectionsEnv := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_MAX_WS_CONNECTIONS",
				Value: maxWsConnections,
			},
		}
		envs = append(envs, maxWsConnectionsEnv...)
	}

	targetCacheSize := "-1"
	targetCacheTTL := "10"
	if cr.Spec.JmxCacheOptions != nil {

		if cr.Spec.JmxCacheOptions.TargetCacheSize != 0 {
			targetCacheSize = strconv.Itoa(int(cr.Spec.JmxCacheOptions.TargetCacheSize))
		}

		if cr.Spec.JmxCacheOptions.TargetCacheTTL != 0 {
			targetCacheTTL = strconv.Itoa(int(cr.Spec.JmxCacheOptions.TargetCacheTTL))
		}
	}
	jmxCacheEnvs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_TARGET_CACHE_SIZE",
			Value: targetCacheSize,
		},
		{
			Name:  "CRYOSTAT_TARGET_CACHE_TTL",
			Value: targetCacheTTL,
		},
	}
	envs = append(envs, jmxCacheEnvs...)
	envsFrom := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Name + "-jmx-auth",
				},
			},
		},
	}

	if openshift {
		// Force OpenShift platform strategy
		openshiftEnvs := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_PLATFORM",
				Value: "io.cryostat.platform.internal.OpenShiftPlatformStrategy",
			},
			{
				Name:  "CRYOSTAT_AUTH_MANAGER",
				Value: "io.cryostat.net.openshift.OpenShiftAuthManager",
			},
			{
				Name:  "CRYOSTAT_OAUTH_CLIENT_ID",
				Value: cr.Name,
			},
			{
				Name:  "CRYOSTAT_BASE_OAUTH_ROLE",
				Value: constants.OperatorNamePrefix + "oauth-client",
			},
		}
		envs = append(envs, openshiftEnvs...)

		if cr.Spec.AuthProperties != nil {
			// Mount Auth properties if specified (on Openshift)
			mounts = append(mounts, corev1.VolumeMount{
				Name:      "auth-properties-" + cr.Spec.AuthProperties.ConfigMapName,
				MountPath: authPropertiesPath,
				SubPath:   "OpenShiftAuthManager.properties",
				ReadOnly:  true,
			})
			envs = append(envs, corev1.EnvVar{
				Name:  "CRYOSTAT_CUSTOM_OAUTH_ROLE",
				Value: cr.Spec.AuthProperties.ClusterRoleName,
			})
		}
	}

	disableBuiltInDiscovery := cr.Spec.TargetDiscoveryOptions != nil && cr.Spec.TargetDiscoveryOptions.BuiltInDiscoveryDisabled
	if disableBuiltInDiscovery {
		envs = append(envs, corev1.EnvVar{
			Name:  "CRYOSTAT_DISABLE_BUILTIN_DISCOVERY",
			Value: "true",
		})
	}

	if !useEmptyDir(cr) {
		envs = append(envs, corev1.EnvVar{
			Name:  "CRYOSTAT_JDBC_URL",
			Value: "jdbc:h2:file:/opt/cryostat.d/conf.d/h2;INIT=create domain if not exists jsonb as varchar",
		}, corev1.EnvVar{
			Name:  "CRYOSTAT_HBM2DDL",
			Value: "update",
		}, corev1.EnvVar{
			Name:  "CRYOSTAT_JDBC_DRIVER",
			Value: "org.h2.Driver",
		}, corev1.EnvVar{
			Name:  "CRYOSTAT_HIBERNATE_DIALECT",
			Value: "org.hibernate.dialect.H2Dialect",
		}, corev1.EnvVar{
			Name:  "CRYOSTAT_JDBC_USERNAME",
			Value: cr.Name,
		}, corev1.EnvVar{
			Name:  "CRYOSTAT_JDBC_PASSWORD",
			Value: cr.Name,
		})
	}

	secretOptional := false
	secretName := cr.Name + "-jmx-credentials-db"
	if cr.Spec.JmxCredentialsDatabaseOptions != nil && cr.Spec.JmxCredentialsDatabaseOptions.DatabaseSecretName != nil {
		secretName = *cr.Spec.JmxCredentialsDatabaseOptions.DatabaseSecretName
	}
	envs = append(envs, corev1.EnvVar{
		Name: "CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      "CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD",
				Optional: &secretOptional,
			},
		},
	})

	if !cr.Spec.Minimal {
		grafanaVars := []corev1.EnvVar{
			{
				Name:  "GRAFANA_DATASOURCE_URL",
				Value: datasourceURL,
			},
		}
		if specs.GrafanaURL != nil {
			grafanaVars = append(grafanaVars,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_EXT_URL",
					Value: specs.GrafanaURL.String(),
				},
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_URL",
					Value: getInternalDashboardURL(tls),
				})
		}
		envs = append(envs, grafanaVars...)
	}

	livenessProbeScheme := corev1.URISchemeHTTP
	if tls == nil {
		// If TLS isn't set up, tell Cryostat to not use it
		envs = append(envs, corev1.EnvVar{
			Name:  "CRYOSTAT_DISABLE_SSL",
			Value: "true",
		})
		// Set CRYOSTAT_SSL_PROXIED if Ingress/Route use HTTPS
		if specs.CoreURL != nil && specs.CoreURL.Scheme == "https" {
			envs = append(envs, corev1.EnvVar{
				Name:  "CRYOSTAT_SSL_PROXIED",
				Value: "true",
			})
		}
	} else {
		// Configure keystore location and password in expected environment variables
		envs = append(envs, corev1.EnvVar{
			Name:  "KEYSTORE_PATH",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/keystore.p12", tls.CryostatSecret),
		})
		envsFrom = append(envsFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: tls.KeystorePassSecret,
				},
			},
		})

		// Mount the TLS secret's keystore
		keystoreMount := corev1.VolumeMount{
			Name:      "keystore",
			MountPath: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s", tls.CryostatSecret),
			ReadOnly:  true,
		}

		mounts = append(mounts, keystoreMount)

		// Use HTTPS for liveness probe
		livenessProbeScheme = corev1.URISchemeHTTPS
	}

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
			Scheme: livenessProbeScheme,
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
				Drop: []corev1.Capability{capabilityAll},
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
			{
				ContainerPort: constants.CryostatJMXContainerPort,
			},
		},
		Env:       envs,
		EnvFrom:   envsFrom,
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
			Name:  "JFR_DATASOURCE_URL",
			Value: datasourceURL,
		},
	}
	mounts := []corev1.VolumeMount{}

	// Configure TLS key/cert if enabled
	livenessProbeScheme := corev1.URISchemeHTTP
	if tls != nil {
		tlsEnvs := []corev1.EnvVar{
			{
				Name:  "GF_SERVER_PROTOCOL",
				Value: "https",
			},
			{
				Name:  "GF_SERVER_CERT_KEY",
				Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.GrafanaSecret, corev1.TLSPrivateKeyKey),
			},
			{
				Name:  "GF_SERVER_CERT_FILE",
				Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.GrafanaSecret, corev1.TLSCertKey),
			},
		}

		tlsSecretMount := corev1.VolumeMount{
			Name:      "grafana-tls-secret",
			MountPath: "/var/run/secrets/operator.cryostat.io/" + tls.GrafanaSecret,
			ReadOnly:  true,
		}

		envs = append(envs, tlsEnvs...)
		mounts = append(mounts, tlsSecretMount)

		// Use HTTPS for liveness probe
		livenessProbeScheme = corev1.URISchemeHTTPS
	}

	var containerSc *corev1.SecurityContext
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.GrafanaSecurityContext != nil {
		containerSc = cr.Spec.SecurityOptions.GrafanaSecurityContext
	} else {
		privEscalation := false
		containerSc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{capabilityAll},
			},
		}
	}

	return corev1.Container{
		Name:            cr.Name + "-grafana",
		Image:           imageTag,
		ImagePullPolicy: getPullPolicy(imageTag),
		VolumeMounts:    mounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.GrafanaContainerPort,
			},
		},
		Env: envs,
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Name + "-grafana-basic",
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: 3000},
					Path:   "/api/health",
					Scheme: livenessProbeScheme,
				},
			},
		},
		SecurityContext: containerSc,
		Resources:       *NewGrafanaContainerResource(cr),
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
				Drop: []corev1.Capability{capabilityAll},
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
				Name:  "LISTEN_HOST",
				Value: constants.LoopbackAddress,
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

func getInternalDashboardURL(tls *TLSConfig) string {
	scheme := "https"
	if tls == nil {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, constants.HealthCheckHostname, constants.GrafanaContainerPort)
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

func seccompProfile(openshift bool) *corev1.SeccompProfile {
	// For backward-compatibility with OpenShift < 4.11,
	// leave the seccompProfile empty. In OpenShift >= 4.11,
	// the restricted-v2 SCC will populate it for us.
	if openshift {
		return nil
	}
	return &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}
}

func useEmptyDir(cr *model.CryostatInstance) bool {
	return cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.EmptyDir != nil && cr.Spec.StorageOptions.EmptyDir.Enabled

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
