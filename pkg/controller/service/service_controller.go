package service

import (
	"context"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_service")

// Add creates a new Service Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileService{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("service-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch services and create FlightRecorder objects if they fit criteria (e.g. port 9091)
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource FlightRecorder and requeue the owner Service
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha1.FlightRecorder{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &corev1.Service{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Service object
type ReconcileService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Service object and makes changes based on the state read
// and what is in the Service.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Service")

	// Fetch the Service instance
	svc := &corev1.Service{}
	ctx := context.Background()
	err := r.client.Get(ctx, request.NamespacedName, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Check if this service appears to be compatible with Container JFR
	if !isJFRAwareService(svc) {
		return reconcile.Result{}, nil
	}
	reqLogger.Info("Found service that appears to be compatible with Container JFR", "Namespace",
		svc.Namespace, "Name", svc.Name)

	// Define a new FlightRecorder object for this service
	jfr, err := r.newFlightRecorderForService(svc)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set Service instance as the owner and controller
	if err := controllerutil.SetControllerReference(svc, jfr, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this FlightRecorder already exists
	found := &rhjmcv1alpha1.FlightRecorder{}
	err = r.client.Get(ctx, types.NamespacedName{Name: jfr.Name, Namespace: jfr.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new FlightRecorder", "Namespace", jfr.Namespace, "Name", jfr.Name)
		err = r.client.Create(ctx, jfr)
		if err != nil {
			return reconcile.Result{}, err
		}
		// Update FlightRecorder Status
		err = r.client.Status().Update(ctx, jfr)
		if err != nil {
			return reconcile.Result{}, err
		}

		// FlightRecorder created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// TODO do we want to delete and recreate?
	// FlightRecorder already exists - don't requeue
	reqLogger.Info("Skip reconcile: FlightRecorder already exists", "Namespace", found.Namespace, "Name", found.Name)
	return reconcile.Result{}, nil
}

const defaultContainerJFRPort int = 9091

func isJFRAwareService(svc *corev1.Service) bool {
	for _, port := range svc.Spec.Ports {
		if port.Name == "jmx" {
			return true
		}
		if port.TargetPort.IntValue() == defaultContainerJFRPort {
			return true
		}
	}
	return false
}

// newFlightRecorderForService returns a FlightRecorder with the same name/namespace as the service
func (r *ReconcileService) newFlightRecorderForService(svc *corev1.Service) (*rhjmcv1alpha1.FlightRecorder, error) {
	appLabel := svc.Name // Use service name as fallback
	if label, pres := svc.Labels["app"]; pres {
		appLabel = label
	}
	labels := map[string]string{
		"app": appLabel,
	}
	ref, err := reference.GetReference(r.scheme, svc)
	if err != nil {
		return nil, err
	}
	return &rhjmcv1alpha1.FlightRecorder{ // TODO should we use OwnerReference for this?
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels:    labels,
		},
		Spec: rhjmcv1alpha1.FlightRecorderSpec{
			Requests: []rhjmcv1alpha1.RecordingRequest{},
		},
		Status: rhjmcv1alpha1.FlightRecorderStatus{
			Target:     ref,
			Recordings: []rhjmcv1alpha1.RecordingInfo{},
		},
	}, nil
}
