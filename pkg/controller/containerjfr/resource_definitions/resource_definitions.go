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

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServiceSpecs struct {
	CoreAddress       string
	CommandAddress    string
	GrafanaAddress    string
	DatasourceAddress string
}

func NewPersistentVolumeClaimForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.PersistentVolumeClaim {
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

func NewDeploymentForCR(cr *rhjmcv1alpha1.ContainerJFR, specs *ServiceSpecs) *appsv1.Deployment {
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
				"redhat.com/containerJfrUrl":   specs.CoreAddress,
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
						"redhat.com/containerJfrUrl": specs.CoreAddress,
					},
				},
				Spec: *NewPodForCR(cr, specs),
			},
		},
	}
}

func NewPodForCR(cr *rhjmcv1alpha1.ContainerJFR, specs *ServiceSpecs) *corev1.PodSpec {
	storePass := GenPasswd(20)
	var containers []corev1.Container
	if cr.Spec.Minimal {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, storePass),
		}
	} else {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs, storePass),
			NewGrafanaContainer(cr),
			NewJfrDatasourceContainer(cr),
		}
	}
	return &corev1.PodSpec{
		ServiceAccountName: "container-jfr-operator",
		Volumes: []corev1.Volume{
			{
				Name: cr.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: cr.Name,
					},
				},
			},
			{
				Name: "service-certs",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cr.Name,
					},
				},
			},
			{
				Name: "ca-bundle",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cr.Name + "-ca-bundle",
						},
					},
				},
			},
			{
				Name: "keystore",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "truststore",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		Containers: containers,
		InitContainers: []corev1.Container{
			NewTLSSetupInitContainer(cr, storePass),
		},
	}
}

func NewCoreContainer(cr *rhjmcv1alpha1.ContainerJFR, specs *ServiceSpecs, storePass string) corev1.Container {
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
			Value: specs.CoreAddress,
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
			Value: specs.CommandAddress,
		},
		{
			Name:  "GRAFANA_DASHBOARD_URL",
			Value: specs.GrafanaAddress,
		},
		{
			Name:  "GRAFANA_DATASOURCE_URL",
			Value: specs.DatasourceAddress,
		},
		{
			Name:  "KEYSTORE_PATH",
			Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s-pkcs12/keystore.p12", cr.Name),
		},
		{
			Name:  "KEYSTORE_PASS",
			Value: storePass,
		},
		{
			// FIXME remove once JMX auth support is present in operator
			Name:  "CONTAINER_JFR_DISABLE_JMX_AUTH",
			Value: "true",
		},
	}
	imageTag := "quay.io/rh-jmc-team/container-jfr:0.20.0"
	if cr.Spec.Minimal {
		imageTag += "-minimal"
		envs = append(envs, corev1.EnvVar{
			Name:  "USE_LOW_MEM_PRESSURE_STREAMING",
			Value: "true",
		})
	}
	return corev1.Container{
		Name:  cr.Name,
		Image: imageTag,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cr.Name,
				MountPath: "flightrecordings",
			},
			{
				Name:      "keystore",
				MountPath: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s-pkcs12", cr.Name),
			},
			{
				Name:      "truststore",
				MountPath: "/truststore",
			},
		},
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
		Env: envs,
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cr.Name + "-jmx-auth",
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port:   intstr.IntOrString{IntVal: 8181},
					Path:   "/api/v1/clienturl",
					Scheme: corev1.URISchemeHTTPS,
				},
			},
		},
	}
}

func NewGrafanaSecretForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Secret {
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

func NewGrafanaContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-grafana",
		Image: "docker.io/grafana/grafana:6.4.4",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3000,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "GF_INSTALL_PLUGINS",
				Value: "grafana-simple-json-datasource",
			},
		},
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
					Port: intstr.IntOrString{IntVal: 3000},
					Path: "/api/health",
				},
			},
		},
	}
}

func NewJfrDatasourceContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-jfr-datasource",
		Image: "quay.io/rh-jmc-team/jfr-datasource:0.0.1",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8080,
			},
		},
		Env: []corev1.EnvVar{},
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Port: intstr.IntOrString{IntVal: 8080},
					Path: "/",
				},
			},
		},
	}
}

func NewTLSSetupInitContainer(cr *rhjmcv1alpha1.ContainerJFR, storePass string) corev1.Container {
	bashCmd := "openssl pkcs12 -export -inkey $(KEY_FILE)" +
		" -in $(CERT_FILE) -out $(KEYSTORE_FILE)" +
		" -password pass:$(KEYSTORE_PASS)" +
		" && chmod 644 $(KEYSTORE_FILE)" +
		" && csplit -z -f /truststore/crt- $(CA_CERTS) '/-----BEGIN CERTIFICATE-----/' '{*}'"
	return corev1.Container{
		Name: "tls-setup",
		// TODO Better or custom image?
		Image:   "registry.access.redhat.com/ubi8/s2i-base", // UBI image with openssl
		Command: []string{"/bin/bash"},
		Args: []string{
			"-c",
			bashCmd,
		},
		Env: []corev1.EnvVar{
			{
				Name:  "KEY_FILE",
				Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s/tls.key", cr.Name),
			},
			{
				Name:  "CERT_FILE",
				Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s/tls.crt", cr.Name),
			},
			{
				Name:  "KEYSTORE_FILE",
				Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s-pkcs12/keystore.p12", cr.Name),
			},
			{
				Name:  "KEYSTORE_PASS",
				Value: storePass,
			},
			{
				Name:  "CA_CERTS",
				Value: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s-ca-bundle/service-ca.crt", cr.Name),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "service-certs",
				MountPath: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s", cr.Name),
			},
			{
				Name:      "ca-bundle",
				MountPath: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s-ca-bundle", cr.Name),
			},
			{
				Name:      "keystore",
				MountPath: fmt.Sprintf("/var/run/secrets/rhjmc.redhat.com/%s-pkcs12", cr.Name),
			},
			{
				Name:      "truststore",
				MountPath: "/truststore",
			},
		},
	}
}

const servingCertAnnotation = "service.beta.openshift.io/serving-cert-secret-name"

func NewExporterService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "container-jfr",
			},
			Annotations: map[string]string{
				// Get OpenShift to generate key-pair/certificate and store in secret
				servingCertAnnotation: cr.Name,
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

func NewCommandChannelService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
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

func NewGrafanaService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
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

func NewJfrDatasourceService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-jfr-datasource",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "jfr-datasource",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "8080-tcp",
					Port:       8080,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				},
			},
		},
	}
}

func NewJmxSecretForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-jmx-auth",
			Namespace: cr.Namespace,
		},
		StringData: map[string]string{
			"CONTAINER_JFR_RJMX_USER": "containerjfr",
			"CONTAINER_JFR_RJMX_PASS": GenPasswd(20),
		},
	}
}

func NewCABundleConfigMap(cr *rhjmcv1alpha1.ContainerJFR) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca-bundle",
			Namespace: cr.Namespace,
			Annotations: map[string]string{
				"service.beta.openshift.io/inject-cabundle": "true",
			},
		},
	}
}
