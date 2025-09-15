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

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// podWebhookLog is for logging in this package.
var podWebhookLog = logf.Log.WithName("pod-webhook")

// Environment variable to override the agent init container image
const agentInitImageTagEnv = "RELATED_IMAGE_AGENT_INIT"

//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod.cryostat.io,admissionReviewVersions=v1

type AgentWebhook interface {
	SetupWebhookWithManager(mgr ctrl.Manager) error
}

type AgentWebhookConfig struct {
	InitImageTag *string
	FIPSEnabled  bool
	common.OSUtils
}

type agentWebhook struct {
	*AgentWebhookConfig
}

var _ AgentWebhook = &agentWebhook{}

func NewAgentWebhook(config *AgentWebhookConfig) AgentWebhook {
	if config.OSUtils == nil {
		config.OSUtils = &common.DefaultOSUtils{}
	}
	return &agentWebhook{
		AgentWebhookConfig: config,
	}
}

func (r *agentWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	gvk, err := apiutil.GVKForObject(&operatorv1beta2.Cryostat{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	webhook := admission.WithCustomDefaulter(mgr.GetScheme(), &corev1.Pod{}, &podMutator{
		client: mgr.GetClient(),
		config: r.AgentWebhookConfig,
		log:    &podWebhookLog,
		gvk:    &gvk,
		ReconcilerTLS: common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
			Client: mgr.GetClient(),
			OS:     r.OSUtils,
		}),
	}).WithRecoverPanic(true)
	// Modify the webhook to never deny the pod from being admitted
	webhook.Handler = allowAllRequests(webhook.Handler)
	mgr.GetWebhookServer().Register("/mutate--v1-pod", webhook)
	return nil
}

type allowAllHandlerWrapper struct {
	impl admission.Handler
}

func (r *allowAllHandlerWrapper) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Call the handler implementation
	result := r.impl.Handle(ctx, req)
	if !result.Allowed {
		msg := ""
		if result.Result != nil {
			msg = result.Result.Message
		}
		podWebhookLog.Info("pod mutation failed", "result", msg)
	}
	// Modify the result to always permit the request
	result.Allowed = true
	return result
}

var _ admission.Handler = &allowAllHandlerWrapper{}

func allowAllRequests(handler admission.Handler) admission.Handler {
	return &allowAllHandlerWrapper{
		impl: handler,
	}
}
