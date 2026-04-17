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
	"slices"
	"strings"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type deploymentMutator struct {
	client client.Client
	log    *logr.Logger
	gvk    *schema.GroupVersionKind
	config *AgentWebhookConfig
	common.ReconcilerTLS
}

var _ admission.CustomDefaulter = &deploymentMutator{}

// Default optionally mutates a deployment to propagate the agent autoconfig labels
// to pods within. The Pod mutator webhook will take care of the rest.
func (r *deploymentMutator) Default(ctx context.Context, obj runtime.Object) error {
	deployment, ok := obj.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("expected a Deployment, but received a %T", obj)
	}

	// Look up Cryostat
	cr := &operatorv1beta2.Cryostat{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      deployment.Labels[constants.AgentLabelCryostatName],
		Namespace: deployment.Labels[constants.AgentLabelCryostatNamespace],
	}, cr)
	if err != nil {
		return err
	}

	// Check if this deployment is within a target namespace of the CR
	if !slices.Contains(cr.Status.TargetNamespaces, deployment.Namespace) {
		return fmt.Errorf("deployment's namespace \"%s\" is not a target namespace of Cryostat \"%s\" in \"%s\"",
			deployment.Namespace, cr.Name, cr.Namespace)
	}

	template := &deployment.Spec.Template

	// Sanity check the non-string labels
	// Callback Port
	_, err = getAgentCallbackPort(deployment.Labels)
	if err != nil {
		return err
	}

	// Write access
	_, err = hasWriteAccess(deployment.Labels)
	if err != nil {
		return err
	}

	// Harvester labels
	_, err = getHarvesterExitMaxAge(deployment.Labels)
	if err != nil {
		return err
	}
	_, err = getHarvesterExitMaxSize(deployment.Labels)
	if err != nil {
		return err
	}

	// Propagate labels that exist. If they don't the pod defaulter will
	// set default values itself.
	for label := range deployment.Labels {
		if strings.HasPrefix(label, constants.AgentLabelPrefix) {
			copyLabelIfExists(template, deployment, label)
		}
	}

	// Use GenerateName for logging if no explicit Name is given
	deploymentName := deployment.Name
	if len(deploymentName) == 0 {
		deploymentName = deployment.GenerateName
	}
	r.log.Info("Configured deployment ", "name", deploymentName, "namespace", deployment.Namespace)
	return nil
}

// Pod defaulter will handle setting default values for missing labels
func copyLabelIfExists(spec *v1.PodTemplateSpec, deployment *appsv1.Deployment, key string) {
	_, exists := deployment.Labels[key]
	if exists {
		spec.Labels[key] = deployment.Labels[key]
	}
}
