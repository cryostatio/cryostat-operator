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

package console

import (
	"context"
	"fmt"
	"slices"

	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type PluginInstaller struct {
	Client    client.Client
	Namespace string
	Scheme    *runtime.Scheme
	Log       logr.Logger
}

func (r *PluginInstaller) InstallConsolePlugin(ctx context.Context) error {
	err := r.createConsolePlugin(ctx)
	if err != nil {
		return err
	}
	return r.registerConsolePlugin(ctx)
}

func (r *PluginInstaller) createConsolePlugin(ctx context.Context) error {
	plugin := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.ConsolePluginName,
		},
	}

	// Use the plugin's ClusterRoleBinding as an owner.
	// Since the binding is managed by OLM, this will cause the ConsolePlugin
	// to be garbage collected when the operator is uninstalled.
	// We could use any OLM-managed object as owner, but since ConsolePlugin
	// is cluster-scoped, the owner must also be cluster-scoped.
	owner := &rbacv1.ClusterRoleBinding{}
	fmt.Printf("%v\n", r.Client)
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.ConsoleClusterRoleBindingName}, owner)
	if err != nil {
		return err
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, plugin, func() error {
		// Apply standard labels
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app.kubernetes.io/instance", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app.kubernetes.io/name", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app.kubernetes.io/part-of", constants.ConsolePluginName)

		err := controllerutil.SetOwnerReference(owner, plugin, r.Scheme)
		if err != nil {
			return err
		}

		// Configure the Plugin Spec
		plugin.Spec.DisplayName = constants.AppName
		plugin.Spec.Backend = consolev1.ConsolePluginBackend{
			Type: consolev1.Service,
			Service: &consolev1.ConsolePluginService{
				BasePath:  "/",
				Name:      constants.ConsoleServiceName,
				Namespace: r.Namespace,
				Port:      constants.ConsoleServicePort,
			},
		}
		plugin.Spec.I18n.LoadType = consolev1.Preload
		plugin.Spec.Proxy = []consolev1.ConsolePluginProxy{
			{
				Alias:         constants.ConsoleProxyName,
				Authorization: consolev1.UserToken,
				Endpoint: consolev1.ConsolePluginProxyEndpoint{
					Type: consolev1.ProxyTypeService,
					Service: &consolev1.ConsolePluginProxyServiceConfig{
						Name:      constants.ConsoleServiceName,
						Namespace: r.Namespace,
						Port:      constants.ConsoleServicePort,
					},
				},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	r.Log.Info(fmt.Sprintf("Console Plugin %s", op), "name", plugin.Name)
	return nil
}

func (r *PluginInstaller) registerConsolePlugin(ctx context.Context) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		console := &openshiftoperatorv1.Console{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: constants.ConsoleCRName}, console)
		if err != nil {
			return err
		}

		// Check if this plugin is already registered
		if !slices.Contains(console.Spec.Plugins, constants.ConsolePluginName) {
			// Add this plugin to the list
			console.Spec.Plugins = append(console.Spec.Plugins, constants.ConsolePluginName)
			err := r.Client.Update(ctx, console)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
