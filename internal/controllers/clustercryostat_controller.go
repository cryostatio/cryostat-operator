package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Generates constants from environment variables at build time
//go:generate go run ../tools/const_generator.go

// Verify that *ClusterCryostatReconciler implements CommonReconciler.
var _ CommonReconciler = (*ClusterCryostatReconciler)(nil)

// CryostatReconciler reconciles a Cryostat object
type ClusterCryostatReconciler struct {
	delegate *Reconciler
	*ReconcilerConfig
}

func NewClusterCryostatReconciler(config *ReconcilerConfig) *ClusterCryostatReconciler {
	return &ClusterCryostatReconciler{
		ReconcilerConfig: config,
		delegate: &Reconciler{
			ReconcilerConfig: config,
		},
	}
}

// +kubebuilder:rbac:groups="",resources=pods;services;services/finalizers;endpoints;persistentvolumeclaims;events;configmaps;secrets;serviceaccounts,verbs=*
// +kubebuilder:rbac:groups="",resources=replicationcontrollers,verbs=get
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=selfsubjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=oauth.openshift.io,resources=oauthaccesstokens,verbs=list;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;update;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=*
// +kubebuilder:rbac:groups=apps.openshift.io,resources=deploymentconfigs,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=*
// +kubebuilder:rbac:namespace=system,groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=clustercryostats,verbs=*
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=clustercryostats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=clustercryostats/finalizers,verbs=update
// +kubebuilder:rbac:groups=console.openshift.io,resources=consolelinks,verbs=get;create;list;update;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=*

// Reconcile processes a ClusterCryostat CR and manages a Cryostat installation accordingly
func (r *ClusterCryostatReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Name", request.Name)

	reqLogger.Info("Reconciling ClusterCryostat")

	// Fetch the Cryostat instance
	cr := &operatorv1beta1.ClusterCryostat{}
	err := r.Client.Get(ctx, request.NamespacedName, cr)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("ClusterCryostat instance not found")
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Error reading ClusterCryostat instance")
		return reconcile.Result{}, err
	}

	instance := model.FromClusterCryostat(cr)
	return r.delegate.reconcileCryostat(ctx, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterCryostatReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.delegate.setupWithManager(mgr, &operatorv1beta1.ClusterCryostat{}, r)
}

func (r *ClusterCryostatReconciler) GetConfig() *ReconcilerConfig {
	return r.ReconcilerConfig
}
