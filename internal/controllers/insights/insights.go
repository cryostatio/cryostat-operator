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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *InsightsReconciler) reconcileInsights(ctx context.Context) error {
	err := r.reconcilePullSecret(ctx)
	if err != nil {
		return err
	}
	err = r.reconcileProxyDeployment(ctx)
	if err != nil {
		return err
	}
	return r.reconcileProxyService(ctx)
}

func (r *InsightsReconciler) reconcilePullSecret(ctx context.Context) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProxySecretName,
			Namespace: r.Namespace,
		},
	}
	owner := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: InsightsConfigMapName,
		Namespace: r.Namespace}, owner)
	if err != nil {
		return err
	}

	token, err := r.getTokenFromPullSecret(ctx)
	if err != nil {
		return err
	}

	params := &apiCastConfigParams{
		FrontendDomains:       fmt.Sprintf("\"%s\",\"%s.%s.svc.cluster.local\"", ProxyServiceName, ProxyServiceName, r.Namespace),
		BackendInsightsDomain: r.backendDomain,
		ProxyDomain:           r.proxyDomain,
		HeaderValue:           *token,
	}
	config, err := getAPICastConfig(params)
	if err != nil {
		return err
	}

	return r.createOrUpdateProxySecret(ctx, secret, owner, *config)
}

func (r *InsightsReconciler) reconcileProxyDeployment(ctx context.Context) error {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProxyDeploymentName,
			Namespace: r.Namespace,
		},
	}
	owner := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: InsightsConfigMapName,
		Namespace: r.Namespace}, owner)
	if err != nil {
		return err
	}

	return r.createOrUpdateProxyDeployment(ctx, deploy, owner)
}

func (r *InsightsReconciler) reconcileProxyService(ctx context.Context) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProxyServiceName,
			Namespace: r.Namespace,
		},
	}
	owner := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: InsightsConfigMapName,
		Namespace: r.Namespace}, owner)
	if err != nil {
		return err
	}

	return r.createOrUpdateProxyService(ctx, svc, owner)
}

func (r *InsightsReconciler) getTokenFromPullSecret(ctx context.Context) (*string, error) {
	// Get the global pull secret
	pullSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, pullSecret)
	if err != nil {
		return nil, err
	}

	// Look for the .dockerconfigjson key within it
	dockerConfigRaw, pres := pullSecret.Data[corev1.DockerConfigJsonKey]
	if !pres {
		return nil, fmt.Errorf("no %s key present in pull secret", corev1.DockerConfigJsonKey)
	}

	// Unmarshal the .dockerconfigjson into a struct
	dockerConfig := struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}{}
	err = json.Unmarshal(dockerConfigRaw, &dockerConfig)
	if err != nil {
		return nil, err
	}

	// Look for the "cloud.openshift.com" auth
	openshiftAuth, pres := dockerConfig.Auths["cloud.openshift.com"]
	if !pres {
		return nil, errors.New("no \"cloud.openshift.com\" auth within pull secret")
	}

	token := strings.TrimSpace(openshiftAuth.Auth)
	if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
		return nil, fmt.Errorf("invalid cloud.openshift.com token")
	}
	return &token, nil
}

func (r *InsightsReconciler) createOrUpdateProxySecret(ctx context.Context, secret *corev1.Secret, owner metav1.Object,
	config string) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set the config map as controller
		if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
			return err
		}
		// Add the APICast config.json
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}
		secret.StringData["config.json"] = config
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Secret %s", op), "name", secret.Name, "namespace", secret.Namespace)
	return nil
}

func (r *InsightsReconciler) createOrUpdateProxyDeployment(ctx context.Context, deploy *appsv1.Deployment, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		labels := map[string]string{"app": ProxyDeploymentName}
		annotations := map[string]string{}
		common.MergeLabelsAndAnnotations(&deploy.ObjectMeta, labels, annotations)
		// Set the config map as controller
		if err := controllerutil.SetControllerReference(owner, deploy, r.Scheme); err != nil {
			return err
		}
		// Immutable, only updated when the deployment is created
		if deploy.CreationTimestamp.IsZero() {
			// Selector is immutable, avoid modifying if possible
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": ProxyDeploymentName,
				},
			}
		}

		// Update pod template spec
		r.createOrUpdateProxyPodSpec(deploy)
		// Update pod template metadata
		common.MergeLabelsAndAnnotations(&deploy.Spec.Template.ObjectMeta, labels, annotations)
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Deployment %s", op), "name", deploy.Name, "namespace", deploy.Namespace)
	return nil
}

func (r *InsightsReconciler) createOrUpdateProxyService(ctx context.Context, svc *corev1.Service, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		// Update labels and annotations
		labels := map[string]string{"app": ProxyDeploymentName}
		annotations := map[string]string{}
		common.MergeLabelsAndAnnotations(&svc.ObjectMeta, labels, annotations)

		// Set the config map as controller
		if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
			return err
		}
		// Update the service type
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = map[string]string{
			"app": ProxyDeploymentName,
		}
		svc.Spec.Ports = []corev1.ServicePort{
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
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Service %s", op), "name", svc.Name, "namespace", svc.Namespace)
	return nil
}

const (
	defaultProxyCPURequest = "50m"
	defaultProxyCPULimit   = "200m"
	defaultProxyMemRequest = "64Mi"
	defaultProxyMemLimit   = "128Mi"
)

func (r *InsightsReconciler) createOrUpdateProxyPodSpec(deploy *appsv1.Deployment) {
	privEscalation := false
	nonRoot := true
	readOnlyMode := int32(0440)

	podSpec := &deploy.Spec.Template.Spec
	// Create the container if it doesn't exist
	var container *corev1.Container
	if deploy.CreationTimestamp.IsZero() {
		podSpec.Containers = []corev1.Container{{}}
	}
	container = &podSpec.Containers[0]

	// Set fields that are hard-coded by operator
	container.Name = ProxyDeploymentName
	container.Image = r.proxyImageTag
	container.Env = []corev1.EnvVar{
		{
			Name:  "THREESCALE_CONFIG_FILE",
			Value: "/tmp/gateway-configuration-volume/config.json",
		},
	}
	container.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "gateway-configuration-volume",
			MountPath: "/tmp/gateway-configuration-volume",
			ReadOnly:  true,
		},
	}
	container.Ports = []corev1.ContainerPort{
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
	}
	container.SecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: &privEscalation,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{constants.CapabilityAll},
		},
	}
	container.LivenessProbe = &corev1.Probe{
		InitialDelaySeconds: 10,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/status/live",
				Port: intstr.FromInt(8090),
			},
		},
	}
	container.ReadinessProbe = &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       30,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/status/ready",
				Port: intstr.FromInt(8090),
			},
		},
	}

	// Set resource requirements only on creation, this allows
	// the user to modify them if they wish
	if deploy.CreationTimestamp.IsZero() {
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(defaultProxyCPURequest),
				corev1.ResourceMemory: resource.MustParse(defaultProxyMemRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(defaultProxyCPULimit),
				corev1.ResourceMemory: resource.MustParse(defaultProxyMemLimit),
			},
		}
	}

	podSpec.Volumes = []corev1.Volume{ // TODO detect change and redeploy
		{
			Name: "gateway-configuration-volume",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ProxySecretName,
					Items: []corev1.KeyToPath{
						{
							Key:  "config.json",
							Path: "config.json",
							Mode: &readOnlyMode,
						},
					},
				},
			},
		},
	}
	podSpec.SecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot:   &nonRoot,
		SeccompProfile: common.SeccompProfile(true),
	}
}
