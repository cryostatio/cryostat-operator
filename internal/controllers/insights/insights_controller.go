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

// Copyright The Cryostat Authors
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package insights

import (
	"context"
	"errors"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/go-logr/logr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// TODO
type InsightsReconciler struct {
	*InsightsReconcilerConfig
	backendDomain string
	proxyDomain   string
	proxyImageTag string
}

type InsightsReconcilerConfig struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Namespace string
	common.OSUtils
}

const (
	InsightsConfigMapName    = "insights-proxy"
	ProxyDeploymentName      = InsightsConfigMapName
	ProxyServiceName         = ProxyDeploymentName
	ProxySecretName          = "apicastconf"
	EnvInsightsBackendDomain = "INSIGHTS_BACKEND_DOMAIN"
	EnvInsightsProxyDomain   = "INSIGHTS_PROXY_DOMAIN"
	EnvInsightsEnabled       = "INSIGHTS_ENABLED"
	// Environment variable to override the Insights proxy image
	EnvInsightsProxyImageTag = "RELATED_IMAGE_INSIGHTS_PROXY"
)

func NewInsightsReconciler(config *InsightsReconcilerConfig) (*InsightsReconciler, error) {
	backendDomain := config.GetEnv(EnvInsightsBackendDomain)
	if len(backendDomain) == 0 {
		return nil, errors.New("no backend domain provided for Insights")
	}
	imageTag := config.GetEnv(EnvInsightsProxyImageTag)
	if len(imageTag) == 0 {
		return nil, errors.New("no proxy image tag provided for Insights")
	}
	proxyDomain := config.GetEnv(EnvInsightsProxyDomain)

	return &InsightsReconciler{
		InsightsReconcilerConfig: config,
		backendDomain:            backendDomain,
		proxyDomain:              proxyDomain,
		proxyImageTag:            imageTag,
	}, nil
}

// +kubebuilder:rbac:groups=apps,resources=deployments;deployments/finalizers,verbs=*
// +kubebuilder:rbac:groups="",resources=services;secrets;configmaps;configmaps/finalizers,verbs=*

// Reconcile processes a Cryostat CR and manages a Cryostat installation accordingly
func (r *InsightsReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Insights Proxy")

	// Reconcile all Insights support
	err := r.reconcileInsights(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InsightsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := ctrl.NewControllerManagedBy(mgr).
		Named("insights").
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.isPullSecretOrProxyConfig)).
		Watches(&source.Kind{Type: &appsv1.Deployment{}},
			handler.EnqueueRequestsFromMapFunc(r.isProxyDeployment)).
		Watches(&source.Kind{Type: &corev1.Service{}},
			handler.EnqueueRequestsFromMapFunc(r.isProxyService))
	return c.Complete(r)
}

func (r *InsightsReconciler) isPullSecretOrProxyConfig(secret client.Object) []reconcile.Request {
	if !(secret.GetNamespace() == "openshift-config" && secret.GetName() == "pull-secret") &&
		!(secret.GetNamespace() == r.Namespace && secret.GetName() == ProxySecretName) {
		return nil
	}
	return r.proxyDeploymentRequest()
}

func (r *InsightsReconciler) isProxyDeployment(deploy client.Object) []reconcile.Request {
	if deploy.GetNamespace() != r.Namespace || deploy.GetName() != ProxyDeploymentName {
		return nil
	}
	return r.proxyDeploymentRequest()
}

func (r *InsightsReconciler) isProxyService(svc client.Object) []reconcile.Request {
	if svc.GetNamespace() != r.Namespace || svc.GetName() != ProxyServiceName {
		return nil
	}
	return r.proxyDeploymentRequest()
}

func (r *InsightsReconciler) proxyDeploymentRequest() []reconcile.Request {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: r.Namespace, Name: ProxyDeploymentName}}
	return []reconcile.Request{req}
}
