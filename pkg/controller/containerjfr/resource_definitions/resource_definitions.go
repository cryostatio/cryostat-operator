// Copyright (c) 2020 Red Hat, Inc.
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
	"math/rand"
	"time"

	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServiceSpecs struct {
	CoreHostname    string
	CommandHostname string
	GrafanaURL      string
}

// TLSConfig contains TLS-related information useful when creating other objects
type TLSConfig struct {
	// Name of the TLS secret for Container JFR
	ContainerJFRSecret string
	// Name of the TLS secret for Grafana
	GrafanaSecret string
	// Name of the secret containing the password for the keystore in ContainerJFRSecret
	KeystorePassSecret string
}

func NewPersistentVolumeClaimForCR(cr *rhjmcv1beta1.ContainerJFR) *corev1.PersistentVolumeClaim {
	storageClassName := ""
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": *resource.NewQuantity(500*1024*1024, resource.BinarySI),
				},
			},
		},
	}
}

func NewDeploymentForCR(cr *rhjmcv1beta1.ContainerJFR, specs *ServiceSpecs, tls *TLSConfig) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":                    cr.Name,
				"kind":                   "containerjfr",
				"app.kubernetes.io/name": "container-jfr",
			},
			Annotations: map[string]string{
				"redhat.com/containerJfrUrl":   specs.CoreHostname,
				"app.openshift.io/connects-to": "container-jfr-operator",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  cr.Name,
					"kind": "containerjfr",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name,
					Namespace: cr.Namespace,
					Labels: map[string]string{
						"app":  cr.Name,
						"kind": "containerjfr",
					},
					Annotations: map[string]string{
						"redhat.com/containerJfrUrl": specs.CoreHostname,
					},
				},
				Spec: *NewPodForCR(cr, specs, tls),
			},
		},
	}
}

func NewPodForCR(cr *rhjmcv1beta1.ContainerJFR, specs *ServiceSpecs, tls *TLSConfig) *corev1.PodSpec {
	var containers []corev1.Container
	if cr.Spec.Minimal {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, tls),
		}
	} else {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, tls),
			NewGrafanaContainer(cr, tls),
			NewJfrDatasourceContainer(cr),
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
	if tls != nil {
		// Create certificate secret volumes in deployment
		secretVolume := corev1.Volume{
			Name: "tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tls.ContainerJFRSecret,
				},
			},
		}
		grafanaSecretVolume := corev1.Volume{
			Name: "grafana-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tls.GrafanaSecret,
				},
			},
		}
		volumes = append(volumes, secretVolume, grafanaSecretVolume)

		customVolumes := []corev1.Volume{}
		for _, secret := range cr.Spec.TrustedCertSecrets {
			volume := corev1.Volume{
				Name: secret.SecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secret.SecretName,
					},
				},
			}
			customVolumes = append(customVolumes, volume)
		}
		volumes = append(volumes, customVolumes...)
	}
	return &corev1.PodSpec{
		ServiceAccountName: "container-jfr-operator",
		Volumes:            volumes,
		Containers:         containers,
	}
}

