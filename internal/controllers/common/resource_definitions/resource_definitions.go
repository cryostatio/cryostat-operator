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
	"crypto/sha256"
	"fmt"
	"math/rand"
	"net/url"
	"time"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	consolev1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Generates image tag constants
//go:generate go run ../../../tools/imagetag_generator.go

// ImageTags contains container image tags for each of the images to deploy
type ImageTags struct {
	CoreImageTag       string
	DatasourceImageTag string
	GrafanaImageTag    string
}

type ServiceSpecs struct {
	CoreURL    *url.URL
	GrafanaURL *url.URL
}

// TLSConfig contains TLS-related information useful when creating other objects
type TLSConfig struct {
	// Name of the TLS secret for Cryostat
	CryostatSecret string
	// Name of the TLS secret for Grafana
	GrafanaSecret string
	// Name of the secret containing the password for the keystore in CryostatSecret
	KeystorePassSecret string
}

func NewPersistentVolumeClaimForCR(cr *operatorv1beta1.Cryostat) *corev1.PersistentVolumeClaim {
	objMeta := metav1.ObjectMeta{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}
	// Check for PVC config within CR
	var pvcSpec corev1.PersistentVolumeClaimSpec
	if cr.Spec.StorageOptions != nil && cr.Spec.StorageOptions.PVC != nil {
		config := cr.Spec.StorageOptions.PVC
		// Import any annotations and labels from the PVC config
		objMeta.Annotations = config.Annotations
		objMeta.Labels = config.Labels
		// Use provided spec if specified
		if config.Spec != nil {
			pvcSpec = *config.Spec
		}
	}

	// Add "app" label. This will override any user-specified "app" label.
	if objMeta.Labels == nil {
		objMeta.Labels = map[string]string{}
	}
	objMeta.Labels["app"] = cr.Name

	// Apply any applicable spec defaults. Don't apply a default storage class name, since nil
	// may be intentionally specified.
	if pvcSpec.Resources.Requests == nil {
		pvcSpec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: *resource.NewQuantity(500*1024*1024, resource.BinarySI),
		}
	}
	if pvcSpec.AccessModes == nil {
		pvcSpec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: objMeta,
		Spec:       pvcSpec,
	}
}

func NewDeploymentForCR(cr *operatorv1beta1.Cryostat, specs *ServiceSpecs, imageTags *ImageTags,
	tls *TLSConfig, fsGroup int64) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":                    cr.Name,
				"kind":                   "cryostat",
				"app.kubernetes.io/name": "cryostat",
			},
			Annotations: map[string]string{
				"app.openshift.io/connects-to": "cryostat-operator",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  cr.Name,
					"kind": "cryostat",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name,
					Namespace: cr.Namespace,
					Labels: map[string]string{
						"app":  cr.Name,
						"kind": "cryostat",
					},
				},
				Spec: *NewPodForCR(cr, specs, imageTags, tls, fsGroup),
			},
		},
	}
}

func NewPodForCR(cr *operatorv1beta1.Cryostat, specs *ServiceSpecs, imageTags *ImageTags,
	tls *TLSConfig, fsGroup int64) *corev1.PodSpec {
	var containers []corev1.Container
	if cr.Spec.Minimal {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, imageTags.CoreImageTag, tls),
		}
	} else {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, imageTags.CoreImageTag, tls),
			NewGrafanaContainer(cr, imageTags.GrafanaImageTag, tls),
			NewJfrDatasourceContainer(cr, imageTags.DatasourceImageTag),
		}
	}
	volumes := []corev1.Volume{
		{
			Name: cr.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cr.Name,
				},
			},
		},
	}
	readOnlyMode := int32(0440)
	if tls != nil {
		// Create certificate secret volumes in deployment
		volSources := []corev1.VolumeProjection{
			{
				// Add Cryostat self-signed CA
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: tls.CryostatSecret,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  CAKey,
							Path: cr.Name + "-ca.crt",
							Mode: &readOnlyMode,
						},
					},
				},
			},
		}

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
		certVolume := corev1.Volume{
			Name: "cert-secrets",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: volSources,
				},
			},
		}
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

		volumes = append(volumes, certVolume, keyVolume)

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
	}

	// Ensure PV mounts are writable
	sc := &corev1.PodSecurityContext{
		FSGroup: &fsGroup,
	}
	return &corev1.PodSpec{
		ServiceAccountName: cr.Name,
		Volumes:            volumes,
		Containers:         containers,
		SecurityContext:    sc,
	}
}

