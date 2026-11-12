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
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controller/constants"
	"github.com/cryostatio/cryostat-operator/internal/cryostatapi"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cfg "sigs.k8s.io/controller-runtime/pkg/client/config"
)

// SampleApplicationImage is the bare workload image, set alongside the Scorecard image at build time.
var SampleApplicationImage = "quay.io/redhat-java-monitoring/cryostat-operator-scorecard-app:latest"

// BundledAgentApplicationImage contains the same Agent JAR used for injection.
var BundledAgentApplicationImage = "quay.io/redhat-java-monitoring/cryostat-operator-scorecard-agent-app:latest"

const sampleAgentTLSPath = "/var/run/secrets/scorecard-agent"

// CryostatAgentTest exercises an application with a bundled, manually configured Agent.
func CryostatAgentTest(_ *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	return agentRecordingTest(namespace, openShiftCertManager, false)
}

// CryostatAgentAutoConfigTest exercises Deployment label propagation, Pod injection,
// and recording operations on an application image that contains no Agent.
func CryostatAgentAutoConfigTest(_ *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	return agentRecordingTest(namespace, openShiftCertManager, true)
}

func agentRecordingTest(namespace string, openShiftCertManager, autoConfig bool) *scapiv1alpha3.TestResult {
	name := CryostatAgentTestName
	if autoConfig {
		name = CryostatAgentAutoConfigTestName
	}
	r := newTestResources(name, namespace)
	if err := r.setupCRTestResources(openShiftCertManager); err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s: %s", name, err))
	}
	defer r.cleanupAndLogs()

	cryostat := r.newCryostatCR()
	if !autoConfig {
		// The bundled Agent calls back through its Pod IP, which is allocated after
		// its certificate is issued. Keep CA trust while disabling hostname checks.
		cryostat.Spec.AgentOptions = &operatorv1beta2.AgentOptions{DisableHostnameVerification: true}
	}
	cr, err := r.createAndWaitTillCryostatAvailable(cryostat)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to deploy Cryostat: %s", err))
	}
	defer r.logApplication(r.Name)
	// Exercise the authenticated Service directly. Agent registration and callbacks
	// also stay within the cluster, and do not require an ingress controller.
	base, err := url.Parse(fmt.Sprintf("https://%s.%s.svc:%d", cr.Name, cr.Namespace, constants.AuthProxyHttpContainerPort))
	if err != nil {
		return r.fail(fmt.Sprintf("invalid application URL: %s", err))
	}
	if err := r.waitTillCryostatReady(base); err != nil {
		return r.fail(fmt.Sprintf("Cryostat did not become ready: %s", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	deployment := r.newAgentApplication(cr, autoConfig)
	if !autoConfig {
		if err := r.createBundledAgentTLS(ctx, cr, deployment); err != nil {
			return r.fail(fmt.Sprintf("failed to configure bundled Agent TLS: %s", err))
		}
	}
	deployed, err := r.Client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return r.fail(fmt.Sprintf("failed to deploy sample application: %s", err))
	}
	// Collect application and init-container logs before deleting the CR-owned workload,
	// including when injection or startup fails.
	defer r.logApplication(deployment.Name)
	if autoConfig {
		for _, key := range []string{constants.AgentLabelCryostatName, constants.AgentLabelCryostatNamespace} {
			if deployed.Spec.Template.Labels[key] != deployment.Labels[key] {
				return r.fail(fmt.Sprintf("Deployment webhook did not propagate %s to the Pod template", key))
			}
		}
	}
	if err := r.waitForDeploymentAvailability(ctx, deployment.Name, namespace); err != nil {
		return r.fail(fmt.Sprintf("sample application did not become available: %s", err))
	}
	pods, err := r.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deployment.Name})
	if err != nil {
		return r.fail(fmt.Sprintf("failed to list sample application Pods: %s", err))
	}
	if len(pods.Items) != 1 {
		return r.fail(fmt.Sprintf("expected one sample application Pod, got %d", len(pods.Items)))
	}
	pod := &pods.Items[0]
	if err := verifyAgentApplication(pod, autoConfig); err != nil {
		return r.fail(err.Error())
	}
	apiClient := r.NewCryostatRESTClientset(base)
	target, err := waitForAgentTarget(ctx, apiClient.Targets(), pod.Name)
	if err != nil {
		return r.fail(fmt.Sprintf("Agent did not register with Cryostat: %s", err))
	}
	if !autoConfig {
		expectedCallback := "https://" + net.JoinHostPort(pod.Status.PodIP, fmt.Sprint(constants.AgentCallbackContainerPort))
		if target.ConnectUrl != expectedCallback {
			return r.fail(fmt.Sprintf("expected bundled Agent callback %s, got %s", expectedCallback, target.ConnectUrl))
		}
	}
	r.Log += fmt.Sprintf("Agent registered target: %+v\n", target)
	if err := r.exerciseRecording(ctx, apiClient, target); err != nil {
		return r.fail(err.Error())
	}
	return r.TestResult
}

