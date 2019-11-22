package grafana

import (
	"context"
	"fmt"
	"net/http"
	"time"

	openshiftv1 "github.com/openshift/api/route/v1"
	routeClient "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	rc := routeClient.NewForConfigOrDie(mgr.GetConfig())
	return &ReconcileGrafana{client: mgr.GetClient(), routeClient: *rc, scheme: mgr.GetScheme()}
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
	client      client.Client
	routeClient routeClient.RouteV1Client
	scheme      *runtime.Scheme
}

func (r *ReconcileGrafana) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Service")

	svc := &corev1.Service{}
	ctx := context.Background()
	err := r.client.Get(ctx, request.NamespacedName, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !isGrafanaService(svc) {
		return reconcile.Result{}, nil
	}
	reqLogger.Info("Found Grafana service", "Namespace",
		svc.Namespace, "Name", svc.Name)

	route, err := r.routeClient.Routes(svc.Namespace).Get(svc.Name, metav1.GetOptions{})
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info("Found Grafana route", "Namespace",
		route.Namespace, "Name", route.Name)

	healthy, err := isServiceHealthy(route)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !healthy {
		reqLogger.Info("Grafana service is not (yet) healthy, requeuing")
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	} else {
		reqLogger.Info("Grafana service is healthy")
	}

	return reconcile.Result{}, nil
}

func isGrafanaService(svc *corev1.Service) bool {
	for k, v := range svc.Labels {
		if k == "component" && v == "grafana" {
			return true
		}
	}
	return false
}

func isServiceHealthy(route *openshiftv1.Route) (bool, error) {
	if len(route.Status.Ingress) < 1 {
		return false, nil
	}
	resp, err := http.Get(fmt.Sprintf("http://%s/api/health", route.Status.Ingress[0].Host))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, nil
}
