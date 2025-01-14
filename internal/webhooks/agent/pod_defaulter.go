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

package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type podMutator struct {
	client client.Client
	log    *logr.Logger
	gvk    *schema.GroupVersionKind
	config *AgentWebhookConfig
	common.ReconcilerTLS
}

var _ admission.CustomDefaulter = &podMutator{}

const (
	agentArg               = "-javaagent:/tmp/cryostat-agent/cryostat-agent-shaded.jar"
	podNameEnvVar          = "CRYOSTAT_AGENT_POD_NAME"
	podIPEnvVar            = "CRYOSTAT_AGENT_POD_IP"
	agentMaxSizeBytes      = "50Mi"
	agentInitCpuRequest    = "10m"
	agentInitMemoryRequest = "32Mi"
)

// Default optionally mutates a pod to inject the Cryostat agent
func (r *podMutator) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod, but received a %T", obj)
	}

	// TODO do this with objectSelector: https://github.com/kubernetes-sigs/controller-tools/issues/553
	// Check for required labels and return early if missing
	if !metav1.HasLabel(pod.ObjectMeta, constants.AgentLabelCryostatName) || !metav1.HasLabel(pod.ObjectMeta, constants.AgentLabelCryostatNamespace) {
		return nil
	}

	// Look up Cryostat
	cr := &operatorv1beta2.Cryostat{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      pod.Labels[constants.AgentLabelCryostatName],
		Namespace: pod.Labels[constants.AgentLabelCryostatNamespace],
	}, cr)
	if err != nil {
		return err
	}

	// Check if this pod is within a target namespace of the CR
	if !slices.Contains(cr.Status.TargetNamespaces, pod.Namespace) {
		return fmt.Errorf("pod's namespace \"%s\" is not a target namespace of Cryostat \"%s\" in \"%s\"",
			pod.Namespace, cr.Name, cr.Namespace)
	}

	// Check whether TLS is enabled for this CR
	crModel := model.FromCryostat(cr)
	tlsEnabled := r.IsCertManagerEnabled(crModel)

	// Select target container
	if len(pod.Spec.Containers) == 0 {
		// Should never happen, Kubernetes doesn't allow this
		return errors.New("pod has no containers")
	}
	// TODO make configurable with label
	container := &pod.Spec.Containers[0]

	// Add init container
	nonRoot := true
	imageTag := r.getImageTag()
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:            "cryostat-agent-init",
		Image:           imageTag,
		ImagePullPolicy: common.GetPullPolicy(imageTag),
		Command:         []string{"cp", "-v", "/cryostat/agent/cryostat-agent-shaded.jar", "/tmp/cryostat-agent/cryostat-agent-shaded.jar"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "cryostat-agent-init",
				MountPath: "/tmp/cryostat-agent",
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot: &nonRoot,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{
					constants.CapabilityAll,
				},
			},
		},
		Resources: corev1.ResourceRequirements{ // TODO allow customization with CRD
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(agentInitCpuRequest),
				corev1.ResourceMemory: resource.MustParse(agentInitMemoryRequest),
			},
		},
	})

	// Add emptyDir volume to copy agent into, and mount it
	sizeLimit := resource.MustParse(agentMaxSizeBytes)
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: "cryostat-agent-init",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: &sizeLimit,
			},
		},
	})

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "cryostat-agent-init",
		MountPath: "/tmp/cryostat-agent",
		ReadOnly:  true,
	})

	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_BASEURI",
			Value: cryostatURL(crModel, tlsEnabled),
		},
		corev1.EnvVar{
			Name: podNameEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_APP_NAME",
			Value: fmt.Sprintf("$(%s)", podNameEnvVar),
		},
		corev1.EnvVar{
			Name: podIPEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_API_WRITES_ENABLED",
			Value: "true", // TODO default to writes enabled, separate label?
		},
	)

	// Append callback environment variables
	container.Env = append(container.Env, r.callbackEnv(crModel, pod.Namespace, tlsEnabled)...)

	if tlsEnabled {
		// Mount the certificate volume
		readOnlyMode := int32(0440)
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "cryostat-agent-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  common.AgentCertificateName(r.gvk, crModel, pod.Namespace),
					DefaultMode: &readOnlyMode,
				},
			},
		})

		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "cryostat-agent-tls",
			MountPath: "/var/run/secrets/io.cryostat/cryostat-agent",
			ReadOnly:  true,
		})

		// Configure the Cryostat agent to use client certificate authentication
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_CERT_PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSCertKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_KEY_PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSPrivateKeyKey),
			},
		)

		// Configure the Cryostat agent to trust the Cryostat CA
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", constants.CAKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__TYPE",
				Value: "X.509",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__ALIAS",
				Value: "cryostat",
			},
		)

		// Configure the Cryostat agent to use HTTPS for its callback server
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_FILE",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSCertKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_TYPE",
				Value: "X.509",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_ALIAS",
				Value: "cryostat",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSPrivateKeyKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_TYPE",
				Value: "RSA",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_ALIAS",
				Value: "cryostat",
			},
		)
	}
	// Inject agent using JAVA_TOOL_OPTIONS, appending to any existing value
	extended, err := extendJavaToolOptions(container.Env)
	if err != nil {
		return err
	}
	container.Env = extended

	// Use GenerateName for logging if no explicit Name is given
	podName := pod.Name
	if len(podName) == 0 {
		podName = pod.GenerateName
	}
	r.log.Info("configured Cryostat agent for pod", "name", podName, "namespace", pod.Namespace)

	return nil
}

