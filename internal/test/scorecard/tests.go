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

package scorecard

import (
	"context"
	"fmt"
	"time"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	OperatorInstallTestName string        = "operator-install"
	CryostatCRTestName      string        = "cryostat-cr"
	operatorDeploymentName  string        = "cryostat-operator-controller-manager"
	testTimeout             time.Duration = time.Minute * 10
)

// OperatorInstallTest checks that the operator installed correctly
func OperatorInstallTest(bundle *apimanifests.Bundle, namespace string) scapiv1alpha3.TestResult {
	r := scapiv1alpha3.TestResult{}
	r.Name = OperatorInstallTestName
	r.State = scapiv1alpha3.PassState
	r.Errors = make([]string, 0)
	r.Suggestions = make([]string, 0)

	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		return fail(r, fmt.Sprintf("failed to create client: %s", err.Error()))
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = waitForDeploymentAvailability(ctx, client, namespace, operatorDeploymentName, &r)
	if err != nil {
		return fail(r, fmt.Sprintf("operator deployment did not become available: %s", err.Error()))
	}

	return r
}

// CryostatCRTest checks that the operator installs Cryostat in response to a Cryostat CR
func CryostatCRTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) scapiv1alpha3.TestResult {
	r := scapiv1alpha3.TestResult{}
	r.Name = CryostatCRTestName
	r.State = scapiv1alpha3.PassState
	r.Errors = make([]string, 0)
	r.Suggestions = make([]string, 0)

	// Create a new Kubernetes REST client for this test
	client, err := NewClientset()
	if err != nil {
		return fail(r, fmt.Sprintf("failed to create client: %s", err.Error()))
	}
	defer cleanupCryostat(&r, client, namespace)

	openshift, err := isOpenShift(client.DiscoveryClient)
	if err != nil {
		return fail(r, fmt.Sprintf("could not determine whether platform is OpenShift: %s", err.Error()))
	}

	if openshift && openShiftCertManager {
		err := installOpenShiftCertManager(&r)
		if err != nil {
			return fail(r, fmt.Sprintf("failed to install cert-manager Operator for Red Hat OpenShift: %s", err.Error()))
		}
	}

	// Create a default Cryostat CR
	cr := newCryostatCR(namespace, !openshift)

	ctx := context.Background()
	cr, err = client.OperatorCRDs().Cryostats(namespace).Create(ctx, cr)
	if err != nil {
		return fail(r, fmt.Sprintf("failed to create Cryostat CR: %s", err.Error()))
	}

	// Poll the deployment until it becomes available or we timeout
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	err = waitForDeploymentAvailability(ctx, client, cr.Namespace, cr.Name, &r)
	if err != nil {
		return fail(r, fmt.Sprintf("Cryostat main deployment did not become available: %s", err.Error()))
	}

	err = wait.PollImmediateUntilWithContext(ctx, time.Second, func(ctx context.Context) (done bool, err error) {
		cr, err = client.OperatorCRDs().Cryostats(namespace).Get(ctx, cr.Name)
		if err != nil {
			return false, fmt.Errorf("failed to get Cryostat CR: %s", err.Error())
		}
		if len(cr.Status.ApplicationURL) > 0 {
			return true, nil
		}
		r.Log += "Application URL is not yet available\n"
		return false, nil
	})
	if err != nil {
		return fail(r, fmt.Sprintf("Application URL not found in CR: %s", err.Error()))
	}
	r.Log += fmt.Sprintf("Application is ready at %s\n", cr.Status.ApplicationURL)

	return r
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
		logErr := logErrors(r, client, namespace, name)
		if logErr != nil {
			r.Log += fmt.Sprintf("failed to look up deployment errors: %s\n", logErr.Error())
		}
	}
	return err
}

func fail(r scapiv1alpha3.TestResult, message string) scapiv1alpha3.TestResult {
	r.State = scapiv1alpha3.FailState
	r.Errors = append(r.Errors, message)
	return r
}

func cleanupCryostat(r *scapiv1alpha3.TestResult, client *CryostatClientset, namespace string) {
	cr := &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-cr-test",
			Namespace: namespace,
		},
	}
	ctx := context.Background()
	err := client.OperatorCRDs().Cryostats(cr.Namespace).Delete(ctx,
		cr.Name, &metav1.DeleteOptions{})
	if err != nil {
		r.Log += fmt.Sprintf("failed to delete Cryostat: %s\n", err.Error())
	}
}

func logErrors(r *scapiv1alpha3.TestResult, client *CryostatClientset, namespace string, name string) error {
	ctx := context.Background()
	deploy, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
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

func newCryostatCR(namespace string, withIngress bool) *operatorv1beta1.Cryostat {
	cr := &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-cr-test",
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
													Name: "cryostat-cr-test",
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
													Name: "cryostat-cr-test-grafana",
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

func isOpenShift(client discovery.DiscoveryInterface) (bool, error) {
	return discovery.IsResourceEnabled(client, routev1.GroupVersion.WithResource("routes"))
}
