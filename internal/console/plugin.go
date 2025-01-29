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
	"errors"
	"fmt"
	"slices"

	"github.com/blang/semver/v4"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type PluginInstaller struct {
	Client    client.Client
	Namespace string
	Scheme    *runtime.Scheme
	Log       logr.Logger
}

// Verify that *PluginInstaller implements manager.Runnable and manager.LeaderElectionRunnable.
var _ manager.Runnable = (*PluginInstaller)(nil)
var _ manager.LeaderElectionRunnable = (*PluginInstaller)(nil)

// Minimum OpenShift version that supports the plugin
const minOpenShiftVersion = "4.15.0"

// Maximum OpenShift version that supports the plugin
const maxOpenShiftVersion = "99.99.0" // Placeholder until needed

// Start implements manager.Runnable.
func (r *PluginInstaller) Start(ctx context.Context) error {
	return r.installConsolePlugin(ctx)
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
func (r *PluginInstaller) NeedLeaderElection() bool {
	return true
}

func (r *PluginInstaller) installConsolePlugin(ctx context.Context) error {
	compat, err := r.isOpenShiftCompatible(ctx)
	if err != nil {
		return err
	}
	if !compat {
		// Return early if this OpenShift cluster is not compatible
		return nil
	}
	err = r.createConsolePlugin(ctx)
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

	owner, err := r.findOwner(ctx)
	if err != nil {
		// Treat this as a warning, and let the ConsolePlugin be unowned if we can't find
		// the appropriate owner
		r.Log.Error(err, "could not locate owner for ConsolePlugin")
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, plugin, func() error {
		// Apply standard labels
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app.kubernetes.io/instance", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app.kubernetes.io/name", constants.ConsolePluginName)
		metav1.SetMetaDataLabel(&plugin.ObjectMeta, "app.kubernetes.io/part-of", constants.ConsolePluginName)

		if owner != nil {
			err := controllerutil.SetOwnerReference(owner, plugin, r.Scheme)
			if err != nil {
				return err
			}
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

func (r *PluginInstaller) SetupWithManager(mgr ctrl.Manager) error {
	return mgr.Add(r)
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
			r.Log.Info("Console updated", "name", console.Name)
		}
		return nil
	})
}

func (r *PluginInstaller) findOwner(ctx context.Context) (*rbacv1.ClusterRoleBinding, error) {
	// Use the plugin's ClusterRoleBinding as an owner.
	// Since the binding is managed by OLM, this will cause the ConsolePlugin
	// to be garbage collected when the operator is uninstalled.
	// We could use any OLM-managed object as owner, but since ConsolePlugin
	// is cluster-scoped, the owner must also be cluster-scoped.

	// Look up the operator's deployment, which should have been installed by OLM
	deploy := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.OperatorDeploymentName, Namespace: r.Namespace}, deploy)
	if err != nil {
		return nil, err
	}

	// OLM should have placed these labels on the deployment, which should have the
	// same value on the ClusterRoleBindings it installed for our operator.
	keys := []string{"olm.owner", "olm.owner.kind", "olm.owner.namespace"}
	selector := labels.Set{}
	for _, key := range keys {
		value, pres := deploy.Labels[key]
		if !pres {
			return nil, fmt.Errorf("could not find OLM label \"%s\"", key)
		}
		selector[key] = value
	}

	// Get a list of all ClusterRoleBindings whose labels point to
	// our operator.
	bindings := &rbacv1.ClusterRoleBindingList{}
	err = r.Client.List(ctx, bindings, &client.ListOptions{
		LabelSelector: selector.AsSelector(),
	})
	if err != nil {
		return nil, err
	}

	// Look for the ClusterRoleBinding that corresponds to the
	// OpenShift Console plugin.
	for i, binding := range bindings.Items {
		for _, subject := range binding.Subjects {
			if subject.Name == constants.ConsoleServiceAccountName && subject.Kind == "ServiceAccount" {
				return &bindings.Items[i], nil
			}
		}
	}
	return nil, errors.New("could not find console plugin cluster role")
}

func (r *PluginInstaller) isOpenShiftCompatible(ctx context.Context) (bool, error) {
	// Build a semver.Version from the minimum/maximum version
	minVersion := semver.MustParse(minOpenShiftVersion)
	maxVersion := semver.MustParse(maxOpenShiftVersion)

	// Look up the cluster's version
	version, err := r.getOpenShiftVersion(ctx)
	if err != nil {
		return false, err
	}

	// Check that the cluster is newer than the minimum
	if version.LT(minVersion) {
		r.Log.Info(fmt.Sprintf("OpenShift version %s is older than the minimum required (%s) for the Console Plugin. Plugin installation will be skipped.",
			version.String(), minVersion.String()))
		return false, nil
	}
	if version.GT(maxVersion) {
		r.Log.Info(fmt.Sprintf("OpenShift version %s is newer than the maximum allowed (%s) for the Console Plugin. Plugin installation will be skipped.",
			version.String(), maxVersion.String()))
		return false, nil
	}
	return true, nil
}

func (r *PluginInstaller) getOpenShiftVersion(ctx context.Context) (*semver.Version, error) {
	// Look up OpenShift version from the ClusterVersion object
	clusterVersion := &configv1.ClusterVersion{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: constants.ClusterVersionName}, clusterVersion)
	if err != nil {
		return nil, err
	}

	// Strip off any suffix from the desired version
	trimmedVer, err := semver.FinalizeVersion(clusterVersion.Status.Desired.Version)
	if err != nil {
		return nil, err
	}
	// Parse result back into a semver.Version
	version := semver.MustParse(trimmedVer)
	return &version, nil
}
