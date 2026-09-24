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

package scorecard

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/blang/semver/v4"
	"github.com/cryostatio/cryostat-operator/internal/controller/constants"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConsolePluginTest checks installation and registration of the OpenShift Console Plugin.
func ConsolePluginTest(bundle *apimanifests.Bundle, namespace string, _ bool) *scapiv1alpha3.TestResult {
	r := newTestResources(ConsolePluginTestName, namespace)
	if err := r.setupCRTestResources(false); err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", ConsolePluginTestName, err))
	}
	if !r.OpenShift {
		r.Log += "Skipping Console Plugin checks on Kubernetes\n"
		return r.TestResult
	}
	manager, err := consolePluginManager(bundle)
	if err != nil {
		return r.fail(err.Error())
	}
	if !slices.Contains(manager.Args, "--openshift-console-plugin") {
		r.Log += "Skipping Console Plugin checks: installation is disabled in the bundle\n"
		return r.TestResult
	}

	config, err := ctrl.GetConfig()
	if err != nil {
		return r.fail(err.Error())
	}
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		configv1.AddToScheme, consolev1.AddToScheme, openshiftoperatorv1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			return r.fail(err.Error())
		}
	}
	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return r.fail(fmt.Sprintf("failed to create OpenShift client: %s", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := r.waitForDeploymentAvailability(ctx, operatorDeploymentName, namespace); err != nil {
		return r.fail(fmt.Sprintf("operator deployment did not become available: %s", err))
	}
	if err := r.checkConsolePlugin(ctx, c, manager.Env); err != nil {
		return r.fail(fmt.Sprintf("Console Plugin check failed: %s", err))
	}
	return r.TestResult
}

func (r *TestResources) checkConsolePlugin(ctx context.Context, c client.Client, env []corev1.EnvVar) error {
	clusterVersion := &configv1.ClusterVersion{}
	if err := c.Get(ctx, types.NamespacedName{Name: constants.ClusterVersionName}, clusterVersion); err != nil {
		return err
	}
	version, err := semver.FinalizeVersion(clusterVersion.Status.Desired.Version)
	if err != nil {
		return err
	}
	parsedVersion := semver.MustParse(version)
	minVersion := consolePluginVersionBound(env, "MIN_OPENSHIFT_VERSION", "0.0.0")
	maxVersion := consolePluginVersionBound(env, "MAX_OPENSHIFT_VERSION", "99.99.0")
	expected := parsedVersion.GTE(minVersion) && parsedVersion.LTE(maxVersion)
	r.Log += fmt.Sprintf("OpenShift version %s, supported range [%s, %s], Console Plugin expected: %t\n",
		clusterVersion.Status.Desired.Version, minVersion, maxVersion, expected)

	plugin := &consolev1.ConsolePlugin{}
	var present, registered bool
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, types.NamespacedName{Name: constants.ConsolePluginName}, plugin)
		if err != nil && !kerrors.IsNotFound(err) {
			return false, err
		}
		present = err == nil
		console := &openshiftoperatorv1.Console{}
		if err := c.Get(ctx, types.NamespacedName{Name: "cluster"}, console); err != nil {
			return false, err
		}
		registered = slices.Contains(console.Spec.Plugins, constants.ConsolePluginName)
		return present == expected && registered == expected, nil
	})
	r.Log += fmt.Sprintf("Console Plugin present: %t, registered: %t\n", present, registered)
	if err != nil {
		return fmt.Errorf("waiting for Console Plugin installation and registration: %w", err)
	}
	if !expected {
		return nil
	}

	backend := plugin.Spec.Backend.Service
	if plugin.Spec.Backend.Type != consolev1.Service || backend == nil || backend.Namespace != r.Namespace {
		return fmt.Errorf("ConsolePlugin does not reference a backend Service in %s", r.Namespace)
	}
	if _, err := r.Client.CoreV1().Services(backend.Namespace).Get(ctx, backend.Name, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("failed to get backend Service: %w", err)
	}
	return r.waitForDeploymentAvailability(ctx, constants.ConsolePluginName, backend.Namespace)
}

func consolePluginManager(bundle *apimanifests.Bundle) (*corev1.Container, error) {
	for _, deployment := range bundle.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if deployment.Name == operatorDeploymentName {
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == "manager" {
					return &container, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("bundle does not contain the manager container in Deployment %s", operatorDeploymentName)
}

func consolePluginVersionBound(env []corev1.EnvVar, name, fallback string) semver.Version {
	for _, variable := range env {
		if variable.Name == name {
			if version, err := semver.Parse(variable.Value); err == nil {
				return version
			}
		}
	}
	// Match the installer's defaults for missing or invalid bounds.
	return semver.MustParse(fallback)
}