func NewCoreContainer(cr *rhjmcv1beta1.ContainerJFR, specs *ServiceSpecs, tls *TLSConfig) corev1.Container {
	envs := []corev1.EnvVar{
		{
			Name:  "CONTAINER_JFR_PLATFORM",
			Value: "com.redhat.rhjmc.containerjfr.platform.openshift.OpenShiftPlatformStrategy",
		},
		{
			Name:  "CONTAINER_JFR_SSL_PROXIED",
			Value: "true",
		},
		{
			Name:  "CONTAINER_JFR_ALLOW_UNTRUSTED_SSL",
			Value: "true",
		},
		{
			Name:  "CONTAINER_JFR_WEB_PORT",
			Value: "8181",
		},
		{
			Name:  "CONTAINER_JFR_EXT_WEB_PORT",
			Value: "443",
		},
		{
			Name:  "CONTAINER_JFR_WEB_HOST",
			Value: specs.CoreHostname,
		},
		{
			Name:  "CONTAINER_JFR_LISTEN_PORT",
			Value: "9090",
		},
		{
			Name:  "CONTAINER_JFR_EXT_LISTEN_PORT",
			Value: "443",
		},
		{
			Name:  "CONTAINER_JFR_LISTEN_HOST",
			Value: specs.CommandHostname,
		},
		{
			Name:  "GRAFANA_DASHBOARD_URL",
			Value: specs.GrafanaURL,
		},
		{
			Name:  "GRAFANA_DATASOURCE_URL",
			Value: DatasourceURL,
		},
		{
			Name:  "CONTAINER_JFR_TEMPLATE_PATH",
			Value: "/templates",
		},
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
			MountPath: "flightrecordings",
			SubPath:   "flightrecordings",
		},
		{
			Name:      cr.Name,
			MountPath: "templates",
			SubPath:   "templates",
		},
	}

	livenessProbeScheme := corev1.URISchemeHTTP
	if tls == nil {
		// If TLS isn't set up, tell Container JFR to not use it
		envs = append(envs, corev1.EnvVar{
			Name:  "CONTAINER_JFR_DISABLE_SSL",
			Value: "true",
		})
	} else {
		// Configure keystore location and password in expected environment variables
		envs = append(envs, corev1.EnvVar{
			Name:  "KEYSTORE_PATH",
			Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s/keystore.p12", tls.ContainerJFRSecret),
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
			Name:      "tls-secret",
			MountPath: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s/keystore.p12", tls.ContainerJFRSecret),
			SubPath:   "keystore.p12",
			ReadOnly:  true,
		}

		// Mount the CA cert in the expected /truststore location
		caCertMount := corev1.VolumeMount{
			Name:      "tls-secret",
			MountPath: fmt.Sprintf("/truststore/%s-ca.crt", cr.Name),
			SubPath:   CAKey,
			ReadOnly:  true,
		}

		mounts = append(mounts, keystoreMount, caCertMount)

		secretMounts := []corev1.VolumeMount{}
		for _, secret := range cr.Spec.TrustedCertSecrets {
			mount := corev1.VolumeMount{
				Name:      secret.SecretName,
				MountPath: fmt.Sprintf("/truststore/%s-%s", secret.SecretName, *secret.CertificateKey),
				SubPath:   *secret.CertificateKey,
				ReadOnly:  true,
			}
			secretMounts = append(secretMounts, mount)
		}

		mounts = append(mounts, secretMounts...)

		// Use HTTPS for liveness probe
		livenessProbeScheme = corev1.URISchemeHTTPS
	}
	imageTag := "quay.io/rh-jmc-team/container-jfr:1.0.0-BETA1"
	if cr.Spec.Minimal {
		imageTag += "-minimal"
		envs = append(envs, corev1.EnvVar{
			Name:  "USE_LOW_MEM_PRESSURE_STREAMING",
			Value: "true",
		})
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
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: 8181},
					Path:   "/api/v1/clienturl",
					Scheme: livenessProbeScheme,
				},
			},
		},
	}
}

func NewGrafanaSecretForCR(cr *rhjmcv1beta1.ContainerJFR) *corev1.Secret {
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

func NewGrafanaContainer(cr *rhjmcv1beta1.ContainerJFR, tls *TLSConfig) corev1.Container {
	envs := []corev1.EnvVar{}
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
				Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s/%s", tls.GrafanaSecret, corev1.TLSPrivateKeyKey),
			},
			{
				Name:  "GF_SERVER_CERT_FILE",
				Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s/%s", tls.GrafanaSecret, corev1.TLSCertKey),
			},
		}

		tlsSecretMount := corev1.VolumeMount{
			Name:      "grafana-tls-secret",
			MountPath: "/var/run/secrets/rhjmc.redhat.com/" + tls.GrafanaSecret,
			ReadOnly:  true,
		}

		envs = append(envs, tlsEnvs...)
		mounts = append(mounts, tlsSecretMount)

		// Use HTTPS for liveness probe
		livenessProbeScheme = corev1.URISchemeHTTPS
	}
	return corev1.Container{
		Name:         cr.Name + "-grafana",
		Image:        "quay.io/rh-jmc-team/container-jfr-grafana-dashboard:0.1.0",
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

func NewJfrDatasourceContainer(cr *rhjmcv1beta1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-jfr-datasource",
		Image: "quay.io/rh-jmc-team/jfr-datasource:0.0.2",
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

func NewExporterService(cr *rhjmcv1beta1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "container-jfr",
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

func NewCommandChannelService(cr *rhjmcv1beta1.ContainerJFR) *corev1.Service {
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

func NewGrafanaService(cr *rhjmcv1beta1.ContainerJFR) *corev1.Service {
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
// ContainerJFR CR to name its JMX credentials secret
const JMXSecretNameSuffix = "-jmx-auth"

// JMXSecretUserKey indexes the username within the Container JFR JMX auth secret
const JMXSecretUserKey = "CONTAINER_JFR_RJMX_USER"

// JMXSecretPassKey indexes the password within the Container JFR JMX auth secret
const JMXSecretPassKey = "CONTAINER_JFR_RJMX_PASS"

func NewJmxSecretForCR(cr *rhjmcv1beta1.ContainerJFR) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + JMXSecretNameSuffix,
			Namespace: cr.Namespace,
		},
		StringData: map[string]string{
			"CONTAINER_JFR_RJMX_USER": "containerjfr",
			"CONTAINER_JFR_RJMX_PASS": GenPasswd(20),
		},
	}
}

func NewKeystoreSecretForCR(cr *rhjmcv1beta1.ContainerJFR) *corev1.Secret {
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
