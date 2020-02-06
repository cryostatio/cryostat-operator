package grafana

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	openshiftv1 "github.com/openshift/api/route/v1"
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
	defaultTransport := http.DefaultTransport.(*http.Transport)
	httpClient := http.Client{
		Transport: &http.Transport{
			Proxy:                 defaultTransport.Proxy,
			DialContext:           defaultTransport.DialContext,
			MaxIdleConns:          defaultTransport.MaxIdleConns,
			IdleConnTimeout:       defaultTransport.IdleConnTimeout,
			ExpectContinueTimeout: defaultTransport.ExpectContinueTimeout,
			TLSHandshakeTimeout:   defaultTransport.TLSHandshakeTimeout,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: strings.EqualFold(os.Getenv("TLS_VERIFY"), "false")},
		},
	}

	return &ReconcileGrafana{client: mgr.GetClient(), scheme: mgr.GetScheme(), httpClient: httpClient}
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
	httpClient http.Client
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

	route := &openshiftv1.Route{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, route)
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Found Grafana route", "Namespace",
		route.Namespace, "Name", route.Name)

	healthy, err := r.isServiceHealthy(route)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !healthy {
		reqLogger.Info("Grafana service is not (yet) healthy, requeuing")
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	} else {
		reqLogger.Info("Grafana service is healthy")
	}

	if err := r.configureGrafanaDatasource(route); err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Grafana datasource configured")

	if err := r.configureGrafanaDashboard(route); err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Grafana dashboard configured")

	return reconcile.Result{}, nil
}

func (r *ReconcileGrafana) isGrafanaService(svc *corev1.Service) bool {
	for k, v := range svc.Labels {
		if k == "app.kubernetes.io/name" && v == "grafana" {
			return true
		}
	}
	return false
}

func (r *ReconcileGrafana) isServiceHealthy(route *openshiftv1.Route) (bool, error) {
	if len(route.Status.Ingress) < 1 {
		return false, nil
	}
	resp, err := r.httpClient.Get(fmt.Sprintf("https://%s/api/health", route.Status.Ingress[0].Host))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}

func (r *ReconcileGrafana) configureGrafanaDatasource(route *openshiftv1.Route) error {
	logger := log.WithValues("Route.Namespace", route.Namespace, "Route.Name", route.Name)

	logger.Info("Checking existing datasource definitions")
	// TODO get an API token, rather than using basic auth and assumed default credentials
	getUrl := fmt.Sprintf("https://admin:admin@%s/api/datasources", route.Status.Ingress[0].Host)
	getResp, err := r.httpClient.Get(getUrl)
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
		logger.Error(err, "Invalid GET response", "Request URL", getUrl, "Response JSON", getBody)
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
	err = r.client.List(context.Background(), &services, client.InNamespace(route.Namespace), client.MatchingLabels{"app.kubernetes.io/name": "jfr-datasource"})
	if err != nil {
		return err
	}
	if len(services.Items) != 1 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected one jfr-datasource service, found %d", len(services.Items))))
	}
	if len(services.Items[0].Spec.Ports) != 1 {
		return errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected service %s to have one Port, but got %d", services.Items[0].Name, len(services.Items[0].Spec.Ports))))
	}
	datasourceUrl := fmt.Sprintf("http://%s:%d", services.Items[0].Spec.ClusterIP, services.Items[0].Spec.Ports[0].Port)

	datasource := GrafanaDatasource{
		Name:      "jfr-datasource",
		Type:      "grafana-simple-json-datasource",
		Url:       datasourceUrl,
		Access:    "proxy",
		BasicAuth: false,
		IsDefault: true,
	}
	logger.Info("POSTing JSON datasource definition", "datasource", datasource)
	dsStr, err := json.Marshal(datasource)
	if err != nil {
		return err
	}
	postResp, err := r.httpClient.Post(fmt.Sprintf("https://admin:admin@%s/api/datasources", route.Status.Ingress[0].Host), "application/json", bytes.NewBuffer(dsStr))
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

func (r *ReconcileGrafana) configureGrafanaDashboard(route *openshiftv1.Route) error {
	logger := log.WithValues("Route.Namespace", route.Namespace, "Route.Name", route.Name)

	// TODO find a way to list/search existing dashboards to avoid creating a duplicate
	postResp, err := r.httpClient.Post(fmt.Sprintf("https://admin:admin@%s/api/dashboards/db", route.Status.Ingress[0].Host), "application/json", bytes.NewBufferString(DashboardDefinitionJSON))
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

type GrafanaDatasourceList []GrafanaDatasource

type GrafanaDatasource struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Url       string `json:"url"`
	Access    string `json:"access"`
	BasicAuth bool   `json:"basicAuth"`
	IsDefault bool   `json:"isDefault"`
}
