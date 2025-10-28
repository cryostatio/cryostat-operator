// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
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

func NewCryostatReconciler(config *ReconcilerConfig) (*CryostatReconciler, error) {
	delegate, err := newReconciler(config, &operatorv1beta2.Cryostat{}, true)
	if err != nil {
		return nil, err
	}
	return &CryostatReconciler{
		ReconcilerConfig: config,
		delegate:         delegate,
	}, nil
}

// +kubebuilder:rbac:groups=operator.cryostat.io,resources=cryostats,verbs=*
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=cryostats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.cryostat.io,resources=cryostats/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods;services;services/finalizers;persistentvolumeclaims;events;configmaps;secrets;serviceaccounts,verbs=*
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=replicationcontrollers,verbs=get
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;update;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=*
// +kubebuilder:rbac:groups=apps.openshift.io,resources=deploymentconfigs,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=*
// +kubebuilder:rbac:namespace=system,groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates/finalizers,verbs=update
// +kubebuilder:rbac:groups=console.openshift.io,resources=consolelinks,verbs=get;create;list;update;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;networkpolicies,verbs=*

// RBAC for Insights controller, remove these when moving to a separate container
// +kubebuilder:rbac:namespace=system,groups=apps,resources=deployments;deployments/finalizers,verbs=create;update;get;list;watch
// +kubebuilder:rbac:namespace=system,groups="",resources=services;secrets;configmaps/finalizers,verbs=create;update;get;list;watch
// +kubebuilder:rbac:namespace=system,groups="",resources=configmaps,verbs=create;update;delete;get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch
// OLM doesn't let us specify RBAC for openshift-config namespace, so we need a cluster-wide permission
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch,resourceNames=pull-secret

// Reconcile processes a Cryostat CR and manages a Cryostat installation accordingly
func (r *CryostatReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("Reconciling Cryostat")

	// Fetch the Cryostat instance
	cr := &operatorv1beta2.Cryostat{}
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
	return r.delegate.setupWithManager(r.NewControllerBuilder(mgr), r)
}

func (r *CryostatReconciler) GetConfig() *ReconcilerConfig {
	return r.ReconcilerConfig
}
