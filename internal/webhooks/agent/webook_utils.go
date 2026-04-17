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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

func getLogLevel(labels map[string]string) string {
	result := defaultLogLevel
	value, pres := labels[constants.AgentLabelLogLevel]
	if pres {
		result = value
	}
	return result
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

func getSmartTriggersConfigMapNames(labels map[string]string) []string {
	result := ""
	value, pres := labels[constants.AgentLabelSmartTriggersConfigMaps]
	if pres {
		result = value
	}
	return strings.Split(result, ",")
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

func getHarvesterPeriod(labels map[string]string) (*int32, error) {
	period, pres := labels[constants.AgentLabelHarvesterPeriod]
	if !pres {
		return nil, nil
	}

	parsed, err := time.ParseDuration(period)
	if err != nil {
		return nil, fmt.Errorf("invalid label value for \"%s\": %s", constants.AgentLabelHarvesterPeriod, err.Error())
	}
	value := int32(parsed.Milliseconds())
	return &value, nil
}

func getHarvesterMaxFiles(labels map[string]string) (*int32, error) {
	maxFiles, pres := labels[constants.AgentLabelHarvesterMaxFiles]
	if !pres {
		return nil, nil
	}

	parsed, err := strconv.ParseInt(maxFiles, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid label value for \"%s\": %s", constants.AgentLabelHarvesterMaxFiles, err.Error())
	}
	if parsed <= 0 {
		return nil, fmt.Errorf("invalid label value for \"%s\": must be positive", constants.AgentLabelHarvesterMaxFiles)
	}
	value := int32(parsed)
	return &value, nil
}

func getResourceRequirements(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{}
	if cr.Spec.AgentOptions != nil {
		resources = cr.Spec.AgentOptions.Resources.DeepCopy()
	}
	common.PopulateResourceRequest(resources, agentInitCpuRequest, agentInitMemoryRequest,
		agentInitCpuLimit, agentInitMemoryLimit)
	return resources
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