func (r *TestResources) newAgentApplication(cr *operatorv1beta2.Cryostat, autoConfig bool) *appsv1.Deployment {
	name := r.Name + "-sample"
	nonRoot, no := true, false
	replicas := int32(1)
	image := os.Getenv("CRYOSTAT_SCORECARD_SAMPLE_IMAGE")
	if image == "" {
		image = SampleApplicationImage
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(cr, operatorv1beta2.GroupVersion.WithKind("Cryostat"))},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &no,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &nonRoot,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{{
						Name: "sample", Image: image, ImagePullPolicy: corev1.PullAlways,
						Env:   []corev1.EnvVar{{Name: "JAVA_TOOL_OPTIONS", Value: "-Xmx128m"}},
						Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}},
						ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromString("http")},
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &no,
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
					}},
				},
			},
		},
	}
	if autoConfig {
		// Label the Deployment only: propagation to the template is part of the test.
		deployment.Labels = map[string]string{
			constants.AgentLabelCryostatName:      cr.Name,
			constants.AgentLabelCryostatNamespace: cr.Namespace,
		}
		return deployment
	}
	container := &deployment.Spec.Template.Spec.Containers[0]
	container.Image = os.Getenv("CRYOSTAT_SCORECARD_AGENT_IMAGE")
	if container.Image == "" {
		container.Image = BundledAgentApplicationImage
	}
	container.Env[0].Value += " -javaagent:/opt/cryostat/cryostat-agent-shaded.jar"
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "SCORECARD_POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
		corev1.EnvVar{Name: "SCORECARD_POD_IP", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_APP_NAME", Value: "$(SCORECARD_POD_NAME)"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_BASEURI", Value: fmt.Sprintf("https://%s-agent.%s.svc:%d", cr.Name, cr.Namespace, constants.AgentProxyContainerPort)},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_CALLBACK", Value: fmt.Sprintf("https://$(SCORECARD_POD_IP):%d", constants.AgentCallbackContainerPort)},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_API_WRITES_ENABLED", Value: "true"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_PORT", Value: fmt.Sprint(constants.AgentCallbackContainerPort)},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_CERT_PATH", Value: sampleAgentTLSPath + "/tls.crt"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_KEY_PATH", Value: sampleAgentTLSPath + "/tls.key"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__PATH", Value: sampleAgentTLSPath + "/ca.crt"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__TYPE", Value: "X.509"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__ALIAS", Value: "cryostat"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_FILE", Value: sampleAgentTLSPath + "/tls.crt"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_TYPE", Value: "X.509"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_ALIAS", Value: "cryostat"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_PATH", Value: sampleAgentTLSPath + "/tls.key"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_TYPE", Value: "RSA"},
		corev1.EnvVar{Name: "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_ALIAS", Value: "cryostat"},
	)
	container.Ports = append(container.Ports, corev1.ContainerPort{Name: constants.AgentCallbackPortName, ContainerPort: constants.AgentCallbackContainerPort})
	container.VolumeMounts = []corev1.VolumeMount{{Name: "agent-tls", MountPath: sampleAgentTLSPath, ReadOnly: true}}
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "agent-tls", VolumeSource: corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{SecretName: name},
	}}}
	return deployment
}

