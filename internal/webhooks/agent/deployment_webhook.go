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
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// podWebhookLog is for logging in this package.
var deploymentWebhookLog = logf.Log.WithName("deployment-webhook")

//+kubebuilder:webhook:path=/mutate--v1-deployment,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=deployments,verbs=create;update,versions=v1,name=mdeployment.cryostat.io,admissionReviewVersions=v1

type AgentDeploymentWebhook interface {
	SetupWebhookWithManager(mgr ctrl.Manager) error
}

type AgentDeploymentWebhookConfig struct {
	InitImageTag *string
	FIPSEnabled  bool
	common.OSUtils
}

type agentDeploymentWebhook struct {
	*AgentDeploymentWebhookConfig
}

var _ AgentDeploymentWebhook = &agentDeploymentWebhook{}

func NewAgentDeploymentWebhook(config *AgentDeploymentWebhookConfig) AgentWebhook {
	if config.OSUtils == nil {
		config.OSUtils = &common.DefaultOSUtils{}
	}
	return &agentDeploymentWebhook{
		AgentDeploymentWebhookConfig: config,
	}
}

func (r *agentDeploymentWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	gvk, err := apiutil.GVKForObject(&operatorv1beta2.Cryostat{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	deploymentWebhook := admission.WithCustomDefaulter(mgr.GetScheme(), &appsv1.Deployment{}, &deploymentMutator{
		client: mgr.GetClient(),
		config: r.AgentDeploymentWebhookConfig,
		log:    &deploymentWebhookLog,
		gvk:    &gvk,
		ReconcilerTLS: common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
			Client: mgr.GetClient(),
			OS:     r.OSUtils,
		}),
	}).WithRecoverPanic(true)

	// Modify the webhook to never deny the pod from being admitted
	deploymentWebhook.Handler = allowAllRequests(deploymentWebhook.Handler)
	mgr.GetWebhookServer().Register("/mutate--v1-deployment", deploymentWebhook)
	return nil
}

var _ admission.Handler = &allowAllHandlerWrapper{}