func NewCoreContainer(cr *operatorv1beta1.Cryostat, specs *ServiceSpecs, imageTag string, tls *TLSConfig) corev1.Container {
	configPath := "/opt/cryostat.d/conf.d"
	archivePath := "/opt/cryostat.d/recordings.d"
	templatesPath := "/opt/cryostat.d/templates.d"
	clientlibPath := "/opt/cryostat.d/clientlib.d"
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_SSL_PROXIED",
			Value: "true",
		},
		{
			Name:  "CRYOSTAT_ALLOW_UNTRUSTED_SSL",
			Value: "true",
		},
		{
			Name:  "CRYOSTAT_WEB_PORT",
			Value: "8181",
		},
		{
			Name:  "CRYOSTAT_LISTEN_PORT",
			Value: "9090",
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

	envsFrom := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cr.Name + "-jmx-auth",
				},
			},
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
			MountPath: "truststore",
			SubPath:   "truststore",
		},
	}

	if !cr.Spec.Minimal {
		grafanaVars := []corev1.EnvVar{
			{
				Name:  "GRAFANA_DATASOURCE_URL",
				Value: DatasourceURL,
			},
		}
		if specs.GrafanaURL != nil {
			grafanaVars = append(grafanaVars, corev1.EnvVar{
				Name:  "GRAFANA_DASHBOARD_URL",
				Value: specs.GrafanaURL.String(),
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

		// Mount the CA cert and user certificates in the expected /truststore location
		caCertMount := corev1.VolumeMount{
			Name:      "cert-secrets",
			MountPath: "/truststore/operator",
			ReadOnly:  true,
		}

		mounts = append(mounts, keystoreMount, caCertMount)

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

		// Use HTTPS for liveness probe
		livenessProbeScheme = corev1.URISchemeHTTPS
	}
	probeHandler := corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Port:   intstr.IntOrString{IntVal: 8181},
			Path:   "/api/v1/clienturl",
			Scheme: livenessProbeScheme,
		},
	}
	return corev1.Container{
		Name:         cr.Name,
		Image:        imageTag,
		VolumeMounts: mounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8181,
			},
			{
				ContainerPort: 9090,
			},
			{
				ContainerPort: 9091,
			},
		},
		Env:     envs,
		EnvFrom: envsFrom,
		LivenessProbe: &corev1.Probe{
			Handler: probeHandler,
		},
		// Expect probe to succeed within 3 minutes
		StartupProbe: &corev1.Probe{
			Handler:          probeHandler,
			FailureThreshold: 18,
		},
	}
}

func NewGrafanaSecretForCR(cr *operatorv1beta1.Cryostat) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana-basic",
			Namespace: cr.Namespace,
		},
		StringData: map[string]string{
			"GF_SECURITY_ADMIN_USER":     "admin",
			"GF_SECURITY_ADMIN_PASSWORD": GenPasswd(20),
		},
	}
}

func GenPasswd(length int) string {
	rand.Seed(time.Now().UnixNano())
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func NewGrafanaContainer(cr *operatorv1beta1.Cryostat, imageTag string, tls *TLSConfig) corev1.Container {
	envs := []corev1.EnvVar{
		{
			Name:  "JFR_DATASOURCE_URL",
			Value: DatasourceURL,
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
	return corev1.Container{
		Name:         cr.Name + "-grafana",
		Image:        imageTag,
		VolumeMounts: mounts,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3000,
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
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: 3000},
					Path:   "/api/health",
					Scheme: livenessProbeScheme,
				},
			},
		},
	}
}

const datasourceHost = "127.0.0.1"
const datasourcePort = "8080"

// DatasourceURL contains the fixed URL to jfr-datasource's web server
const DatasourceURL = "http://" + datasourceHost + ":" + datasourcePort

func NewJfrDatasourceContainer(cr *operatorv1beta1.Cryostat, imageTag string) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-jfr-datasource",
		Image: imageTag,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8080,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "LISTEN_HOST",
				Value: datasourceHost,
			},
		},
		// Can't use HTTP probe since the port is not exposed over the network
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{"curl", "--fail", DatasourceURL},
				},
			},
		},
	}
}