func (r *TestResources) createBundledAgentTLS(ctx context.Context, cr *operatorv1beta2.Cryostat, deployment *appsv1.Deployment) error {
	config, err := cfg.GetConfig()
	if err != nil {
		return err
	}
	scheme := runtime.NewScheme()
	if err := certv1.AddToScheme(scheme); err != nil {
		return err
	}
	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	certificate := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: deployment.Name, Namespace: r.Namespace, OwnerReferences: deployment.OwnerReferences},
		Spec: certv1.CertificateSpec{
			SecretName: deployment.Name,
			DNSNames:   []string{deployment.Name},
			IssuerRef:  certmeta.IssuerReference{Name: cr.Name + "-ca", Kind: "Issuer"},
			PrivateKey: &certv1.CertificatePrivateKey{Algorithm: certv1.RSAKeyAlgorithm},
			Usages:     []certv1.KeyUsage{certv1.UsageDigitalSignature, certv1.UsageKeyEncipherment, certv1.UsageServerAuth, certv1.UsageClientAuth},
		},
	}
	if err := c.Create(ctx, certificate); err != nil {
		return err
	}
	// cert-manager does not always add an owner reference to issued Secrets.
	// Attach the CR so individual test runs also garbage-collect this TLS Secret.
	if err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		secret, err := r.Client.CoreV1().Secrets(r.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if len(secret.Data[corev1.TLSCertKey]) == 0 || len(secret.Data[corev1.TLSPrivateKeyKey]) == 0 || len(secret.Data[constants.CAKey]) == 0 {
			return false, nil
		}
		owner := *metav1.NewControllerRef(cr, operatorv1beta2.GroupVersion.WithKind("Cryostat"))
		owner.Controller = nil
		patch, err := json.Marshal(map[string]any{"metadata": map[string]any{
			"ownerReferences": append(secret.OwnerReferences, owner),
		}})
		if err != nil {
			return false, err
		}
		_, err = r.Client.CoreV1().Secrets(r.Namespace).Patch(ctx, secret.Name, types.MergePatchType, patch, metav1.PatchOptions{})
		return err == nil, err
	}); err != nil {
		return fmt.Errorf("waiting for sample Agent certificate: %w", err)
	}
	return nil
}

// Agent targets must be discovered through registration, never synthesized through
// POST /targets or selected from another workload's JMX endpoint.
func waitForAgentTarget(ctx context.Context, c *TargetClient, podName string) (*cryostatapi.Target, error) {
	var found *cryostatapi.Target
	var lastErr error
	err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		targets, err := c.List(ctx)
		if err != nil {
			lastErr = err
			return false, nil
		}
		for _, target := range targets {
			callback, err := url.Parse(target.ConnectUrl)
			if err == nil && target.Alias == podName && target.Id != 0 &&
				(callback.Scheme == "https" || callback.Scheme == "http") {
				found = &target
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("waiting for HTTP(S) target for Pod %s: %w (last API error: %v)", podName, err, lastErr)
	}
	return found, nil
}

func verifyAgentApplication(pod *corev1.Pod, autoConfig bool) error {
	injected := false
	for _, container := range pod.Spec.InitContainers {
		if container.Name == "cryostat-agent-init" {
			injected = true
		}
	}
	if injected != autoConfig {
		return fmt.Errorf("pod %s: expected Agent injection=%t, got %t", pod.Name, autoConfig, injected)
	}
	for _, container := range pod.Spec.Containers {
		if container.Name != "sample" {
			continue
		}
		for _, env := range container.Env {
			if env.Name == "JAVA_TOOL_OPTIONS" && strings.Contains(env.Value, "-Xmx128m") &&
				strings.Contains(env.Value, "-javaagent:") {
				return nil
			}
		}
	}
	return fmt.Errorf("pod %s is missing Agent JVM options or the application's original JVM options", pod.Name)
}

func (r *TestResources) logApplication(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pods, err := r.Client.CoreV1().Pods(r.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + name})
	if err != nil {
		r.Log += fmt.Sprintf("failed to list application Pods for logs: %s\n", err)
		return
	}
	for _, pod := range pods.Items {
		for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
			logs, err := r.Client.CoreV1().Pods(r.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).DoRaw(ctx)
			r.Log += fmt.Sprintf("\nApplication %s/%s logs:\n%s\n", pod.Name, container.Name, logs)
			if err != nil {
				r.Log += fmt.Sprintf("failed to retrieve logs: %s\n", err)
			}
		}
	}
}
