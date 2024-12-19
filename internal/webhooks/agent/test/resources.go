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

package test

import (
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentWebhookTestResources struct {
	*test.TestResources
}

func (r *AgentWebhookTestResources) NewPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-webhook-test",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"cryostat.io/name":      r.Name,
				"cryostat.io/namespace": r.Namespace,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "example.com/test:latest",
					Env: []corev1.EnvVar{
						{
							Name:  "TEST",
							Value: "some-value",
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &[]bool{false}[0],
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{
								"ALL",
							},
						},
					},
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &[]bool{true}[0],
			},
		},
	}
}

func (r *AgentWebhookTestResources) NewPodJavaToolOptions() *corev1.Pod {
	pod := r.NewPod()
	container := &pod.Spec.Containers[0]
	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: "-Dexisting=var",
		})
	return pod
}

func (r *AgentWebhookTestResources) NewPodJavaToolOptionsFrom() *corev1.Pod {
	pod := r.NewPod()
	container := &pod.Spec.Containers[0]
	container.Env = append(container.Env,
		corev1.EnvVar{
			Name: "JAVA_TOOL_OPTIONS",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "my-config",
					},
					Key:      "java-tool-options",
					Optional: &[]bool{true}[0],
				},
			},
		})
	return pod
}

func (r *AgentWebhookTestResources) NewPodOtherNamespace(namespace string) *corev1.Pod {
	pod := r.NewPod()
	pod.Namespace = namespace
	return pod
}

func (r *AgentWebhookTestResources) NewPodNoNameLabel() *corev1.Pod {
	pod := r.NewPod()
	delete(pod.Labels, "cryostat.io/name")
	return pod
}

func (r *AgentWebhookTestResources) NewPodNoNamespaceLabel() *corev1.Pod {
	pod := r.NewPod()
	delete(pod.Labels, "cryostat.io/namespace")
	return pod
}

type mutatedPodOptions struct {
	javaToolOptions string
	namespace       string
	image           string
	pullPolicy      corev1.PullPolicy
	proxyPort       int32
}

func (r *AgentWebhookTestResources) setDefaultMutatedPodOptions(options *mutatedPodOptions) {
	if len(options.namespace) == 0 {
		options.namespace = r.Namespace
	}
	if len(options.image) == 0 {
		options.image = "quay.io/cryostat/cryostat-agent-init:latest"
	}
	if len(options.pullPolicy) == 0 {
		options.pullPolicy = corev1.PullAlways
	}
	if options.proxyPort == 0 {
		options.proxyPort = 8282
	}
}

func (r *AgentWebhookTestResources) NewMutatedPod() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{})
}

func (r *AgentWebhookTestResources) NewMutatedPodJavaToolOptions() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		javaToolOptions: "-Dexisting=var ",
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodOtherNamespace(namespace string) *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		namespace: namespace,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodCustomImage() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		image:      "example.com/agent-init:2.0.0",
		pullPolicy: corev1.PullIfNotPresent,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodCustomDevImage() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		image:      "example.com/agent-init:latest",
		pullPolicy: corev1.PullAlways,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodProxyPort() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		proxyPort: 8080,
	})
}