func NewExporterService(cr *operatorv1beta1.Cryostat) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "cryostat",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "export",
					Port:       8181,
					TargetPort: intstr.IntOrString{IntVal: 8181},
				},
				{
					Name:       "jfr-jmx",
					Port:       9091,
					TargetPort: intstr.IntOrString{IntVal: 9091},
				},
			},
		},
	}
}

func NewCommandChannelService(cr *operatorv1beta1.Cryostat) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-command",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "command-channel",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "cmdchan",
					Port:       9090,
					TargetPort: intstr.IntOrString{IntVal: 9090},
				},
			},
		},
	}
}

func NewGrafanaService(cr *operatorv1beta1.Cryostat) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "grafana",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "3000-tcp",
					Port:       3000,
					TargetPort: intstr.IntOrString{IntVal: 3000},
				},
			},
		},
	}
}

// JMXSecretNameSuffix is the suffix to be appended to the name of a
// Cryostat CR to name its JMX credentials secret
const JMXSecretNameSuffix = "-jmx-auth"

// JMXSecretUserKey indexes the username within the Cryostat JMX auth secret
const JMXSecretUserKey = "CRYOSTAT_RJMX_USER"

// JMXSecretPassKey indexes the password within the Cryostat JMX auth secret
const JMXSecretPassKey = "CRYOSTAT_RJMX_PASS"

func NewJmxSecretForCR(cr *operatorv1beta1.Cryostat) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + JMXSecretNameSuffix,
			Namespace: cr.Namespace,
		},
		StringData: map[string]string{
			"CRYOSTAT_RJMX_USER": "cryostat",
			"CRYOSTAT_RJMX_PASS": GenPasswd(20),
		},
	}
}

func NewKeystoreSecretForCR(cr *operatorv1beta1.Cryostat) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-keystore",
			Namespace: cr.Namespace,
		},
		StringData: map[string]string{
			"KEYSTORE_PASS": GenPasswd(20),
		},
	}
}

func NewServiceAccountForCR(cr *operatorv1beta1.Cryostat) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}
}

func NewRoleForCR(cr *operatorv1beta1.Cryostat) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "list", "watch"},
				APIGroups: []string{""},
				Resources: []string{"endpoints"},
			},
			{
				Verbs:     []string{"get"},
				APIGroups: []string{""},
				Resources: []string{"pods", "replicationcontrollers"},
			},
			{
				Verbs:     []string{"get"},
				APIGroups: []string{"apps"},
				Resources: []string{"replicasets", "deployments", "daemonsets", "statefulsets"},
			},
			{
				Verbs:     []string{"get"},
				APIGroups: []string{"apps.openshift.io"},
				Resources: []string{"deploymentconfigs"},
			},
			{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{"route.openshift.io"},
				Resources: []string{"routes"},
			},
		},
	}
}

func NewRoleBindingForCR(cr *operatorv1beta1.Cryostat) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     cr.Name,
		},
	}
}

func NewClusterRoleBindingForCR(cr *operatorv1beta1.Cryostat) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterUniqueName(cr),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      cr.Name,
				Namespace: cr.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cryostat-operator-cryostat",
		},
	}
}

func NewConsoleLink(cr *operatorv1beta1.Cryostat, url string) *consolev1.ConsoleLink {
	// Cluster scoped, so use a unique name to avoid conflicts
	return &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterUniqueName(cr),
		},
		Spec: consolev1.ConsoleLinkSpec{
			Link: consolev1.Link{
				Text: "Cryostat",
				Href: url,
			},
			Location: consolev1.NamespaceDashboard,
			NamespaceDashboard: &consolev1.NamespaceDashboardSpec{
				Namespaces: []string{cr.Namespace},
			},
		},
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

func clusterUniqueName(cr *operatorv1beta1.Cryostat) string {
	// Use the SHA256 checksum of the namespaced name as a suffix
	nn := types.NamespacedName{Namespace: cr.Namespace, Name: cr.Name}
	suffix := fmt.Sprintf("%x", sha256.Sum256([]byte(nn.String())))
	return "cryostat-" + suffix
}
