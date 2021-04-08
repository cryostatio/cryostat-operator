// Copyright (c) 2021 Red Hat, Inc.
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

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	cryostatClient "github.com/cryostatio/cryostat-operator/controllers/client"
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
	Client client.Client
	// Optional field to specify an alternate ClientFactory used by
	// Reconciler to create CryostatClients
	ClientFactory CryostatClientFactory
	// Optional field to override the default behaviour when interacting
	// with the operating system
	OS OSUtils
}

// Reconciler contains helpful methods to communicate with Cryostat
// It is meant to be embedded within other Reconcilers.
type Reconciler interface {
	FindCryostat(ctx context.Context, namespace string) (*operatorv1beta1.Cryostat, error)
	GetCryostatClient(ctx context.Context, namespace string, jmxAuth *operatorv1beta1.JMXAuthSecret) (cryostatClient.CryostatClient, error)
	GetPodTarget(targetPod *corev1.Pod, jmxPort int32) (*cryostatClient.TargetAddress, error)
	ReconcilerTLS
}

type commonReconciler struct {
	*ReconcilerConfig
	ReconcilerTLS
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
		ReconcilerTLS: NewReconcilerTLS(&ReconcilerTLSConfig{
			Client:  configCopy.Client,
			OSUtils: configCopy.OS,
		}),
	}
}

// GetCryostatClient creates a client to communicate with the Cryostat
// instance deployed by this operator in the given namespace
func (r *commonReconciler) GetCryostatClient(ctx context.Context, namespace string,
	jmxAuth *operatorv1beta1.JMXAuthSecret) (cryostatClient.CryostatClient, error) {
	// Look up Cryostat instance within the given namespace
	cryostat, err := r.FindCryostat(ctx, namespace)
	if err != nil {
		return nil, err
	}
	// Get CA certificate if TLS is enabled
	var caCert []byte
	protocol := "http"
	if r.IsCertManagerEnabled() {
		caCert, err = r.GetCryostatCABytes(ctx, cryostat)
		if err != nil {
			return nil, err
		}
		protocol = "https"
	}
	// Get the URL to the Cryostat web service
	serverURL, err := r.getServerURL(ctx, cryostat.Namespace, cryostat.Name, protocol)
	if err != nil {
		return nil, err
	}
	// Read bearer token from mounted secret
	tok, err := r.OS.GetFileContents("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, err
	}
	strTok := string(tok)

	// Get JMX authentication credentials, if present
	var jmxCreds *cryostatClient.JMXAuthCredentials
	if jmxAuth != nil {
		jmxCreds, err = r.getJMXCredentialsFromSecret(ctx, namespace, jmxAuth)
		if err != nil {
			return nil, err
		}
	}

	// Create Cryostat HTTP(S) client
	config := &cryostatClient.Config{
		ServerURL:      serverURL,
		AccessToken:    &strTok,
		CACertificate:  caCert,
		JMXCredentials: jmxCreds,
	}
	cryostatClient, err := r.ClientFactory.CreateClient(config)
	if err != nil {
		return nil, err
	}
	return cryostatClient, nil
}

// GetPodTarget returns a TargetAddress for a particular pod and port number
func (r *commonReconciler) GetPodTarget(targetPod *corev1.Pod, jmxPort int32) (*cryostatClient.TargetAddress, error) {
	// Create TargetAddress using pod's IP address and provided port
	podIP, err := getPodIP(targetPod)
	if err != nil {
		return nil, err
	}
	return &cryostatClient.TargetAddress{
		Host: *podIP,
		Port: jmxPort,
	}, nil
}

func (r *commonReconciler) FindCryostat(ctx context.Context, namespace string) (*operatorv1beta1.Cryostat, error) {
	// TODO Consider how to find Cryostat object if this operator becomes cluster-scoped
	// Look up the Cryostat object for this operator, which will help us find its services
	cryostatList := &operatorv1beta1.CryostatList{}
	err := r.Client.List(ctx, cryostatList)
	if err != nil {
		return nil, err
	}
	if len(cryostatList.Items) == 0 {
		return nil, errors.New("No Cryostat objects found")
	} else if len(cryostatList.Items) > 1 {
		// Does not seem like a proper use-case
		log.Info("More than one Cryostat object found in namespace, using only the first one listed",
			"namespace", namespace)
	}
	return &cryostatList.Items[0], nil
}

func (r *commonReconciler) getServerURL(ctx context.Context, namespace string, svcName string, protocol string) (*url.URL, error) {
	// Look up Cryostat service, and build URL to web service
	cryostatSvc := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: svcName}, cryostatSvc)
	if err != nil {
		return nil, err
	}
	webServerPort, err := getWebServerPort(cryostatSvc)
	if err != nil {
		return nil, err
	}
	return url.Parse(fmt.Sprintf("%s://%s.%s.svc:%d/", protocol, svcName, namespace, webServerPort))
}

func (r *commonReconciler) getJMXCredentialsFromSecret(ctx context.Context, namespace string,
	jmxSecret *operatorv1beta1.JMXAuthSecret) (*cryostatClient.JMXAuthCredentials, error) {
	// Look up referenced secret
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: jmxSecret.SecretName, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}

	// Get credentials from secret
	username, err := getValueFromSecret(secret, jmxSecret.UsernameKey, operatorv1beta1.DefaultUsernameKey)
	if err != nil {
		return nil, err
	}
	password, err := getValueFromSecret(secret, jmxSecret.PasswordKey, operatorv1beta1.DefaultPasswordKey)
	if err != nil {
		return nil, err
	}

	return &cryostatClient.JMXAuthCredentials{
		Username: *username,
		Password: *password,
	}, nil
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
	return 0, errors.New("Cryostat service had no port named \"export\"")
}

func getValueFromSecret(secret *corev1.Secret, key *string, defaultKey string) (*string, error) {
	// Use the default key if no key was specified
	if key == nil {
		key = &defaultKey
	}
	// Return an error if value is missing in secret
	rawValue, pres := secret.Data[*key]
	if !pres {
		return nil, fmt.Errorf("No key \"%s\" found in secret \"%s/%s\"", *key, secret.Namespace, secret.Name)
	}
	result := string(rawValue)
	return &result, nil
}
