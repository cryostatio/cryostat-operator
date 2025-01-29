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

package test

import (
	"github.com/cryostatio/cryostat-operator/internal/test"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type PluginTestResources struct {
	*test.TestResources
}

func (r *PluginTestResources) NewConsolePlugin() *consolev1.ConsolePlugin {
	return &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cryostat-plugin",
			Labels: map[string]string{
				"app":                        "cryostat-plugin",
				"app.kubernetes.io/instance": "cryostat-plugin",
				"app.kubernetes.io/name":     "cryostat-plugin",
				"app.kubernetes.io/part-of":  "cryostat-plugin",
			},
		},
		Spec: consolev1.ConsolePluginSpec{
			DisplayName: "Cryostat",
			Backend: consolev1.ConsolePluginBackend{
				Type: consolev1.Service,
				Service: &consolev1.ConsolePluginService{
					Name:      "cryostat-plugin",
					Namespace: r.Namespace,
					Port:      9443,
					BasePath:  "/",
				},
			},
			I18n: consolev1.ConsolePluginI18n{
				LoadType: consolev1.Preload,
			},
			Proxy: []consolev1.ConsolePluginProxy{
				{
					Alias:         "cryostat-plugin-proxy",
					Authorization: consolev1.UserToken,
					Endpoint: consolev1.ConsolePluginProxyEndpoint{
						Type: consolev1.ProxyTypeService,
						Service: &consolev1.ConsolePluginProxyServiceConfig{
							Name:      "cryostat-plugin",
							Namespace: r.Namespace,
							Port:      9443,
						},
					},
				},
			},
		},
	}
}

func (r *PluginTestResources) NewConsole() *openshiftoperatorv1.Console {
	return &openshiftoperatorv1.Console{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: openshiftoperatorv1.ConsoleSpec{
			OperatorSpec: openshiftoperatorv1.OperatorSpec{
				ManagementState: openshiftoperatorv1.Managed,
			},
			Plugins: []string{
				"other-plugin",
			},
		},
	}
}

func (r *PluginTestResources) NewConsoleExisting() *openshiftoperatorv1.Console {
	return &openshiftoperatorv1.Console{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: openshiftoperatorv1.ConsoleSpec{
			OperatorSpec: openshiftoperatorv1.OperatorSpec{
				ManagementState: openshiftoperatorv1.Managed,
			},
			Plugins: []string{
				"other-plugin",
				"cryostat-plugin",
			},
		},
	}
}

func (r *PluginTestResources) NewPluginClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cryostat-plugin",
			Labels: map[string]string{
				"olm.owner":           "cryostat-operator.v0.0.0",
				"olm.owner.kind":      "ClusterServiceVersion",
				"olm.owner.namespace": r.Namespace,
				"something":           "else",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "cryostat-plugin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "cryostat-plugin",
				Namespace: r.Namespace,
			},
		},
	}
}

func (r *PluginTestResources) NewPluginClusterRoleBindingMissingLabels() *rbacv1.ClusterRoleBinding {
	binding := r.NewPluginClusterRoleBinding()
	delete(binding.Labels, "olm.owner")
	return binding
}

func (r *PluginTestResources) NewPluginClusterRoleBindingMissingSA() *rbacv1.ClusterRoleBinding {
	binding := r.NewPluginClusterRoleBinding()
	binding.Subjects[0].Name = "not-cryostat-plugin"
	return binding
}

func (r *PluginTestResources) NewOperatorDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-operator-controller",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"olm.owner":           "cryostat-operator.v0.0.0",
				"olm.owner.kind":      "ClusterServiceVersion",
				"olm.owner.namespace": r.Namespace,
				"something":           "different",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: metav1.SetAsLabelSelector(labels.Set{
				"app.kubernetes.io/name": "cryostat-operator",
				"control-plane":          "controller-manager",
			}),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "cryostat-operator",
						"control-plane":          "controller-manager",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "manager",
							Image: "example.com/operator:latest",
						},
					},
				},
			},
		},
	}
}

func (r *PluginTestResources) NewOperatorDeploymentMissingLabels() *appsv1.Deployment {
	deploy := r.NewOperatorDeployment()
	delete(deploy.Labels, "olm.owner")
	return deploy
}

func (r *PluginTestResources) NewClusterVersion() *configv1.ClusterVersion {
	return r.newClusterVersion("4.17.0-foo+bar")
}

func (r *PluginTestResources) NewClusterVersionOld() *configv1.ClusterVersion {
	return r.newClusterVersion("4.12.0-foo+bar")
}

func (r *PluginTestResources) NewClusterVersionNew() *configv1.ClusterVersion {
	return r.newClusterVersion("100.0.0-foo+bar")
}

func (r *PluginTestResources) NewClusterVersionBad() *configv1.ClusterVersion {
	return r.newClusterVersion("")
}

func (r *PluginTestResources) newClusterVersion(version string) *configv1.ClusterVersion {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "00000000-0000-0000-0000-000000000000",
		},
		Status: configv1.ClusterVersionStatus{
			Desired: configv1.Release{
				Version: version,
			},
		},
	}
}
