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
	"math"
	"strconv"

	"github.com/cryostatio/cryostat-operator/internal/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

func (r *AgentWebhookTestResources) NewPodMultiContainer() *corev1.Pod {
	pod := r.NewPod()
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name:  "other",
		Image: "example.com/other:latest",
		Env: []corev1.EnvVar{
			{
				Name:  "OTHER",
				Value: "some-other-value",
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
	})
	return pod
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

func (r *AgentWebhookTestResources) NewPodPortLabel() *corev1.Pod {
	pod := r.NewPod()
	pod.Labels["cryostat.io/callback-port"] = "9998"
	return pod
}

func (r *AgentWebhookTestResources) NewPodPortLabelInvalid() *corev1.Pod {
	pod := r.NewPod()
	pod.Labels["cryostat.io/callback-port"] = "not-an-int"
	return pod
}

func (r *AgentWebhookTestResources) NewPodPortLabelTooBig() *corev1.Pod {
	pod := r.NewPod()
	pod.Labels["cryostat.io/callback-port"] = strconv.FormatInt(math.MaxInt32+1, 10)
	return pod
}

func (r *AgentWebhookTestResources) NewPodContainerLabel() *corev1.Pod {
	pod := r.NewPodMultiContainer()
	pod.Labels["cryostat.io/container"] = "other"
	return pod
}

func (r *AgentWebhookTestResources) NewPodContainerBadLabel() *corev1.Pod {
	pod := r.NewPodMultiContainer()
	pod.Labels["cryostat.io/container"] = "wrong"
	return pod
}

func (r *AgentWebhookTestResources) NewPodReadOnlyLabel() *corev1.Pod {
	pod := r.NewPod()
	pod.Labels["cryostat.io/read-only"] = "true"
	return pod
}

func (r *AgentWebhookTestResources) NewPodReadOnlyLabelInvalid() *corev1.Pod {
	pod := r.NewPod()
	pod.Labels["cryostat.io/read-only"] = "banana"
	return pod
}

type mutatedPodOptions struct {
	javaToolOptions string
	namespace       string
	image           string
	pullPolicy      corev1.PullPolicy
	gatewayPort     int32
	callbackPort    int32
	writeAccess     *bool
	scheme          string
	// Function to produce mutated container array
	containersFunc func(*AgentWebhookTestResources, *mutatedPodOptions) []corev1.Container
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
	if options.gatewayPort == 0 {
		options.gatewayPort = 8282
	}
	if options.callbackPort == 0 {
		options.callbackPort = 9977
	}
	if options.writeAccess == nil {
		options.writeAccess = &[]bool{true}[0]
	}
	options.scheme = "https"
	if !r.TLS {
		options.scheme = "http"
	}
	if options.containersFunc == nil {
		options.containersFunc = newMutatedContainers
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

func (r *AgentWebhookTestResources) NewMutatedPodGatewayPort() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		gatewayPort: 8080,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodCallbackPort() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		callbackPort: 9998,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodMultiContainer() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		containersFunc: newMutatedMultiContainers,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodContainerLabel() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		containersFunc: newMutatedMultiContainersLabel,
	})
}

func (r *AgentWebhookTestResources) NewMutatedPodReadOnlyLabel() *corev1.Pod {
	return r.newMutatedPod(&mutatedPodOptions{
		writeAccess: &[]bool{false}[0],
	})
}

func (r *AgentWebhookTestResources) newMutatedPod(options *mutatedPodOptions) *corev1.Pod {
	r.setDefaultMutatedPodOptions(options)
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
			Containers: options.containersFunc(r, options),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &[]bool{true}[0],
			},
			Volumes: []corev1.Volume{
				{
					Name: "cryostat-agent-init",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: &[]resource.Quantity{resource.MustParse("50Mi")}[0],
						},
					},
				},
			},
		},
	}

	if r.TLS {
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

func newMutatedContainers(r *AgentWebhookTestResources, options *mutatedPodOptions) []corev1.Container {
	containers := r.NewPodMultiContainer().Spec.Containers
	return []corev1.Container{*r.newMutatedContainer(&containers[0], options)}
}

func newMutatedMultiContainers(r *AgentWebhookTestResources, options *mutatedPodOptions) []corev1.Container {
	containers := r.NewPodMultiContainer().Spec.Containers
	return []corev1.Container{*r.newMutatedContainer(&containers[0], options), containers[1]}
}

func newMutatedMultiContainersLabel(r *AgentWebhookTestResources, options *mutatedPodOptions) []corev1.Container {
	containers := r.NewPodMultiContainer().Spec.Containers
	return []corev1.Container{containers[0], *r.newMutatedContainer(&containers[1], options)}
}

func (r *AgentWebhookTestResources) newMutatedContainer(original *corev1.Container, options *mutatedPodOptions) *corev1.Container {
	container := &corev1.Container{
		Name:  original.Name,
		Image: original.Image,
		Env: append(original.Env, []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_BASEURI",
				Value: fmt.Sprintf("%s://%s-agent.%s.svc:%d", options.scheme, r.Name, r.Namespace, options.gatewayPort),
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
				Value: strconv.FormatBool(*options.writeAccess),
			},
			{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_PORT",
				Value: strconv.Itoa(int(options.callbackPort)),
			},
			{
				Name:  "JAVA_TOOL_OPTIONS",
				Value: options.javaToolOptions + "-javaagent:/tmp/cryostat-agent/cryostat-agent-shaded.jar",
			},
		}...),
		Ports: []corev1.ContainerPort{
			{
				Name:          "cryostat-cb",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: options.callbackPort,
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
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
	}

	if r.TLS {
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
	}

	var callbackEnvs []corev1.EnvVar
	if r.DisableAgentHostnameVerify {
		callbackEnvs = []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK",
				Value: fmt.Sprintf("%s://$(CRYOSTAT_AGENT_POD_IP):%d", options.scheme, options.callbackPort),
			},
		}
	} else {
		callbackEnvs = []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK_SCHEME",
				Value: options.scheme,
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
				Value: strconv.Itoa(int(options.callbackPort)),
			},
		}
	}
	container.Env = append(container.Env, callbackEnvs...)

	return container
}
