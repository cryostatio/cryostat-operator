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

package common

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/tls"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("common_reconciler")

// ReconcilerConfig contains configuration used to customize a Reconciler
// built with NewReconciler
type ReconcilerConfig struct {
	// This client, initialized using mgr.Client(), is a split client
	// that reads objects from the cache and writes to the apiserver
	Client        client.Client
	ClientFactory ContainerJFRClientFactory
	OS            OSUtils
}

// Reconciler contains helpful methods to communicate with Container JFR.
// It is meant to be embedded within other Reconcilers.
type Reconciler interface {
	FindContainerJFR(ctx context.Context, namespace string) (*rhjmcv1alpha1.ContainerJFR, error)
	GetContainerJFRClient(ctx context.Context, namespace string) (jfrclient.ContainerJfrClient, error)
	GetPodTarget(targetPod *corev1.Pod, jmxPort int32) (*jfrclient.TargetAddress, error)
	tls.ReconcilerTLS
}

type commonReconciler struct {
	*ReconcilerConfig
	tls.ReconcilerTLS
}

// blank assignment to verify that commonReconciler implements Reconciler
var _ Reconciler = &commonReconciler{}

// NewReconciler creates a new Reconciler using the provided configuration
func NewReconciler(config *ReconcilerConfig) Reconciler {
	configCopy := *config
	if config.ClientFactory == nil {
		configCopy.ClientFactory = &defaultClientFactory{}
	}
	if config.OS == nil {
		configCopy.OS = &defaultOSUtils{}
	}
	return &commonReconciler{
		ReconcilerConfig: &configCopy,
		ReconcilerTLS: tls.NewReconciler(&tls.ReconcilerTLSConfig{
			Client: configCopy.Client,
		}),
	}
}

// GetContainerJFRClient creates a client to communicate with the Container JFR
// instance deployed by this operator in the given namespace
func (r *commonReconciler) GetContainerJFRClient(ctx context.Context, namespace string) (jfrclient.ContainerJfrClient, error) {
	// Look up ContainerJFR instance within the given namespace
	cjfr, err := r.FindContainerJFR(ctx, namespace)
	if err != nil {
		return nil, err
	}
	// Get CA certificate if TLS is enabled
	var caCert []byte
	protocol := "http"
	if r.IsCertManagerEnabled() {
		caCert, err = r.GetContainerJFRCABytes(ctx, cjfr)
		if err != nil {
			return nil, err
		}
		protocol = "https"
	}
	// Get the URL to the Container JFR web service
	serverURL, err := r.getServerURL(ctx, cjfr.Namespace, cjfr.Name, protocol)
	if err != nil {
		return nil, err
	}
	// Read bearer token from mounted secret
	tok, err := r.OS.GetFileContents("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, err
	}
	strTok := string(tok)

	// Create Container JFR HTTP(S) client
	config := &jfrclient.Config{ServerURL: serverURL, AccessToken: &strTok, CACertificate: caCert}
	jfrClient, err := r.ClientFactory.CreateClient(config)
	if err != nil {
		return nil, err
	}
	return jfrClient, nil
}

// GetPodTarget returns a TargetAddress for a particular pod and port number
func (r *commonReconciler) GetPodTarget(targetPod *corev1.Pod, jmxPort int32) (*jfrclient.TargetAddress, error) {
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

func (r *commonReconciler) FindContainerJFR(ctx context.Context, namespace string) (*rhjmcv1alpha1.ContainerJFR, error) {
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

func (r *commonReconciler) getServerURL(ctx context.Context, namespace string, svcName string, protocol string) (*url.URL, error) {
	// Look up Container JFR service, and build URL to web service
	cjfrSvc := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: svcName}, cjfrSvc)
	if err != nil {
		return nil, err
	}
	webServerPort, err := getWebServerPort(cjfrSvc)
	if err != nil {
		return nil, err
	}
	return url.Parse(fmt.Sprintf("%s://%s.%s.svc:%d/", protocol, svcName, namespace, webServerPort))
}

func getPodIP(pod *corev1.Pod) (*string, error) {
	podIP := pod.Status.PodIP
	if len(podIP) == 0 {
		return nil, fmt.Errorf("PodIP unavailable for %s", pod.Name)
	}
	return &podIP, nil
}

func getWebServerPort(svc *corev1.Service) (int32, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == "export" {
			return port.Port, nil
		}
	}
	return 0, errors.New("ContainerJFR service had no port named \"export\"")
}
