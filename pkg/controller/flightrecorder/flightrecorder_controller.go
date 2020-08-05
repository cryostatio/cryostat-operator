// Copyright (c) 2020 Red Hat, Inc.
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package flightrecorder

import (
	"context"
	"time"

	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	common "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
func newReconciler(mgr manager.Manager) *ReconcileFlightRecorder {
	return &ReconcileFlightRecorder{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
		Reconciler: common.NewReconciler(&common.ReconcilerConfig{
			Client: mgr.GetClient(),
		}),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileFlightRecorder) error {
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
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client client.Client
	Scheme *runtime.Scheme
	common.Reconciler
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

	// Fetch the FlightRecorder instance
	instance := &rhjmcv1alpha2.FlightRecorder{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
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

	// Check for a valid target reference
	targetRef := instance.Status.Target
	if targetRef == nil {
		// FlightRecorder status must not have been updated yet
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	cjfr, err := r.FindContainerJFR(ctx, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Keep client open to Container JFR as long as it doesn't fail
	if !r.IsClientConnected() {
		err := r.ConnectToContainerJFR(ctx, cjfr.Namespace, cjfr.Name)
		if err != nil {
			// Need Container JFR in order to reconcile anything, requeue until it appears
			return reconcile.Result{}, err
		}
	}

	// Look up pod corresponding to this FlightRecorder object
	targetPod := &corev1.Pod{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: targetRef.Namespace, Name: targetRef.Name}, targetPod)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Get a TargetAddress for this pod
	targetAddr, err := r.GetPodTarget(targetPod, instance.Status.Port)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Retrieve list of available events
	log.Info("Listing event types for pod", "name", targetPod.Name, "namespace", targetPod.Namespace)
	events, err := r.ListEventTypes(targetAddr)
	if err != nil {
		log.Error(err, "failed to list event types")
		r.CloseClient()
		return reconcile.Result{}, err
	}

	// Update Status with events
	instance.Status.Events = events
	err = r.Client.Status().Update(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("FlightRecorder successfully updated", "Namespace", instance.Namespace, "Name", instance.Name)
	return reconcile.Result{}, nil
}
