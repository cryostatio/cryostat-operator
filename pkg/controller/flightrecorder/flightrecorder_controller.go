package flightrecorder

import (
	"context"
	"fmt"
	"net/url"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_flightrecorder")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new FlightRecorder Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileFlightRecorder{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("flightrecorder-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource FlightRecorder
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha1.FlightRecorder{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileFlightRecorder implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileFlightRecorder{}

// ReconcileFlightRecorder reconciles a FlightRecorder object
type ReconcileFlightRecorder struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client    client.Client
	scheme    *runtime.Scheme
	jfrClient *jfrclient.ContainerJfrClient
}

// Reconcile reads that state of the cluster for a FlightRecorder object and makes changes based on the state read
// and what is in the FlightRecorder.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFlightRecorder) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling FlightRecorder")

	if r.jfrClient == nil {
		jfrClient, err := r.connectToContainerJFR(ctx, request.Namespace)
		if err != nil {
			// Need service in order to reconcile anything, requeue until it appears
			return reconcile.Result{}, err
		}
		r.jfrClient = jfrClient
	}

	// Fetch the FlightRecorder instance
	instance := &rhjmcv1alpha1.FlightRecorder{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	targetRef := instance.Status.Target
	targetSvc := &corev1.Service{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: targetRef.Namespace, Name: targetRef.Name}, targetSvc)
	if err != nil {
		return reconcile.Result{}, err // TODO should we requeue?
	}

	clusterIP, err := getClusterIP(targetSvc)
	if err != nil {
		return reconcile.Result{}, err // TODO should we requeue?
	}
	err = r.jfrClient.ListRecordings(*clusterIP, 9091) // FIXME hardcoded port
	if err != nil {
		log.Error(err, "failed to connect to command server")
		r.jfrClient.Close()
		r.jfrClient = nil
		return reconcile.Result{}, err
	}

	reqLogger.Info("Found FlightRecorder", "Namespace", instance.Namespace, "Name", instance.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileFlightRecorder) connectToContainerJFR(ctx context.Context, namespace string) (*jfrclient.ContainerJfrClient, error) {
	commandSvc := &corev1.Service{}
	commandSvcName := "containerjfr-command"                                                               // TODO make const or get from ContainerJFR
	err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: commandSvcName}, commandSvc) // TODO what if it's a different namespace
	if err != nil {
		return nil, err
	}

	clusterIP, err := getClusterIP(commandSvc)
	if err != nil {
		return nil, err
	}
	host := fmt.Sprintf("%s:%d", *clusterIP, 9090) // FIXME hardcoded port
	commandURL := &url.URL{Scheme: "ws", Host: host, Path: "command"}
	config := &jfrclient.ClientConfig{ServerURL: commandURL}
	jfrClient, err := jfrclient.Create(config)
	if err != nil {
		return nil, err
	}
	return jfrClient, nil
}

func getClusterIP(svc *corev1.Service) (*string, error) {
	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == corev1.ClusterIPNone {
		return nil, fmt.Errorf("ClusterIP unavailable for %s", svc.Name)
	}
	return &clusterIP, nil
}
