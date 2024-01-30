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
	return r.delegate.setupWithManager(mgr, r)
}

func (r *CryostatReconciler) GetConfig() *ReconcilerConfig {
	return r.ReconcilerConfig
}
