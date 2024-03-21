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

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	operatorDeploymentName string        = "cryostat-operator-controller-manager"
	testTimeout            time.Duration = time.Minute * 10
)

type TestResources struct {
	OpenShift bool
	Client    *CryostatClientset
	*scapiv1alpha3.TestResult
}

func waitForDeploymentAvailability(ctx context.Context, client *CryostatClientset, namespace string,
	name string, r *scapiv1alpha3.TestResult) error {
	err := wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
		logErr := logWorkloadEvents(r, client, namespace, name)
		if logErr != nil {
			r.Log += fmt.Sprintf("failed to look up deployment errors: %s\n", logErr.Error())
		}
	}
	return err
}

func logError(r *scapiv1alpha3.TestResult, message string) {
	r.State = scapiv1alpha3.FailState
	r.Errors = append(r.Errors, message)
}

func fail(r scapiv1alpha3.TestResult, message string) scapiv1alpha3.TestResult {
	r.State = scapiv1alpha3.FailState
	r.Errors = append(r.Errors, message)
	return r
}

func logWorkloadEvents(r *scapiv1alpha3.TestResult, client *CryostatClientset, namespace string, name string) error {
	ctx := context.Background()
	deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
	err = logEvents(r, client, namespace, scheme.Scheme, deploy)
	if err != nil {
		return err
	}

	// Look up replica sets for deployment and log conditions and events
	selector, err := metav1.LabelSelectorAsSelector(deploy.Spec.Selector)
	if err != nil {
		return err
	}
	replicaSets, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
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
		err = logEvents(r, client, namespace, scheme.Scheme, &rs)
		if err != nil {
			return err
		}
	}

	// Look up pods for deployment and log conditions and events
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
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
		err = logEvents(r, client, namespace, scheme.Scheme, &pod)
		if err != nil {
			return err
		}
	}
	return nil
}

