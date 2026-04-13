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
	"strings"
	"time"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		return fmt.Errorf("pod's namespace \"%s\" is not a target namespace of Cryostat \"%s\" in \"%s\"",
			pod.Namespace, cr.Name, cr.Namespace)
	}

	template, err := getTargetPodTemplate(&deployment)
	if err != nil {
		return err
	}
	
	targetCryostat, err := getCryostatName(template.ObjectMeta.Labels)
	if err != nil {
		return err
	}

	targetNamespace, err := getCryostatNamespace(template.ObjectMeta.Labels)
	if err != nil {
		return err
	}

	metav1.SetMetaDataLabel(&template.ObjectMeta, AgentLabelCryostatName, targetCryostat)
	metav1.SetMetaDataLabel(&template.ObjectMeta, AgentLabelCryostatNamespace, targetNamespace)

	r.log.Info("configured Cryostat agent for pod", "name", podName, "namespace", pod.Namespace)

	return nil
}

func getTargetPodTemplate(deployment *appsv1.Deployment) (*corev1.PodTemplate, error) {
	if !deployment.Spec.Template {
		// Should never happen, Kubernetes doesn't allow this
		return nil, errors.New("Deployment has no Template.")
	}
	template := deployment.Spec.Template
	
	return template
}


func getCryostatName(labels map[string]string) string {
	result := ""
	value, pres := labels[constants.AgentLabelCryostatName]
	if pres {
		result = value
	}
	return result
}

func getCryostatNamespace(labels map[string]string) string {
	result := ""
	value, pres := labels[constants.AgentLabelCryostatNamespace]
	if pres {
		result = value
	}
	return result
}