func cryostatURL(cr *model.CryostatInstance, tls bool) string {
	// Build the URL to the agent proxy service
	scheme := "https"
	if !tls {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s.%s.svc:%d", scheme, common.AgentProxyServiceName(cr), cr.InstallNamespace,
		getAgentProxyHTTPPort(cr))
}

func getAgentProxyHTTPPort(cr *model.CryostatInstance) int32 {
	port := constants.AgentProxyContainerPort
	if cr.Spec.ServiceOptions != nil && cr.Spec.ServiceOptions.AgentConfig != nil && cr.Spec.ServiceOptions.AgentConfig.HTTPPort != nil {
		port = *cr.Spec.ServiceOptions.AgentConfig.HTTPPort
	}
	return port
}

func (r *podMutator) callbackEnv(cr *model.CryostatInstance, namespace string, tls bool) []corev1.EnvVar {
	scheme := "https"
	if !tls {
		scheme = "http"
	}

	// TODO make customizable
	port := 9977

	var envs []corev1.EnvVar
	if cr.Spec.AgentOptions != nil && cr.Spec.AgentOptions.DisableHostnameVerification {
		envs = []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK",
				Value: fmt.Sprintf("%s://$(%s):%d", scheme, podIPEnvVar, port),
			},
		}
	} else {
		envs = []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK_SCHEME",
				Value: scheme,
			},
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK_HOST_NAME",
				Value: fmt.Sprintf("$(%s), $(%s)[replace(\".\"\\, \"-\")]", podNameEnvVar, podIPEnvVar),
			},
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK_DOMAIN_NAME",
				Value: fmt.Sprintf("%s.%s.svc", common.AgentHeadlessServiceName(r.gvk, cr), namespace),
			},
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK_PORT",
				Value: strconv.Itoa(port),
			},
		}
	}

	return envs
}

func (r *podMutator) getImageTag() string {
	// Lazily look up image tag
	if r.config.InitImageTag == nil {
		agentInitImage := r.config.GetEnvOrDefault(agentInitImageTagEnv, constants.DefaultAgentInitImageTag)
		r.config.InitImageTag = &agentInitImage
	}
	return *r.config.InitImageTag
}

func extendJavaToolOptions(envs []corev1.EnvVar) ([]corev1.EnvVar, error) {
	existing, err := findJavaToolOptions(envs)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.Value += " " + agentArg
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: agentArg,
		})
	}

	return envs, nil
}

var errJavaToolOptionsValueFrom error = errors.New("environment variable JAVA_TOOL_OPTIONS uses \"valueFrom\" and cannot be extended")

func findJavaToolOptions(envs []corev1.EnvVar) (*corev1.EnvVar, error) {
	for i, env := range envs {
		if env.Name == "JAVA_TOOL_OPTIONS" {
			if env.ValueFrom != nil {
				return nil, errJavaToolOptionsValueFrom
			}
			return &envs[i], nil
		}
	}
	return nil, nil
}
