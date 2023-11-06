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

package insights

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type InsightsIntegration struct {
	Manager ctrl.Manager
	Log     *logr.Logger
	common.OSUtils
}

func NewInsightsIntegration(mgr ctrl.Manager, log *logr.Logger) *InsightsIntegration {
	return &InsightsIntegration{
		Manager: mgr,
		Log:     log,
		OSUtils: &common.DefaultOSUtils{},
	}
}

func (i *InsightsIntegration) Setup() (*url.URL, error) {
	var proxyUrl *url.URL
	namespace := i.getOperatorNamespace()
	// This will happen when running the operator locally
	if len(namespace) == 0 {
		i.Log.Info("Operator namespace not detected, disabling Insights integration")
		return nil, nil
	}

	ctx := context.Background()
	if i.isInsightsEnabled() {
		err := i.createInsightsController(namespace)
		if err != nil {
			i.Log.Error(err, "unable to add controller to manager", "controller", "Insights")
			return nil, err
		}
		// Create a Config Map to be used as a parent of all Insights Proxy related objects
		err = i.createConfigMap(ctx, namespace)
		if err != nil {
			i.Log.Error(err, "failed to create config map for Insights")
			return nil, err
		}
		proxyUrl = i.getProxyURL(namespace)
	} else {
		// Delete any previously created Config Map (and its children)
		err := i.deleteConfigMap(ctx, namespace)
		if err != nil {
			i.Log.Error(err, "failed to delete config map for Insights")
			return nil, err
		}

	}
	return proxyUrl, nil
}

func (i *InsightsIntegration) isInsightsEnabled() bool {
	return strings.ToLower(i.GetEnv(EnvInsightsEnabled)) == "true"
}

func (i *InsightsIntegration) getOperatorNamespace() string {
	return i.GetEnv("NAMESPACE")
}

func (i *InsightsIntegration) createInsightsController(namespace string) error {
	config := &InsightsReconcilerConfig{
		Client:    i.Manager.GetClient(),
		Log:       ctrl.Log.WithName("controllers").WithName("Insights"),
		Scheme:    i.Manager.GetScheme(),
		Namespace: namespace,
		OSUtils:   i.OSUtils,
	}
	controller, err := NewInsightsReconciler(config)
	if err != nil {
		return err
	}
	if err := controller.SetupWithManager(i.Manager); err != nil {
		return err
	}
	return nil
}

func (i *InsightsIntegration) createConfigMap(ctx context.Context, namespace string) error {
	// The config map should be owned by the operator deployment to ensure it and its descendants are garbage collected
	owner := &appsv1.Deployment{}
	// Use the APIReader instead of the cache, since the cache may not be synced yet
	err := i.Manager.GetAPIReader().Get(ctx, types.NamespacedName{
		Name: constants.OperatorDeploymentName, Namespace: namespace}, owner)
	if err != nil {
		return err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InsightsConfigMapName,
			Namespace: namespace,
		},
	}
	err = controllerutil.SetControllerReference(owner, cm, i.Manager.GetScheme())
	if err != nil {
		return err
	}

	err = i.Manager.GetClient().Create(ctx, cm, &client.CreateOptions{})
	if err == nil {
		i.Log.Info("Config Map for Insights created", "name", cm.Name, "namespace", cm.Namespace)
	}
	// This may already exist if the pod restarted
	return client.IgnoreAlreadyExists(err)
}

func (i *InsightsIntegration) deleteConfigMap(ctx context.Context, namespace string) error {
	// Children will be garbage collected
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InsightsConfigMapName,
			Namespace: namespace,
		},
	}

	err := i.Manager.GetClient().Delete(ctx, cm, &client.DeleteOptions{})
	if err == nil {
		i.Log.Info("Config Map for Insights deleted", "name", cm.Name, "namespace", cm.Namespace)
	}
	// This may not exist if no config map was previously created
	return client.IgnoreNotFound(err)
}

func (i *InsightsIntegration) getProxyURL(namespace string) *url.URL {
	return &url.URL{
		Scheme: "http", // TODO add https support (r.IsCertManagerInstalled)
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local", ProxyServiceName, namespace),
	}
}
