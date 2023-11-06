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

package test

import (
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/test"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type InsightsTestResources struct {
	*test.TestResources
}

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
								"value": "world"
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
			  }`, r.Namespace),
		},
	}
}

func (r *InsightsTestResources) NewInsightsProxyDeployment() *appsv1.Deployment {
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
							Resources: corev1.ResourceRequirements{}, // TODO
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
