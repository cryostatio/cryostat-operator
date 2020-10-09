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

package grafana

import (
	"bytes"
	"context"
	gotls "crypto/tls"
	"crypto/x509"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/tls"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_grafana")

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGrafana{client: mgr.GetClient(), scheme: mgr.GetScheme(),
		Reconciler: common.NewReconciler(&common.ReconcilerConfig{
			Client: mgr.GetClient(),
		})}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New("grafana-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileGrafana{}

type ReconcileGrafana struct {
	client     client.Client
	scheme     *runtime.Scheme
	httpClient *http.Client
	common.Reconciler
}

func (r *ReconcileGrafana) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	svc := &corev1.Service{}
	ctx := context.Background()
	err := r.client.Get(ctx, request.NamespacedName, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !r.isGrafanaService(svc) {
		return reconcile.Result{}, nil
	}
	reqLogger.Info("Found Grafana service", "Namespace",
		svc.Namespace, "Name", svc.Name)

	https := r.IsCertManagerEnabled()
	if r.httpClient == nil { // TODO disable caching? CA changing would cause problem
		// Get CA certificate if TLS is enabled
		cjfr, err := r.FindContainerJFR(ctx, svc.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		var rootCAPool *x509.CertPool
		if https {
			caCert, err := r.GetContainerJFRCABytes(ctx, cjfr)
			if err != nil {
				if err == tls.ErrNotReady {
					log.Info("Waiting for CA certificate")
					return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
				}
				return reconcile.Result{}, err
			}

			// Create CertPool for CA certificate
			rootCAPool = x509.NewCertPool()
			ok := rootCAPool.AppendCertsFromPEM(caCert)
			if !ok {
				return reconcile.Result{}, goerrors.New("Failed to parse CA certificate")
			}
		}

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &gotls.Config{
			RootCAs:            rootCAPool,
			InsecureSkipVerify: strings.EqualFold(os.Getenv("TLS_VERIFY"), "false"),
		}
		httpClient := &http.Client{
			Transport: transport,
		}
		r.httpClient = httpClient
	}

	protocol := "http"
	if https {
		protocol = "https"
	}

	secret := &corev1.Secret{}
	err = r.client.Get(ctx, types.NamespacedName{Name: svc.Name + "-basic", Namespace: request.Namespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Could not find Grafana credentials secret")
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}
	reqLogger.Info("Found Grafana credentials secret", "Namespace",
		secret.Namespace, "Name", secret.Name)

	healthy, err := r.isServiceHealthy(svc, protocol)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !healthy {
		reqLogger.Info("Grafana service is not (yet) healthy, requeuing")
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	} else {
		reqLogger.Info("Grafana service is healthy")
	}

	if err := r.configureGrafanaDatasource(secret, svc, protocol); err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Grafana datasource configured")

	if err := r.configureGrafanaDashboard(secret, svc, protocol); err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Grafana dashboard configured")

	return reconcile.Result{}, nil
}

func (r *ReconcileGrafana) isGrafanaService(svc *corev1.Service) bool {
	for k, v := range svc.Labels {
		if k == "component" && v == "grafana" {
			return true
		}
	}
	return false
}

func (r *ReconcileGrafana) isServiceHealthy(svc *corev1.Service, protocol string) (bool, error) {
	resp, err := r.httpClient.Get(fmt.Sprintf("%s://%s.%s.svc:%d/api/health", protocol, svc.Name, svc.Namespace,
		svc.Spec.Ports[0].Port))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (r *ReconcileGrafana) configureGrafanaDatasource(secret *corev1.Secret, svc *corev1.Service, protocol string) error {
	logger := log.WithValues("Route.Namespace", svc.Namespace, "Route.Name", svc.Name)

	logger.Info("Checking existing datasource definitions")
	// TODO get an API token, rather than using basic auth and assumed default credentials
	getURL := GetCredentialedHostPathURL(secret, svc, protocol, "/api/datasources")
	getResp, err := r.httpClient.Get(getURL)
	if err != nil {
		return err
	}
	defer getResp.Body.Close()
	configuredDatasources := GrafanaDatasourceList{}
	getBody, err := ioutil.ReadAll(getResp.Body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(getBody, &configuredDatasources); err != nil {
		logger.Error(err, "Invalid GET response", "Request URL", getURL, "Response JSON", getBody)
		return err
	}
	logger.Info("Found existing datasource definitions", "datasources", configuredDatasources)
	if len(configuredDatasources) > 1 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected zero or one configured datasources, found %d", len(configuredDatasources))))
	} else if len(configuredDatasources) == 1 {
		if configuredDatasources[0].Name != "jfr-datasource" {
			return errors.NewInternalError(goerrors.New(fmt.Sprintf("Found unexpected configured datasource %s", configuredDatasources[0].Name)))
		} else {
			return nil
		}
	}

	logger.Info("Checking for jfr-datasource service")
	services := corev1.ServiceList{}
	err = r.client.List(context.Background(), &services, client.InNamespace(svc.Namespace), client.MatchingLabels{"component": "jfr-datasource"})
	if err != nil {
		return err
	}
	if len(services.Items) != 1 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected one jfr-datasource service, found %d", len(services.Items))))
	}
	if len(services.Items[0].Spec.Ports) != 1 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected service %s to have one Port, but got %d", services.Items[0].Name, len(services.Items[0].Spec.Ports))))
	}
	datasourceURL := fmt.Sprintf("http://%s:%d", services.Items[0].Spec.ClusterIP, services.Items[0].Spec.Ports[0].Port)

	datasource := GrafanaDatasource{
		Name:      "jfr-datasource",
		Type:      "grafana-simple-json-datasource",
		URL:       datasourceURL,
		Access:    "proxy",
		BasicAuth: false,
		IsDefault: true,
	}
	logger.Info("POSTing JSON datasource definition", "datasource", datasource)
	dsStr, err := json.Marshal(datasource)
	if err != nil {
		return err
	}
	postResp, err := r.httpClient.Post(GetCredentialedHostPathURL(secret, svc, protocol, "/api/datasources"), "application/json", bytes.NewBuffer(dsStr))
	logger.Info("POST response", "Status", postResp.Status, "StatusCode", postResp.StatusCode)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()
	if postResp.StatusCode != 200 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Grafana service responded HTTP %d when creating datasource", postResp.StatusCode)))
	}
	postBody, err := ioutil.ReadAll(postResp.Body)
	if err != nil {
		return err
	}
	logger.Info("POST response", "Body", string(postBody))
	return nil
}

func (r *ReconcileGrafana) configureGrafanaDashboard(secret *corev1.Secret, svc *corev1.Service, protocol string) error {
	logger := log.WithValues("Route.Namespace", svc.Namespace, "Route.Name", svc.Name)

	// TODO find a way to list/search existing dashboards to avoid creating a duplicate
	postResp, err := r.httpClient.Post(GetCredentialedHostPathURL(secret, svc, protocol, "/api/dashboards/db"), "application/json", bytes.NewBufferString(DashboardDefinitionJSON))
	logger.Info("POST response", "Status", postResp.Status, "StatusCode", postResp.StatusCode)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()
	if postResp.StatusCode != 200 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Grafana service responded HTTP %d when creating dashboard", postResp.StatusCode)))
	}
	postBody, err := ioutil.ReadAll(postResp.Body)
	if err != nil {
		return err
	}
	logger.Info("POST response", "Body", string(postBody))
	return nil
}

func GetCredentialedHostPathURL(secret *corev1.Secret, svc *corev1.Service, protocol string, path string) string {
	user := secret.Data["GF_SECURITY_ADMIN_USER"]
	pass := secret.Data["GF_SECURITY_ADMIN_PASSWORD"]
	return fmt.Sprintf("%s://%s:%s@%s.%s.svc:%d%s", protocol, strings.TrimSpace(string(user)), strings.TrimSpace(string(pass)),
		svc.Name, svc.Namespace, svc.Spec.Ports[0].Port, path)
}

type GrafanaDatasourceList []GrafanaDatasource

type GrafanaDatasource struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Access    string `json:"access"`
	BasicAuth bool   `json:"basicAuth"`
	IsDefault bool   `json:"isDefault"`
}
