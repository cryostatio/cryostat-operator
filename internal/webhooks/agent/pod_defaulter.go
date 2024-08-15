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

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type podMutator struct {
	client client.Client
	log    *logr.Logger
	common.ReconcilerTLS
}

var _ admission.CustomDefaulter = &podMutator{}

const agentArg = "-javaagent:/tmp/cryostat-agent/cryostat-agent-shaded.jar"

// Default optionally mutates a pod to inject the Cryostat agent
func (r *podMutator) Default(ctx context.Context, obj runtime.Object) error {
	// FIXME Do not return error, it blocks pod creation
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod, but received a %T", obj)
	}
	// TODO could get very chatty (also name is blank for GenerateName)
	r.log.Info("checking for Cryostat labels", "name", pod.Name, "namespace", pod.Namespace)

	// TODO do this with objectSelector: https://github.com/kubernetes-sigs/controller-tools/issues/553
	// Check for required labels and return early if missing
	if !metav1.HasLabel(pod.ObjectMeta, LabelCryostatName) || !metav1.HasLabel(pod.ObjectMeta, LabelCryostatNamespace) {
		return nil
	}

	// Look up Cryostat
	cr := &operatorv1beta2.Cryostat{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      pod.Labels[LabelCryostatName],
		Namespace: pod.Labels[LabelCryostatNamespace],
	}, cr)
	if err != nil {
		return err
	}

	// Check if this pod is within a target namespace of the CR
	if !isTargetNamespace(cr.Status.TargetNamespaces, pod.Namespace) {
		return fmt.Errorf("pod's namespace \"%s\" is not a target namespace of Cryostat \"%s\" in \"%s\"",
			pod.Namespace, cr.Name, cr.Namespace)
	}

	// Select target container
	if len(pod.Spec.Containers) == 0 {
		return nil
	}
	container := &pod.Spec.Containers[0]

	// Add init container
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:    "cryostat-agent-init",
		Image:   "quay.io/cryostat/cryostat-agent-init:latest", // TODO related images
		Command: []string{"cp", "-v", "/cryostat/agent/cryostat-agent-shaded.jar", "/tmp/cryostat-agent/cryostat-agent-shaded.jar"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "cryostat-agent-init",
				MountPath: "/tmp/cryostat-agent",
			},
		}, // TODO Resources?
	})

	// Add emptyDir volume to copy agent into, and mount it
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: "cryostat-agent-init",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{}, // TODO size limit?
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
			Value: r.cryostatURL(cr),
		},
		corev1.EnvVar{
			Name: "CRYOSTAT_AGENT_POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_CALLBACK",
			Value: "http://$(CRYOSTAT_AGENT_POD_IP)", // TODO HTTPS
		},
	)
	extended, err := extendJavaToolOptions(container.Env)
	if err != nil {
		return err
	}
	container.Env = extended

	// TODO JAVA_TOOL_OPTIONS for -javaagent?

	// TODO figure out authz. may not be suitable to create a service account in each target ns with pod/exec perms.
	// someone with access to the namespace should not necessarily have access to that permission.
	//
	// TokenRequest API, can we autorenew? Non-expiring token would work but security warns against using it. If
	// using non-expiring token. Remember RBAC is validated against install namespace. Need SA per target namespace,
	// with a RoleBinding in install namespace.
	//
	// could do an authz check on webhook user. if they have pod/exec permissions, then create the sa and/or secret.
	// this would count as a side effect of the webhook. this could still allow other ns users to escalate though.

	// TODO default to writes enabled, separate label?

	return nil
}

func (r *podMutator) cryostatURL(cr *operatorv1beta2.Cryostat) string {
	scheme := "https"
	if !r.IsCertManagerEnabled(model.FromCryostat(cr)) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s.%s.svc:%d", scheme, cr.Name, cr.Namespace, // TODO maybe use service instead of CR meta
		constants.AuthProxyHttpContainerPort)
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

func isTargetNamespace(targetNamespaces []string, podNamespace string) bool {
	for _, ns := range targetNamespaces {
		if ns == podNamespace {
			return true
		}
	}
	return false
}
