package cleanup

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cleanup")

// Add creates a new Cleanup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	watchNs, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return err
	}

	return add(mgr, newReconciler(mgr, watchNs))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, watchNs string) reconcile.Reconciler {
	return &ReconcileDeployment{client: mgr.GetClient(),
		scheme: mgr.GetScheme(), watchNs: watchNs}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cleanup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Fetch the operator's deployment namespace and name
	opNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return err // TODO local case possible?
	}
	opName, err := k8sutil.GetOperatorName()
	if err != nil {
		return err
	}

	// Only watch the deployment for this operator
	p := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Meta.GetName() == opName && e.Meta.GetNamespace() == opNs
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Meta.GetName() == opName && e.Meta.GetNamespace() == opNs
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.MetaNew.GetName() == opName && e.MetaNew.GetNamespace() == opNs
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// Watch for changes to primary resource Deployment
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDeployment{}

// ReconcileDeployment reconciles a Deployment object
type ReconcileDeployment struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	// Namespace that the operator is watching
	watchNs string
}

// ContainerJFRFinalizer is a finalizer to clean up external resources before exiting
const ContainerJFRFinalizer = "rhjmc.redhat.com/containerjfrFinalizer"

// Reconcile reads that state of the cluster for a Deployment object and makes changes based on the state read
// and what is in the Deployment.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Operator Deployment")

	// Fetch the Deployment instance
	instance := &appsv1.Deployment{}
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

	// Check if this operator deployment is being deleted
	if instance.GetDeletionTimestamp() != nil {
		if hasCleanupFinalizer(instance) {
			// Cleanly delete all recordings
			err := r.deleteRecordings(ctx, r.watchNs)
			if err != nil {
				log.Error(err, "failed to delete recordings", "namespace", instance.Namespace,
					"name", instance.Name)
				return reconcile.Result{}, err
			}

			// Remove our finalizer only once our cleanup logic has succeeded
			err = r.removeCleanupFinalizer(ctx, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		// Ready for deletion
		return reconcile.Result{}, nil
	}

	// Add our finalizer, so we can clean up Container JFR resources upon deletion
	if !hasCleanupFinalizer(instance) {
		err := r.addCleanupFinalizer(ctx, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info("Operator Deployment successfully updated", "Namespace", instance.Namespace, "Name", instance.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileDeployment) deleteRecordings(ctx context.Context, namespace string) error {
	// Delete all recordings in the foreground, so that when this returns we can safely exit
	propagation := metav1.DeletePropagationForeground
	err := r.client.DeleteAllOf(ctx, &rhjmcv1alpha2.Recording{}, &client.DeleteAllOfOptions{
		DeleteOptions: client.DeleteOptions{
			PropagationPolicy: &propagation,
		},
		ListOptions: client.ListOptions{
			Namespace: namespace, // TODO Might make more sense to delete at cluster-level going forward
		},
	})
	// TODO Do we need finalizer on ContainerJFR CR as well?

	// Check if any recordings still exist
	recordings := &v1alpha2.RecordingList{}
	err = r.client.List(ctx, recordings)
	if err != nil {
		return err
	}
	if len(recordings.Items) > 0 {
		return fmt.Errorf("%d recordings still exist in %s", len(recordings.Items), namespace)
	}
	return err
}

func (r *ReconcileDeployment) addCleanupFinalizer(ctx context.Context, deployment *appsv1.Deployment) error {
	log.Info("adding finalizer for recording", "namespace", deployment.Namespace, "name", deployment.Name)
	finalizers := append(deployment.GetFinalizers(), ContainerJFRFinalizer)
	deployment.SetFinalizers(finalizers)

	err := r.client.Update(ctx, deployment)
	if err != nil {
		log.Error(err, "failed to add finalizer to deployment", "namespace", deployment.Namespace,
			"name", deployment.Name)
		return err
	}
	return nil
}

func (r *ReconcileDeployment) removeCleanupFinalizer(ctx context.Context, deployment *appsv1.Deployment) error {
	finalizers := deployment.GetFinalizers()
	foundIdx := -1
	for idx, finalizer := range finalizers {
		if finalizer == ContainerJFRFinalizer {
			foundIdx = idx
			break
		}
	}

	if foundIdx >= 0 {
		// Remove our finalizer from the slice
		finalizers = append(finalizers[:foundIdx], finalizers[foundIdx+1:]...)
		deployment.SetFinalizers(finalizers)
		err := r.client.Update(ctx, deployment)
		if err != nil {
			log.Error(err, "failed to remove finalizer from deployment", "namespace", deployment.Namespace,
				"name", deployment.Name)
			return err
		}
	}
	return nil
}

func hasCleanupFinalizer(deployment *appsv1.Deployment) bool {
	for _, finalizer := range deployment.GetFinalizers() {
		if finalizer == ContainerJFRFinalizer {
			return true
		}
	}
	return false
}
