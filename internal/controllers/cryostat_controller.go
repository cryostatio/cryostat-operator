package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Verify that *CryostatReconciler implements CommonReconciler.
var _ CommonReconciler = (*CryostatReconciler)(nil)

// CryostatReconciler reconciles a Cryostat object
type CryostatReconciler struct {
	delegate *Reconciler
	*ReconcilerConfig
}

func NewCryostatReconciler(config *ReconcilerConfig) *CryostatReconciler {
	return &CryostatReconciler{
		ReconcilerConfig: config,
		delegate: &Reconciler{
			ReconcilerConfig: config,
		},
	}
}

// +kubebuilder:rbac:groups=operator.cryostat.io,resources=cryostats,verbs=*
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=cryostats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=cryostats/finalizers,verbs=update

// Reconcile processes a Cryostat CR and manages a Cryostat installation accordingly
func (r *CryostatReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("Reconciling Cryostat")

	// Fetch the Cryostat instance
	cr := &operatorv1beta1.Cryostat{}
	err := r.Client.Get(ctx, request.NamespacedName, cr)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Cryostat instance not found")
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Error reading Cryostat instance")
		return reconcile.Result{}, err
	}

	instance := model.FromCryostat(cr)
	return r.delegate.reconcileCryostat(ctx, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *CryostatReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.delegate.setupWithManager(mgr, &operatorv1beta1.Cryostat{}, r)
}

func (r *CryostatReconciler) GetConfig() *ReconcilerConfig {
	return r.ReconcilerConfig
}