func (r *AgentWebhookTestResources) newMutatedPod(options *mutatedPodOptions) *corev1.Pod {
	r.setDefaultMutatedPodOptions(options)
	scheme := "https"
	if !r.TLS {
		scheme = "http"
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-webhook-test",
			Namespace: options.namespace,
			Labels: map[string]string{
				"cryostat.io/name":      r.Name,
				"cryostat.io/namespace": r.Namespace,
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:            "cryostat-agent-init",
					Image:           options.image,
					ImagePullPolicy: options.pullPolicy,
					Command:         []string{"cp", "-v", "/cryostat/agent/cryostat-agent-shaded.jar", "/tmp/cryostat-agent/cryostat-agent-shaded.jar"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "cryostat-agent-init",
							MountPath: "/tmp/cryostat-agent",
						},
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{
								"ALL",
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "example.com/test:latest",
					Env: []corev1.EnvVar{
						{
							Name:  "TEST",
							Value: "some-value",
						},
						{
							Name:  "CRYOSTAT_AGENT_BASEURI",
							Value: fmt.Sprintf("%s://%s-agent.%s.svc:%d", scheme, r.Name, r.Namespace, options.proxyPort),
						},
						{
							Name: "CRYOSTAT_AGENT_POD_NAME",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "metadata.name",
								},
							},
						},
						{
							Name:  "CRYOSTAT_AGENT_APP_NAME",
							Value: "$(CRYOSTAT_AGENT_POD_NAME)",
						},
						{
							Name: "CRYOSTAT_AGENT_POD_IP",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									APIVersion: "v1",
									FieldPath:  "status.podIP",
								},
							},
						},
						{
							Name:  "CRYOSTAT_AGENT_API_WRITES_ENABLED",
							Value: "true", // TODO default to writes enabled, separate label?
						},

						{
							Name:  "CRYOSTAT_AGENT_CALLBACK_SCHEME",
							Value: scheme,
						},
						{
							Name:  "CRYOSTAT_AGENT_CALLBACK_HOST_NAME",
							Value: "$(CRYOSTAT_AGENT_POD_NAME), $(CRYOSTAT_AGENT_POD_IP)[replace(\".\"\\, \"-\")]",
						},
						{
							Name:  "CRYOSTAT_AGENT_CALLBACK_DOMAIN_NAME",
							Value: fmt.Sprintf("%s.%s.svc", r.GetAgentServiceName(), options.namespace),
						},
						{
							Name:  "CRYOSTAT_AGENT_CALLBACK_PORT",
							Value: "9977",
						},
						{
							Name:  "JAVA_TOOL_OPTIONS",
							Value: options.javaToolOptions + "-javaagent:/tmp/cryostat-agent/cryostat-agent-shaded.jar",
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &[]bool{false}[0],
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{
								"ALL",
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "cryostat-agent-init",
							MountPath: "/tmp/cryostat-agent",
							ReadOnly:  true,
						},
					},
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &[]bool{true}[0],
			},
			Volumes: []corev1.Volume{
				{
					Name: "cryostat-agent-init",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	if r.TLS {
		container := &pod.Spec.Containers[0]
		tlsEnvs := []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_CERT_PATH",
				Value: "/var/run/secrets/io.cryostat/cryostat-agent/tls.crt",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_KEY_PATH",
				Value: "/var/run/secrets/io.cryostat/cryostat-agent/tls.key",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__PATH",
				Value: "/var/run/secrets/io.cryostat/cryostat-agent/ca.crt",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__TYPE",
				Value: "X.509",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__ALIAS",
				Value: "cryostat",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_FILE",
				Value: "/var/run/secrets/io.cryostat/cryostat-agent/tls.crt",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_TYPE",
				Value: "X.509",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_ALIAS",
				Value: "cryostat",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_PATH",
				Value: "/var/run/secrets/io.cryostat/cryostat-agent/tls.key",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_TYPE",
				Value: "RSA",
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_ALIAS",
				Value: "cryostat",
			},
		}
		container.Env = append(container.Env, tlsEnvs...)
		container.VolumeMounts = append(container.VolumeMounts,
			corev1.VolumeMount{
				Name:      "cryostat-agent-tls",
				MountPath: "/var/run/secrets/io.cryostat/cryostat-agent",
				ReadOnly:  true,
			})
		pod.Spec.Volumes = append(pod.Spec.Volumes,
			corev1.Volume{
				Name: "cryostat-agent-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  r.GetClusterUniqueNameForAgent(options.namespace),
						DefaultMode: &[]int32{0440}[0],
					},
				},
			})
	}

	return pod
}
