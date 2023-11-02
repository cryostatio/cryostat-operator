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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	owner := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: OperatorDeploymentName,
		Namespace: r.Namespace}, owner)
	if err != nil {
		return err
	}

	token, err := r.getTokenFromPullSecret(ctx)
	if err != nil {
		// TODO warn instead of fail?
		return err
	}

	// TODO convert to APICast secret
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

	return r.createOrUpdateSecret(ctx, secret, owner, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}
		secret.StringData["config.json"] = *config
		return nil
	})
}

func (r *InsightsReconciler) reconcileProxyDeployment(ctx context.Context) error {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProxyDeploymentName,
			Namespace: r.Namespace,
		},
	}
	owner := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: OperatorDeploymentName,
		Namespace: r.Namespace}, owner)
	if err != nil {
		return err
	}

	return r.createOrUpdateDeployment(ctx, deploy, owner)
}

func (r *InsightsReconciler) reconcileProxyService(ctx context.Context) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProxyServiceName,
			Namespace: r.Namespace,
		},
	}
	owner := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: OperatorDeploymentName,
		Namespace: r.Namespace}, owner)
	if err != nil {
		return err
	}

	return r.createOrUpdateService(ctx, svc, owner)
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

	fmt.Println("dockerconfig: " + string(dockerConfigRaw))
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

// FIXME dedup
func (r *InsightsReconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret, owner metav1.Object,
	delegate controllerutil.MutateFn) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
			return err
		}
		// Call the delegate for secret-specific mutations
		return delegate()
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Secret %s", op), "name", secret.Name, "namespace", secret.Namespace)
	return nil
}

// TODO dedup
func (r *InsightsReconciler) createOrUpdateDeployment(ctx context.Context, deploy *appsv1.Deployment, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		labels := map[string]string{"app": ProxyDeploymentName}
		annotations := map[string]string{}
		mergeLabelsAndAnnotations(&deploy.ObjectMeta, labels, annotations)
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, deploy, r.Scheme); err != nil {
			return err
		}
		// Immutable, only updated when the deployment is created
		if deploy.CreationTimestamp.IsZero() {
			// Selector is immutable, avoid modifying if possible
			deploy.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": ProxyDeploymentName, // FIXME Does this need to be "deployment"?
				},
			}
		}
		// TODO handle selector modified case?

		// Update pod template spec to propagate any changes from Cryostat CR
		deploy.Spec.Template.Spec = *r.getPodSpec()
		// Update pod template metadata
		mergeLabelsAndAnnotations(&deploy.Spec.Template.ObjectMeta, labels, annotations)
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Deployment %s", op), "name", deploy.Name, "namespace", deploy.Namespace)
	return nil
}

func (r *InsightsReconciler) createOrUpdateService(ctx context.Context, svc *corev1.Service, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		// Update labels and annotations
		labels := map[string]string{"app": ProxyDeploymentName}
		annotations := map[string]string{}
		mergeLabelsAndAnnotations(&svc.ObjectMeta, labels, annotations)

		// Set the Cryostat CR as controller
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

// TODO dedup
// ALL capability to drop for restricted pod security. See:
// https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
const capabilityAll corev1.Capability = "ALL"

func (r *InsightsReconciler) getPodSpec() *corev1.PodSpec {
	privEscalation := false
	nonRoot := true
	readOnlyMode := int32(0440)

	return &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  ProxyDeploymentName,
				Image: r.proxyImageTag,
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
					AllowPrivilegeEscalation: &privEscalation,
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{capabilityAll},
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
		Volumes: []corev1.Volume{ // TODO detect change and redeploy
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
		},
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: seccompProfile(true),
		},
	}
}

// TODO dedup or remove
func seccompProfile(openshift bool) *corev1.SeccompProfile {
	// For backward-compatibility with OpenShift < 4.11,
	// leave the seccompProfile empty. In OpenShift >= 4.11,
	// the restricted-v2 SCC will populate it for us.
	if openshift {
		return nil
	}
	return &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}
}

// TODO dedup
func mergeLabelsAndAnnotations(dest *metav1.ObjectMeta, srcLabels, srcAnnotations map[string]string) {
	// Check and create labels/annotations map if absent
	if dest.Labels == nil {
		dest.Labels = map[string]string{}
	}
	if dest.Annotations == nil {
		dest.Annotations = map[string]string{}
	}

	// Merge labels and annotations, preferring those in the source
	for k, v := range srcLabels {
		dest.Labels[k] = v
	}
	for k, v := range srcAnnotations {
		dest.Annotations[k] = v
	}
}
