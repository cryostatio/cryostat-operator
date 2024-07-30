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
	"fmt"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type podMutator struct {
	client client.Client
	log    *logr.Logger
}

var _ admission.CustomDefaulter = &podMutator{}

// Default optionally mutates a pod to inject the Cryostat agent
func (r *podMutator) Default(ctx context.Context, obj runtime.Object) error {
	// FIXME Do not return error, it blocks pod creation
	pod, ok := obj.(*v1.Pod)
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

	// Select target container
	if len(pod.Spec.Containers) == 0 {
		return nil
	}
	container := &pod.Spec.Containers[0]

	// Add init container
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, v1.Container{
		Name:    "cryostat-agent-init",
		Image:   "quay.io/cryostat/cryostat-agent-init:latest", // TODO related images
		Command: []string{"cp", "-v", "/cryostat/agent/cryostat-agent-shaded.jar", "/tmp/cryostat-agent/cryostat-agent-shaded.jar"},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "cryostat-agent-init",
				MountPath: "/tmp/cryostat-agent",
			},
		},
	})

	// Add emptyDir volume to copy agent into, and mount it
	pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
		Name: "cryostat-agent-init",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{}, // TODO size limit?
		},
	})

	container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
		Name:      "cryostat-agent-init",
		MountPath: "/tmp/cryostat-agent",
		ReadOnly:  true,
	})

	// TODO JAVA_TOOL_OPTIONS for -javaagent?

	// TODO figure out authz. may not be suitable to create a service account in each target ns with pod/exec perms.
	// someone with access to the namespace should not necessarily have access to that permission.
	//
	// could do an authz check on webhook user. if they have pod/exec permissions, then create the sa and/or secret.
	// this would count as a side effect of the webhook. this could still allow other ns users to escalate though.

	return nil
}
