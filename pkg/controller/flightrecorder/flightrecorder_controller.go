package flightrecorder

import (
	"context"
	"time"

	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	common "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_flightrecorder")

// Add creates a new FlightRecorder Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileFlightRecorder{scheme: mgr.GetScheme(),
		CommonReconciler: &common.CommonReconciler{
			Client: mgr.GetClient(),
		},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("flightrecorder-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource FlightRecorder
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha2.FlightRecorder{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileFlightRecorder implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileFlightRecorder{}

// ReconcileFlightRecorder reconciles a FlightRecorder object
type ReconcileFlightRecorder struct {
	scheme *runtime.Scheme
	*common.CommonReconciler
}

// Reconcile reads that state of the cluster for a FlightRecorder object and makes changes based on the state read
// and what is in the FlightRecorder.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFlightRecorder) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling FlightRecorder")

	cjfr, err := r.FindContainerJFR(ctx, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Keep client open to Container JFR as long as it doesn't fail
	if r.JfrClient == nil {
		jfrClient, err := r.ConnectToContainerJFR(ctx, cjfr.Namespace, cjfr.Name)
		if err != nil {
			// Need Container JFR in order to reconcile anything, requeue until it appears
			return reconcile.Result{}, err
		}
		r.JfrClient = jfrClient
	}

	// Fetch the FlightRecorder instance
	instance := &rhjmcv1alpha2.FlightRecorder{}
	err = r.Client.Get(ctx, request.NamespacedName, instance)
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

	// Look up service corresponding to this FlightRecorder object
	targetRef := instance.Status.Target
	if targetRef == nil {
		// FlightRecorder status must not have been updated yet
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	targetSvc := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: targetRef.Namespace, Name: targetRef.Name}, targetSvc)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Tell Container JFR to connect to the target service
	jfrclient.ClientLock.Lock()
	defer jfrclient.ClientLock.Unlock()
	err = r.ConnectToService(targetSvc, instance.Status.Port)
	if err != nil {
		return reconcile.Result{}, err
	}
	defer r.DisconnectClient()

	reqLogger.Info("FlightRecorder successfully updated", "Namespace", instance.Namespace, "Name", instance.Name)
	return reconcile.Result{}, nil
}
