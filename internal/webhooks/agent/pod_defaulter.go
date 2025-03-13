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
	"time"

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
	agentArg                    = "-javaagent:/tmp/cryostat-agent/cryostat-agent-shaded.jar"
	podNameEnvVar               = "CRYOSTAT_AGENT_POD_NAME"
	podIPEnvVar                 = "CRYOSTAT_AGENT_POD_IP"
	agentMaxSizeBytes           = "50Mi"
	agentInitCpuRequest         = "10m"
	agentInitMemoryRequest      = "32Mi"
	defaultJavaOptsVar          = "JAVA_TOOL_OPTIONS"
	defaultHarvesterExitMaxAge  = int32(30000)
	kib                         = int32(1024)
	mib                         = 1024 * kib
	defaultHarvesterExitMaxSize = 20 * mib
)

// Default optionally mutates a pod to inject the Cryostat agent
func (r *podMutator) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod, but received a %T", obj)
	}

	// Check for required labels and return early if missing.
	// This should not happen because such pods are filtered out by Kubernetes server-side due to our object selector.
	if !metav1.HasLabel(pod.ObjectMeta, constants.AgentLabelCryostatName) || !metav1.HasLabel(pod.ObjectMeta, constants.AgentLabelCryostatNamespace) {
		r.log.Info("pod is missing required labels")
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
	container, err := getTargetContainer(pod)
	if err != nil {
		return err
	}

	// Determine the callback port number
	port, err := getAgentCallbackPort(pod.Labels)
	if err != nil {
		return err
	}

	// Check whether write access has been disabled
	write, err := hasWriteAccess(pod.Labels)
	if err != nil {
		return err
	}

	harvesterTemplate := getHarvesterTemplate(pod.Labels)
	harvesterExitMaxAge, err := getHarvesterExitMaxAge(pod.Labels)
	if err != nil {
		return err
	}
	harvesterExitMaxSize, err := getHarvesterExitMaxSize(pod.Labels)
	if err != nil {
		return err
	}

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
		Resources: *getResourceRequirements(crModel),
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
			Value: strconv.FormatBool(*write),
		},
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_WEBSERVER_PORT",
			Value: strconv.Itoa(int(*port)),
		},
	)

	if len(harvesterTemplate) > 0 {
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_HARVESTER_TEMPLATE",
				Value: harvesterTemplate,
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_HARVESTER_EXIT_MAX_AGE_MS",
				Value: strconv.Itoa(int(*harvesterExitMaxAge)),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_HARVESTER_EXIT_MAX_SIZE_B",
				Value: strconv.Itoa(int(*harvesterExitMaxSize)),
			},
		)
	}

	// Append a port for the callback server
	container.Ports = append(container.Ports, corev1.ContainerPort{
		Name:          constants.AgentCallbackPortName,
		Protocol:      corev1.ProtocolTCP,
		ContainerPort: *port,
	})

	// Append callback environment variables
	container.Env = append(container.Env, r.callbackEnv(crModel, pod.Namespace, tlsEnabled, *port)...)

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
	} else {
		// Allow the agent to connect to non-HTTPS Cryostat server
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_REQUIRED",
				Value: "false",
			})
	}

	// Inject agent using JAVA_TOOL_OPTIONS or specified variable, appending to any existing value
	extended, err := extendJavaOptsVar(container.Env, getJavaOptionsVar(pod.Labels))
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
	return fmt.Sprintf("%s://%s.%s.svc:%d", scheme, common.AgentGatewayServiceName(cr), cr.InstallNamespace,
		getAgentGatewayHTTPPort(cr))
}

func getAgentGatewayHTTPPort(cr *model.CryostatInstance) int32 {
	port := constants.AgentProxyContainerPort
	if cr.Spec.ServiceOptions != nil && cr.Spec.ServiceOptions.AgentGatewayConfig != nil &&
		cr.Spec.ServiceOptions.AgentGatewayConfig.HTTPPort != nil {
		port = *cr.Spec.ServiceOptions.AgentGatewayConfig.HTTPPort
	}
	return port
}

func getAgentCallbackPort(labels map[string]string) (*int32, error) {
	result := constants.AgentCallbackContainerPort
	port, pres := labels[constants.AgentLabelCallbackPort]
	if pres {
		// Parse the label value into an int32 and return an error if invalid
		parsed, err := strconv.ParseInt(port, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid label value for \"%s\": %s", constants.AgentLabelCallbackPort, err.Error())
		}
		result = int32(parsed)
	}
	return &result, nil
}