func logEvents(r *scapiv1alpha3.TestResult, client *CryostatClientset, namespace string,
	scheme *runtime.Scheme, obj runtime.Object) error {
	events, err := client.CoreV1().Events(namespace).Search(scheme, obj)
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

func LogWorkloadEventsOnError(r *scapiv1alpha3.TestResult, client *CryostatClientset, namespace string, name string) {
	if len(r.Errors) > 0 {
		r.Log += "\nWORKLOAD EVENTS:\n"
		for _, deployName := range []string{name, name + "-reports"} {
			logErr := logWorkloadEvents(r, client, namespace, deployName)
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

func newTestResources(testName string) *TestResources {
	return &TestResources{
		TestResult: newEmptyTestResult(testName),
	}
}

func setupCRTestResources(tr *TestResources, openShiftCertManager bool) error {
	r := tr.TestResult

	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		logError(r, fmt.Sprintf("failed to create client: %s", err.Error()))
		return err
	}
	tr.Client = client

	openshift, err := isOpenShift(client)
	if err != nil {
		logError(r, fmt.Sprintf("could not determine whether platform is OpenShift: %s", err.Error()))
		return err
	}
	tr.OpenShift = openshift

	if openshift && openShiftCertManager {
		err := installOpenShiftCertManager(r)
		if err != nil {
			logError(r, fmt.Sprintf("failed to install cert-manager Operator for Red Hat OpenShift: %s", err.Error()))
			return err
		}
	}
	return nil
}

func newCryostatCR(name string, namespace string, withIngress bool) *operatorv1beta1.Cryostat {
	cr := &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: operatorv1beta1.CryostatSpec{
			Minimal:           false,
			EnableCertManager: &[]bool{true}[0],
		},
	}

	if withIngress {
		pathType := netv1.PathTypePrefix
		cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
			CoreConfig: &operatorv1beta1.NetworkConfiguration{
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
			GrafanaConfig: &operatorv1beta1.NetworkConfiguration{
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
	return cr
}

func createAndWaitTillCryostatAvailable(cr *operatorv1beta1.Cryostat, resources *TestResources) (*operatorv1beta1.Cryostat, error) {
	client := resources.Client
	r := resources.TestResult

	cr, err := client.OperatorCRDs().Cryostats(cr.Namespace).Create(context.Background(), cr)
	if err != nil {
		logError(r, fmt.Sprintf("failed to create Cryostat CR: %s", err.Error()))
		return nil, err
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = waitForDeploymentAvailability(ctx, client, cr.Namespace, cr.Name, r)
	if err != nil {
		logError(r, fmt.Sprintf("Cryostat main deployment did not become available: %s", err.Error()))
		return nil, err
	}

	err = wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		cr, err = client.OperatorCRDs().Cryostats(cr.Namespace).Get(ctx, cr.Name)
		if err != nil {
			return false, fmt.Errorf("failed to get Cryostat CR: %s", err.Error())
		}
		if len(cr.Status.ApplicationURL) > 0 {
			return true, nil
		}
		r.Log += "application URL is not yet available\n"
		return false, nil
	})
	if err != nil {
		logError(r, fmt.Sprintf("application URL not found in CR: %s", err.Error()))
		return nil, err
	}
	r.Log += fmt.Sprintf("application is available at %s\n", cr.Status.ApplicationURL)

	return cr, nil
}

func waitTillCryostatReady(base *url.URL, resources *TestResources) error {
	return sendHealthRequest(base, resources, func(resp *http.Response, r *scapiv1alpha3.TestResult) (done bool, err error) {
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

func waitTillReportReady(name string, namespace string, port int32, resources *TestResources) error {
	client := resources.Client
	r := resources.TestResult

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := waitForDeploymentAvailability(ctx, client, namespace, name, r)
	if err != nil {
		return fmt.Errorf("report sidecar deployment did not become available: %s", err.Error())
	}

	reportsUrl := fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", name, namespace, port)
	base, err := url.Parse(reportsUrl)
	if err != nil {
		return fmt.Errorf("application URL is invalid: %s", err.Error())
	}

	return sendHealthRequest(base, resources, func(resp *http.Response, r *scapiv1alpha3.TestResult) (done bool, err error) {
		r.Log += fmt.Sprintf("reports sidecar is ready at %s\n", base.String())
		return true, nil
	})
}

func sendHealthRequest(base *url.URL, resources *TestResources, healthCheck func(resp *http.Response, r *scapiv1alpha3.TestResult) (done bool, err error)) error {
	client := NewHttpClient()
	r := resources.TestResult

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
		return healthCheck(resp, r)
	})
	return err
}

func updateAndWaitTillCryostatAvailable(cr *operatorv1beta1.Cryostat, resources *TestResources) error {
	client := resources.Client
	r := resources.TestResult

	cr, err := client.OperatorCRDs().Cryostats(cr.Namespace).Update(context.Background(), cr)
	if err != nil {
		r.Log += fmt.Sprintf("failed to update Cryostat CR: %s", err.Error())
		return err
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		deploy, err := client.AppsV1().Deployments(cr.Namespace).Get(ctx, cr.Name, metav1.GetOptions{})
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

func cleanupAndLogs(r *scapiv1alpha3.TestResult, client *CryostatClientset, name string, namespace string, logChannel *chan *ContainerLog) {
	LogWorkloadEventsOnError(r, client, namespace, name)

	cr := &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	ctx := context.Background()
	err := client.OperatorCRDs().Cryostats(cr.Namespace).Delete(ctx, cr.Name, &metav1.DeleteOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete Cryostat: %s\n", err.Error())
		}
	}

	if logChannel != nil {
		CollectContainersLogsToResult(r, *logChannel)
	}
}

func getCryostatPodNameForCR(clientset *kubernetes.Clientset, cr *operatorv1beta1.Cryostat) (string, error) {
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       cr.Name,
			"component": "cryostat",
		},
	}
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(selector.MatchLabels).String(),
	}

	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()

	pods, err := clientset.CoreV1().Pods(cr.Namespace).List(ctx, opts)
	if err != nil {
		return "", err
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no matching cryostat pods for cr: %s", cr.Name)
	}

	return pods.Items[0].ObjectMeta.Name, nil
}
