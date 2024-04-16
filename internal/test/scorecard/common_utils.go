// Copyright The Cryostat Authors.
// Copyright 2016 The Kubernetes Authors.
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	operatorDeploymentName string        = "cryostat-operator-controller-manager"
	testTimeout            time.Duration = time.Minute * 10
)

type TestResources struct {
	Name             string
	Namespace        string
	TargetNamespaces []string
	OpenShift        bool
	Client           *CryostatClientset
	LogChannel       chan *ContainerLog
	*scapiv1alpha3.TestResult
}

func (r *TestResources) waitForDeploymentAvailability(ctx context.Context, name string, namespace string) error {
	err := wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		deploy, err := r.Client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("deployment %s is not yet found\n", name)
				return false, nil // Retry
			}
			return false, fmt.Errorf("failed to get deployment: %s", err.Error())
		}
		// Check for Available condition
		for _, condition := range deploy.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable &&
				condition.Status == corev1.ConditionTrue {
				r.Log += fmt.Sprintf("deployment %s is available\n", deploy.Name)
				return true, nil
			}
			if condition.Type == appsv1.DeploymentReplicaFailure &&
				condition.Status == corev1.ConditionTrue {
				r.Log += fmt.Sprintf("deployment %s is failing, %s: %s\n", deploy.Name,
					condition.Reason, condition.Message)
			}
		}
		r.Log += fmt.Sprintf("deployment %s is not yet available\n", deploy.Name)
		return false, nil
	})
	if err != nil {
		logErr := r.logWorkloadEvents(r.Name)
		if logErr != nil {
			r.Log += fmt.Sprintf("failed to look up deployment errors: %s\n", logErr.Error())
		}
	}
	return err
}

func (r *TestResources) logError(message string) {
	r.State = scapiv1alpha3.FailState
	r.Errors = append(r.Errors, message)
}

func (r *TestResources) fail(message string) *scapiv1alpha3.TestResult {
	r.State = scapiv1alpha3.FailState
	r.Errors = append(r.Errors, message)
	return r.TestResult
}

func (r *TestResources) logWorkloadEvents(name string) error {
	ctx := context.Background()
	deploy, err := r.Client.AppsV1().Deployments(r.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	// Log deployment conditions and events
	r.Log += fmt.Sprintf("deployment %s conditions:\n", deploy.Name)
	for _, condition := range deploy.Status.Conditions {
		r.Log += fmt.Sprintf("\t%s == %s, %s: %s\n", condition.Type,
			condition.Status, condition.Reason, condition.Message)
	}

	r.Log += fmt.Sprintf("deployment %s warning events:\n", deploy.Name)
	err = r.logEvents(scheme.Scheme, deploy)
	if err != nil {
		return err
	}

	// Look up replica sets for deployment and log conditions and events
	selector, err := metav1.LabelSelectorAsSelector(deploy.Spec.Selector)
	if err != nil {
		return err
	}
	replicaSets, err := r.Client.AppsV1().ReplicaSets(r.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return err
	}
	for _, rs := range replicaSets.Items {
		r.Log += fmt.Sprintf("replica set %s conditions:\n", rs.Name)
		for _, condition := range rs.Status.Conditions {
			r.Log += fmt.Sprintf("\t%s == %s, %s: %s\n", condition.Type, condition.Status,
				condition.Reason, condition.Message)
		}
		r.Log += fmt.Sprintf("replica set %s warning events:\n", rs.Name)
		err = r.logEvents(scheme.Scheme, &rs)
		if err != nil {
			return err
		}
	}

	// Look up pods for deployment and log conditions and events
	pods, err := r.Client.CoreV1().Pods(r.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		r.Log += fmt.Sprintf("pod %s phase: %s\n", pod.Name, pod.Status.Phase)
		r.Log += fmt.Sprintf("pod %s conditions:\n", pod.Name)
		for _, condition := range pod.Status.Conditions {
			r.Log += fmt.Sprintf("\t%s == %s, %s: %s\n", condition.Type, condition.Status,
				condition.Reason, condition.Message)
		}
		r.Log += fmt.Sprintf("pod %s warning events:\n", pod.Name)
		err = r.logEvents(scheme.Scheme, &pod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *TestResources) logEvents(scheme *runtime.Scheme, obj runtime.Object) error {
	events, err := r.Client.CoreV1().Events(r.Namespace).Search(scheme, obj)
	if err != nil {
		return err
	}
	for _, event := range events.Items {
		if event.Type == corev1.EventTypeWarning {
			r.Log += fmt.Sprintf("\t%s: %s\n", event.Reason, event.Message)
		}
	}
	return nil
}

func (r *TestResources) LogWorkloadEventsOnError() {
	if len(r.Errors) > 0 {
		r.Log += "\nWORKLOAD EVENTS:\n"
		for _, deployName := range []string{r.Name, r.Name + "-reports"} {
			logErr := r.logWorkloadEvents(deployName)
			if logErr != nil {
				r.Log += fmt.Sprintf("failed to get workload logs: %s", logErr)
			}
		}
	}
}

func newEmptyTestResult(testName string) *scapiv1alpha3.TestResult {
	return &scapiv1alpha3.TestResult{
		Name:        testName,
		State:       scapiv1alpha3.PassState,
		Errors:      make([]string, 0),
		Suggestions: make([]string, 0),
	}
}

func newTestResources(testName string, namespace string) *TestResources {
	return &TestResources{
		Name:       testName,
		Namespace:  namespace,
		TestResult: newEmptyTestResult(testName),
	}
}

func (r *TestResources) setupCRTestResources(openShiftCertManager bool) error {
	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		r.logError(fmt.Sprintf("failed to create client: %s", err.Error()))
		return err
	}
	r.Client = client

	openshift, err := isOpenShift(client)
	if err != nil {
		r.logError(fmt.Sprintf("could not determine whether platform is OpenShift: %s", err.Error()))
		return err
	}
	r.OpenShift = openshift

	if openshift && openShiftCertManager {
		err := r.installOpenShiftCertManager()
		if err != nil {
			r.logError(fmt.Sprintf("failed to install cert-manager Operator for Red Hat OpenShift: %s", err.Error()))
			return err
		}
	}
	return nil
}

func (r *TestResources) setupTargetNamespace() error {
	ctx := context.Background()

	for _, namespaceName := range r.TargetNamespaces {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		ns, err := r.Client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create namespace %s: %s", namespaceName, err.Error())
		}
		r.Log += fmt.Sprintf("created namespace: %s\n", ns.Name)
	}
	return nil
}

func (r *TestResources) newCryostatCR() *operatorv1beta2.Cryostat {
	cr := &operatorv1beta2.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: operatorv1beta2.CryostatSpec{
			EnableCertManager: &[]bool{true}[0],
		},
	}
	if !r.OpenShift {
		configureIngress(cr.Name, &cr.Spec)
	}

	return cr
}