func hasWriteAccess(labels map[string]string) (*bool, error) {
	// Default to true
	result := true
	value, pres := labels[constants.AgentLabelReadOnly]
	if pres {
		// Parse the label value into a bool and return an error if invalid
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid label value for \"%s\": %s", constants.AgentLabelReadOnly, err.Error())
		}
		result = !parsed
	}
	return &result, nil
}

func getJavaOptionsVar(labels map[string]string) string {
	result := defaultJavaOptsVar
	value, pres := labels[constants.AgentLabelJavaOptionsVar]
	if pres {
		result = value
	}
	return result
}

func getHarvesterTemplate(labels map[string]string) string {
	result := ""
	value, pres := labels[constants.AgentLabelHarvesterTemplate]
	if pres {
		result = value
	}
	return result
}

func getHarvesterExitMaxAge(labels map[string]string) (*int32, error) {
	value := defaultHarvesterExitMaxAge
	age, pres := labels[constants.AgentLabelHarvesterExitMaxAge]
	if pres {
		// Parse the label value into an int32 and return an error if invalid
		parsed, err := time.ParseDuration(age)
		if err != nil {
			return nil, fmt.Errorf("invalid label value for \"%s\": %s", constants.AgentLabelHarvesterExitMaxAge, err.Error())
		}
		value = int32(parsed.Milliseconds())
	}
	return &value, nil
}

func getHarvesterExitMaxSize(labels map[string]string) (*int32, error) {
	value := defaultHarvesterExitMaxSize
	size, pres := labels[constants.AgentLabelHarvesterExitMaxSize]
	if pres {
		parsed, err := resource.ParseQuantity(size)
		if err != nil {
			return nil, fmt.Errorf("invalid label value for \"%s\": %s", constants.AgentLabelHarvesterExitMaxSize, err.Error())
		}
		value = int32(parsed.Value())
	}
	return &value, nil
}

func getResourceRequirements(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.AgentOptions != nil {
		resources = cr.Spec.AgentOptions.Resources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, agentInitCpuRequest, agentInitMemoryRequest)
	return resources
}

func (r *podMutator) callbackEnv(cr *model.CryostatInstance, namespace string, tls bool, containerPort int32) []corev1.EnvVar {
	scheme := "https"
	if !tls {
		scheme = "http"
	}

	var envs []corev1.EnvVar
	if cr.Spec.AgentOptions != nil && cr.Spec.AgentOptions.DisableHostnameVerification {
		envs = []corev1.EnvVar{
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK",
				Value: fmt.Sprintf("%s://$(%s):%d", scheme, podIPEnvVar, containerPort),
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
				Value: fmt.Sprintf("%s.%s.svc", common.AgentCallbackServiceName(r.gvk, cr), namespace),
			},
			{
				Name:  "CRYOSTAT_AGENT_CALLBACK_PORT",
				Value: strconv.Itoa(int(containerPort)),
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

func getTargetContainer(pod *corev1.Pod) (*corev1.Container, error) {
	if len(pod.Spec.Containers) == 0 {
		// Should never happen, Kubernetes doesn't allow this
		return nil, errors.New("pod has no containers")
	}
	label, pres := pod.Labels[constants.AgentLabelContainer]
	if !pres {
		// Use the first container by default
		return &pod.Spec.Containers[0], nil
	}
	// Find the container matching the label
	return findNamedContainer(pod.Spec.Containers, label)
}

func findNamedContainer(containers []corev1.Container, name string) (*corev1.Container, error) {
	for i, container := range containers {
		if container.Name == name {
			return &containers[i], nil
		}
	}
	return nil, fmt.Errorf("no container found with name \"%s\"", name)
}

func extendJavaOptsVar(envs []corev1.EnvVar, javaOptsVar string) ([]corev1.EnvVar, error) {
	existing, err := findJavaOptsVar(envs, javaOptsVar)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.Value += " " + agentArg
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  javaOptsVar,
			Value: agentArg,
		})
	}

	return envs, nil
}

func findJavaOptsVar(envs []corev1.EnvVar, javaOptsVar string) (*corev1.EnvVar, error) {
	for i, env := range envs {
		if env.Name == javaOptsVar {
			if env.ValueFrom != nil {
				return nil, fmt.Errorf("environment variable %s uses \"valueFrom\" and cannot be extended", javaOptsVar)
			}
			return &envs[i], nil
		}
	}
	return nil, nil
}
