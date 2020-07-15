// Copyright (c) 2020 Red Hat, Inc.
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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("common_reconciler")

// CommonReconciler contains helpful methods to communicate with Container JFR.
// It is meant to be embedded within other Reconcilers.
type CommonReconciler struct {
	// This client, initialized using mgr.Client(), is a split client
	// that reads objects from the cache and writes to the apiserver
	Client    client.Client
	JfrClient *jfrclient.ContainerJfrClient
}

// FindContainerJFR retrieves a ContainerJFR instance within a given namespace
func (r *CommonReconciler) FindContainerJFR(ctx context.Context, namespace string) (*rhjmcv1alpha1.ContainerJFR, error) {
	// TODO Consider how to find ContainerJFR object if this operator becomes cluster-scoped
	// Look up the ContainerJFR object for this operator, which will help us find its services
	cjfrList := &rhjmcv1alpha1.ContainerJFRList{}
	err := r.Client.List(ctx, cjfrList)
	if err != nil {
		return nil, err
	}
	if len(cjfrList.Items) == 0 {
		return nil, errors.New("No ContainerJFR objects found")
	} else if len(cjfrList.Items) > 1 {
		// Does not seem like a proper use-case
		log.Info("More than one ContainerJFR object found in namespace, using only the first one listed",
			"namespace", namespace)
	}
	return &cjfrList.Items[0], nil
}

// ConnectToContainerJFR opens a WebSocket connect to the Container JFR service deployed
// by this operator
func (r *CommonReconciler) ConnectToContainerJFR(ctx context.Context, namespace string,
	svcName string) (*jfrclient.ContainerJfrClient, error) {
	// Query the "clienturl" endpoint of Container JFR for the command URL
	clientURL, err := r.getClientURL(ctx, namespace, svcName)
	if err != nil {
		return nil, err
	}
	tok, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, err
	}
	strTok := string(tok)
	config := &jfrclient.Config{ServerURL: clientURL, AccessToken: &strTok, TLSVerify: !strings.EqualFold(os.Getenv("TLS_VERIFY"), "false")}
	jfrClient, err := jfrclient.Create(config)
	if err != nil {
		return nil, err
	}
	return jfrClient, nil
}

// GetPodTarget returns a TargetAddress for a particular pod and port number
func (r *CommonReconciler) GetPodTarget(targetPod *corev1.Pod, jmxPort int32) (*jfrclient.TargetAddress, error) {
	// Create TargetAddress using pod's IP address and provided port
	podIP, err := getPodIP(targetPod)
	if err != nil {
		return nil, err
	}
	return &jfrclient.TargetAddress{
		Host: *podIP,
		Port: jmxPort,
	}, nil
}

// CloseClient closes the underlying WebSocket connection to Container JFR
func (r *CommonReconciler) CloseClient() {
	r.JfrClient.Close()
	r.JfrClient = nil
}

func (r *CommonReconciler) getClientURL(ctx context.Context, namespace string, svcName string) (*url.URL, error) {
	// Look up Container JFR service, and query "clienturl" endpoint
	cjfrSvc := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: svcName}, cjfrSvc)
	if err != nil {
		return nil, err
	}
	clusterIP, err := getClusterIP(cjfrSvc)
	if err != nil {
		return nil, err
	}
	webServerPort, err := getWebServerPort(cjfrSvc)
	if err != nil {
		return nil, err
	}
	host := fmt.Sprintf("http://%s:%d/api/v1/clienturl", *clusterIP, webServerPort)
	resp, err := http.Get(host)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Find "clientUrl" JSON property in repsonse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	clientURLHolder := struct {
		ClientURL string `json:"clientUrl"`
	}{}
	err = json.Unmarshal(body, &clientURLHolder)
	if err != nil {
		return nil, err
	}
	return url.Parse(clientURLHolder.ClientURL)
}

func getPodIP(pod *corev1.Pod) (*string, error) {
	podIP := pod.Status.PodIP
	if len(podIP) == 0 {
		return nil, fmt.Errorf("PodIP unavailable for %s", pod.Name)
	}
	return &podIP, nil
}

func getClusterIP(svc *corev1.Service) (*string, error) {
	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == corev1.ClusterIPNone {
		return nil, fmt.Errorf("ClusterIP unavailable for %s", svc.Name)
	}
	return &clusterIP, nil
}

func getWebServerPort(svc *corev1.Service) (int32, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == "export" {
			return port.Port, nil
		}
	}
	return 0, errors.New("ContainerJFR service had no port named \"export\"")
}