func (r *TestResources) newMultiNamespaceCryostatCR() *operatorv1beta2.Cryostat {
	cr := &operatorv1beta2.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: operatorv1beta2.CryostatSpec{
			TargetNamespaces:  r.TargetNamespaces,
			EnableCertManager: &[]bool{true}[0],
		},
	}
	if !r.OpenShift {
		configureIngress(cr.Name, &cr.Spec)
	}

	return cr
}

func configureIngress(name string, cryostatSpec *operatorv1beta2.CryostatSpec) {
	pathType := netv1.PathTypePrefix
	cryostatSpec.NetworkOptions = &operatorv1beta2.NetworkConfigurationList{
		CoreConfig: &operatorv1beta2.NetworkConfiguration{
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
			},
			IngressSpec: &netv1.IngressSpec{
				TLS: []netv1.IngressTLS{{}},
				Rules: []netv1.IngressRule{
					{
						Host: "testing.cryostat",
						IngressRuleValue: netv1.IngressRuleValue{
							HTTP: &netv1.HTTPIngressRuleValue{
								Paths: []netv1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: &pathType,
										Backend: netv1.IngressBackend{
											Service: &netv1.IngressServiceBackend{
												Name: name,
												Port: netv1.ServiceBackendPort{
													Number: 8181,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		GrafanaConfig: &operatorv1beta2.NetworkConfiguration{
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
			},
			IngressSpec: &netv1.IngressSpec{
				TLS: []netv1.IngressTLS{{}},
				Rules: []netv1.IngressRule{
					{
						Host: "testing.cryostat-grafana",
						IngressRuleValue: netv1.IngressRuleValue{
							HTTP: &netv1.HTTPIngressRuleValue{
								Paths: []netv1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: &pathType,
										Backend: netv1.IngressBackend{
											Service: &netv1.IngressServiceBackend{
												Name: fmt.Sprintf("%s-grafana", name),
												Port: netv1.ServiceBackendPort{
													Number: 3000,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *TestResources) createAndWaitTillCryostatAvailable(cr *operatorv1beta2.Cryostat) (*operatorv1beta2.Cryostat, error) {
	cr, err := r.Client.OperatorCRDs().Cryostats(cr.Namespace).Create(context.Background(), cr)
	if err != nil {
		r.logError(fmt.Sprintf("failed to create Cryostat CR: %s", err.Error()))
		return nil, err
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = r.waitForDeploymentAvailability(ctx, r.Name, r.Namespace)
	if err != nil {
		r.logError(fmt.Sprintf("Cryostat main deployment did not become available: %s", err.Error()))
		return nil, err
	}

	err = wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		cr, err = r.Client.OperatorCRDs().Cryostats(cr.Namespace).Get(ctx, cr.Name)
		if err != nil {
			return false, fmt.Errorf("failed to get Cryostat CR: %s", err.Error())
		}
		if len(cr.Spec.TargetNamespaces) > 0 {
			if len(cr.Status.TargetNamespaces) == 0 {
				r.Log += "application's target namespaces are not available"
				return false, nil // Retry
			}
			for i := range cr.Status.TargetNamespaces {
				if cr.Status.TargetNamespaces[i] != cr.Spec.TargetNamespaces[i] {
					return false, fmt.Errorf("application's target namespaces do not correctly match CR's")
				}
			}
		}
		if len(cr.Status.ApplicationURL) == 0 {
			r.Log += "application URL is not yet available\n"
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		r.logError(fmt.Sprintf("application URL not found in CR: %s", err.Error()))
		return nil, err
	}
	r.Log += fmt.Sprintf("application has access to the following namespaces: %s\n", cr.Status.TargetNamespaces)
	r.Log += fmt.Sprintf("application is available at %s\n", cr.Status.ApplicationURL)

	return cr, nil
}

func (r *TestResources) waitTillCryostatReady(base *url.URL) error {
	return r.sendHealthRequest(base, func(resp *http.Response, result *scapiv1alpha3.TestResult) (done bool, err error) {
		health := &HealthResponse{}
		err = ReadJSON(resp, health)
		if err != nil {
			return false, fmt.Errorf("failed to read response body: %s", err.Error())
		}

		if err = health.Ready(); err != nil {
			r.Log += fmt.Sprintf("application is not yet ready: %s\n", err.Error())
			return false, nil // Try again
		}

		r.Log += fmt.Sprintf("application is ready at %s\n", base.String())
		return true, nil
	})
}

func (r *TestResources) waitTillReportReady(port int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := r.waitForDeploymentAvailability(ctx, r.Name+"-reports", r.Namespace)
	if err != nil {
		return fmt.Errorf("report sidecar deployment did not become available: %s", err.Error())
	}

	reportsUrl := fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", r.Name+"-reports", r.Namespace, port)
	base, err := url.Parse(reportsUrl)
	if err != nil {
		return fmt.Errorf("application URL is invalid: %s", err.Error())
	}

	return r.sendHealthRequest(base, func(resp *http.Response, result *scapiv1alpha3.TestResult) (done bool, err error) {
		r.Log += fmt.Sprintf("reports sidecar is ready at %s\n", base.String())
		return true, nil
	})
}

func (r *TestResources) sendHealthRequest(base *url.URL, healthCheck func(resp *http.Response, result *scapiv1alpha3.TestResult) (done bool, err error)) error {
	client := NewHttpClient()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		url := base.JoinPath("/health")
		req, err := NewHttpRequest(ctx, http.MethodGet, url.String(), nil, make(http.Header))
		if err != nil {
			return false, fmt.Errorf("failed to create a an http request: %s", err.Error())
		}
		req.Header.Add("Accept", "*/*")

		resp, err := client.Do(req)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil // Retry
			}
			return false, err
		}
		defer resp.Body.Close()

		if !StatusOK(resp.StatusCode) {
			if resp.StatusCode == http.StatusServiceUnavailable {
				r.Log += fmt.Sprintf("application is not yet reachable at %s\n", base.String())
				return false, nil // Try again
			}
			return false, fmt.Errorf("API request failed with status code %d: %s", resp.StatusCode, ReadError(resp))
		}
		return healthCheck(resp, r.TestResult)
	})
	return err
}

func (r *TestResources) updateAndWaitTillCryostatAvailable(cr *operatorv1beta2.Cryostat) error {
	cr, err := r.Client.OperatorCRDs().Cryostats(cr.Namespace).Update(context.Background(), cr)
	if err != nil {
		r.Log += fmt.Sprintf("failed to update Cryostat CR: %s", err.Error())
		return err
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		deploy, err := r.Client.AppsV1().Deployments(cr.Namespace).Get(ctx, cr.Name, metav1.GetOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("deployment %s is not yet found\n", cr.Name)
				return false, nil // Retry
			}
			return false, fmt.Errorf("failed to get deployment: %s", err.Error())
		}

		// Wait for deployment to update by verifying Cryostat has PVC configured
		for _, volume := range deploy.Spec.Template.Spec.Volumes {
			if volume.VolumeSource.EmptyDir != nil {
				r.Log += fmt.Sprintf("Cryostat deployment is still updating. Storage: %s\n", volume.VolumeSource.EmptyDir)
				return false, nil // Retry
			}
			if volume.VolumeSource.PersistentVolumeClaim != nil {
				break
			}
		}

		// Derived from kubectl: https://github.com/kubernetes/kubectl/blob/24d21a0/pkg/polymorphichelpers/rollout_status.go#L75-L91
		// Check for deployment condition
		if deploy.Generation <= deploy.Status.ObservedGeneration {
			for _, condition := range deploy.Status.Conditions {
				if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse && condition.Reason == "ProgressDeadlineExceeded" {
					return false, fmt.Errorf("deployment %s exceeded its progress deadline", deploy.Name) // Don't Retry
				}
			}
			if deploy.Spec.Replicas != nil && deploy.Status.UpdatedReplicas < *deploy.Spec.Replicas {
				r.Log += fmt.Sprintf("Waiting for deployment %s rollout to finish: %d out of %d new replicas have been updated... \n", deploy.Name, deploy.Status.UpdatedReplicas, *deploy.Spec.Replicas)
				return false, nil
			}
			if deploy.Status.Replicas > deploy.Status.UpdatedReplicas {
				r.Log += fmt.Sprintf("Waiting for deployment %s rollout to finish: %d old replicas are pending termination... \n", deploy.Name, deploy.Status.Replicas-deploy.Status.UpdatedReplicas)
				return false, nil
			}
			if deploy.Status.AvailableReplicas < deploy.Status.UpdatedReplicas {
				r.Log += fmt.Sprintf("Waiting for deployment %s rollout to finish: %d out of %d updated replicas are available... \n", deploy.Name, deploy.Status.AvailableReplicas, deploy.Status.UpdatedReplicas)
				return false, nil
			}
			r.Log += fmt.Sprintf("deployment %s successfully rolled out\n", deploy.Name)
			return true, nil
		}
		r.Log += "Waiting for deployment spec update to be observed...\n"
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to look up deployment errors: %s", err.Error())
	}
	return err
}

func (r *TestResources) cleanupAndLogs() {
	r.LogWorkloadEventsOnError()

	ctx := context.Background()
	err := r.Client.OperatorCRDs().Cryostats(r.Namespace).Delete(ctx, r.Name, &metav1.DeleteOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete Cryostat: %s\n", err.Error())
		}
	}

	for _, namespaceName := range r.TargetNamespaces {
		err := r.Client.CoreV1().Namespaces().Delete(ctx, namespaceName, metav1.DeleteOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete namespace %s: %s", namespaceName, err.Error())
			}
		}
	}

	if r.LogChannel != nil {
		r.CollectContainersLogsToResult()
	}
}

func (r *TestResources) getCryostatPodNameForCR(cr *operatorv1beta2.Cryostat) (string, error) {
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       cr.Name,
			"kind":      "cryostat",
			"component": "cryostat",
		},
	}

	names, err := r.getPodnamesForSelector(cr.Namespace, selector)
	if err != nil {
		return "", err
	}

	if len(names) == 0 {
		return "", fmt.Errorf("no matching cryostat pods for cr: %s", cr.Name)
	}
	return names[0].ObjectMeta.Name, nil
}

func (r *TestResources) getReportPodNameForCR(cr *operatorv1beta1.Cryostat) (string, error) {
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       cr.Name,
			"kind":      "cryostat",
			"component": "reports",
		},
	}

	names, err := r.getPodnamesForSelector(cr.Namespace, selector)
	if err != nil {
		return "", err
	}

	if len(names) == 0 {
		return "", fmt.Errorf("no matching report sidecar pods for cr: %s", cr.Name)
	}
	return names[0].ObjectMeta.Name, nil
}

func (r *TestResources) getPodnamesForSelector(namespace string, selector metav1.LabelSelector) ([]corev1.Pod, error) {
	labelSelector := labels.Set(selector.MatchLabels).String()

	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()

	pods, err := r.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	return pods.Items, err
}
