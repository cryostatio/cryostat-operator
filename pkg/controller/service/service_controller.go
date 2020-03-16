package service

import (
	"context"
	"errors"

	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
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
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha2.FlightRecorder{}}, &handler.EnqueueRequestForOwner{
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
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Check if this service appears to be compatible with Container JFR
	jmxPort, err := getServiceJMXPort(svc)
	jmxCompatible := err == nil

	// Check if this FlightRecorder already exists
	found := &rhjmcv1alpha2.FlightRecorder{}
	jfrName := svc.Name
	err = r.client.Get(ctx, types.NamespacedName{Name: jfrName, Namespace: request.Namespace}, found)
	if err != nil && kerrors.IsNotFound(err) {

		if jmxCompatible {
			reqLogger.Info("Creating a new FlightRecorder", "Namespace", request.Namespace, "Name", jfrName)

			// Define a new FlightRecorder object for this service
			jfr, err := r.newFlightRecorderForService(jfrName, svc, jmxPort)
			if err != nil {
				return reconcile.Result{}, err
			}

			// Set Service instance as the owner and controller
			if err := controllerutil.SetControllerReference(svc, jfr, r.scheme); err != nil {
				return reconcile.Result{}, err
			}

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
		} else {
			// this service is not compatible and no existing FlightRecorder found, so nothing to do
			return reconcile.Result{}, nil
		}

	} else if err == nil {
		// existing FlightRecorder found

		if !jmxCompatible {
			// Service was previously JMX-compatible but is not anymore,
			// so delete its FlightRecorder and do not requeue a new one
			reqLogger.Info("Deleting dangling FlightRecorder", "Namespace", request.Namespace, "Name", jfrName)
			err = r.client.Delete(ctx, found)
			if err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		if found.Status.Port != jmxPort {
			// FlightRecorder is incorrect - service was likely modified. Delete outdated FlightRecorder
			// and requeue creation of corrected one
			reqLogger.Info("Deleting outdated FlightRecorder", "Namespace", request.Namespace, "Name", jfrName)
			err = r.client.Delete(ctx, found)
			if err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{Requeue: true}, nil
		}
	} else if err != nil && jmxCompatible {
		return reconcile.Result{}, err
	}

	// FlightRecorder already exists and is correct - don't requeue
	reqLogger.Info("Skip reconcile: FlightRecorder already exists", "Namespace", found.Namespace, "Name", found.Name)
	return reconcile.Result{}, nil
}

const defaultContainerJFRPort int32 = 9091
const jmxServicePortName = "jfr-jmx"

func getServiceJMXPort(svc *corev1.Service) (int32, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == jmxServicePortName {
			return port.Port, nil
		}
		if port.TargetPort.IntValue() == int(defaultContainerJFRPort) {
			return defaultContainerJFRPort, nil
		}
	}
	return 0, errors.New("Service does not appear to have a JMX port")
}

// newFlightRecorderForService returns a FlightRecorder with the same name/namespace as the service
func (r *ReconcileService) newFlightRecorderForService(name string, svc *corev1.Service, jmxPort int32) (*rhjmcv1alpha2.FlightRecorder, error) {
	// Inherit "app" label from service
	appLabel := svc.Name // Use service name as fallback
	if label, pres := svc.Labels["app"]; pres {
		appLabel = label
	}
	labels := map[string]string{
		"app": appLabel,
	}

	// Add reference to service to this FlightRecorder
	ref, err := reference.GetReference(r.scheme, svc)
	if err != nil {
		return nil, err
	}

	// Use label selector matching the name of this FlightRecorder
	selector := &metav1.LabelSelector{}
	selector = metav1.AddLabelToSelector(selector, rhjmcv1alpha2.RecordingLabel, name)

	return &rhjmcv1alpha2.FlightRecorder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: svc.Namespace,
			Labels:    labels,
		},
		Spec: rhjmcv1alpha2.FlightRecorderSpec{
			RecordingSelector: selector,
		},
		Status: rhjmcv1alpha2.FlightRecorderStatus{
			Events: []rhjmcv1alpha2.EventInfo{},
			Target: ref,
			Port:   jmxPort,
		},
	}, nil
}
