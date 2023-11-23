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
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/test"
	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type InsightsTestResources struct {
	*test.TestResources
	Resources *corev1.ResourceRequirements
}

const expectedOperatorVersion = "2.5.0-dev"

func (r *InsightsTestResources) NewGlobalPullSecret() *corev1.Secret {
	config := `{"auths":{"example.com":{"auth":"hello"},"cloud.openshift.com":{"auth":"world"}}}`
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: "openshift-config",
		},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(config),
		},
	}
}

func (r *InsightsTestResources) NewOperatorDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-operator-controller-manager",
			Namespace: r.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "controller-manager",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"control-plane": "controller-manager",
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

func (r *InsightsTestResources) NewProxyConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "insights-proxy",
			Namespace: r.Namespace,
		},
	}
}

func (r *InsightsTestResources) NewInsightsProxySecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apicastconf",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"config.json": fmt.Sprintf(`{
				"services": [
				  {
					"id": "1",
					"backend_version": "1",
					"proxy": {
					  "hosts": ["insights-proxy","insights-proxy.%s.svc.cluster.local"],
					  "api_backend": "https://insights.example.com:443/",
					  "backend": { "endpoint": "http://127.0.0.1:8081", "host": "backend" },
					  "policy_chain": [
						{
						  "name": "default_credentials",
						  "version": "builtin",
						  "configuration": {
							"auth_type": "user_key",
							"user_key": "dummy_key"
						  }
						},
						{
						  "name": "headers",
						  "version": "builtin",
						  "configuration": {
							"request": [
							  {
								"op": "set",
								"header": "Authorization",
								"value_type": "plain",
								"value": "Bearer world"
							  },
							  {
								"op": "set",
								"header": "User-Agent",
								"value_type": "plain",
								"value": "cryostat-operator/%s cluster/abcde"
							  }
							]
						  }
						},
						{
						  "name": "apicast.policy.apicast"
						}
					  ],
					  "proxy_rules": [
						{
						  "http_method": "POST",
						  "pattern": "/",
						  "metric_system_name": "hits",
						  "delta": 1,
						  "parameters": [],
						  "querystring_parameters": {}
						}
					  ]
					}
				  }
				]
			  }`, r.Namespace, expectedOperatorVersion),
		},
	}
}

func (r *InsightsTestResources) NewInsightsProxySecretWithProxyDomain() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apicastconf",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"config.json": fmt.Sprintf(`{
				"services": [
				  {
					"id": "1",
					"backend_version": "1",
					"proxy": {
					  "hosts": ["insights-proxy","insights-proxy.%s.svc.cluster.local"],
					  "api_backend": "https://insights.example.com:443/",
					  "backend": { "endpoint": "http://127.0.0.1:8081", "host": "backend" },
					  "policy_chain": [
						{
						  "name": "default_credentials",
						  "version": "builtin",
						  "configuration": {
							"auth_type": "user_key",
							"user_key": "dummy_key"
						  }
						},
						{
						  "name": "apicast.policy.http_proxy",
						  "configuration": {
						    "https_proxy": "http://proxy.example.com/",
						    "http_proxy": "http://proxy.example.com/"
						  }
						},
						{
						  "name": "headers",
						  "version": "builtin",
						  "configuration": {
							"request": [
							  {
								"op": "set",
								"header": "Authorization",
								"value_type": "plain",
								"value": "Bearer world"
							  },
							  {
								"op": "set",
								"header": "User-Agent",
								"value_type": "plain",
								"value": "cryostat-operator/%s cluster/abcde"
							  }
							]
						  }
						},
						{
						  "name": "apicast.policy.apicast"
						}
					  ],
					  "proxy_rules": [
						{
						  "http_method": "POST",
						  "pattern": "/",
						  "metric_system_name": "hits",
						  "delta": 1,
						  "parameters": [],
						  "querystring_parameters": {}
						}
					  ]
					}
				  }
				]
			  }`, r.Namespace, expectedOperatorVersion),
		},
	}
}

func (r *InsightsTestResources) NewInsightsProxyDeployment() *appsv1.Deployment {
	var resources *corev1.ResourceRequirements
	if r.Resources != nil {
		resources = r.Resources
	} else {
		resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "insights-proxy",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app": "insights-proxy",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "insights-proxy",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "insights-proxy",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "insights-proxy",
							Image: "example.com/proxy:latest",
							Env: []corev1.EnvVar{
								{
									Name:  "THREESCALE_CONFIG_FILE",
									Value: "/tmp/gateway-configuration-volume/config.json",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "gateway-configuration-volume",
									MountPath: "/tmp/gateway-configuration-volume",
									ReadOnly:  true,
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "proxy",
									ContainerPort: 8080,
								},
								{
									Name:          "management",
									ContainerPort: 8090,
								},
								{
									Name:          "metrics",
									ContainerPort: 9421,
								},
							},
							Resources: *resources,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/status/live",
										Port: intstr.FromInt(8090),
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 15,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/status/ready",
										Port: intstr.FromInt(8090),
									},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "gateway-configuration-volume",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "apicastconf",
									Items: []corev1.KeyToPath{
										{
											Key:  "config.json",
											Path: "config.json",
											Mode: &[]int32{0440}[0],
										},
									},
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
					},
				},
			},
		},
	}
}

func (r *InsightsTestResources) NewInsightsProxyService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "insights-proxy",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app": "insights-proxy",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": "insights-proxy",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "proxy",
					Port:       8080,
					TargetPort: intstr.FromString("proxy"),
				},
				{
					Name:       "management",
					Port:       8090,
					TargetPort: intstr.FromString("management"),
				},
			},
		},
	}
}

func (r *InsightsTestResources) NewClusterVersion() *configv1.ClusterVersion {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Spec: configv1.ClusterVersionSpec{
			ClusterID: "abcde",
		},
	}
}
