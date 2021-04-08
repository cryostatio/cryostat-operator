// Copyright (c) 2021 Red Hat, Inc.
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

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"time"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	common "github.com/cryostatio/cryostat-operator/controllers/common"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// FlightRecorderReconciler reconciles a FlightRecorder object
type FlightRecorderReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	common.Reconciler
}

// +kubebuilder:rbac:namespace=system,groups="",resources=pods;services;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:namespace=system,groups=cert-manager.io,resources=issuers;certificates,verbs=create;get;list;update;watch
// +kubebuilder:rbac:namespace=system,groups=operator.cryostat.io,resources=cryostats;flightrecorders,verbs=*
// +kubebuilder:rbac:namespace=system,groups=operator.cryostat.io,resources=flightrecorders/status,verbs=get;update;patch

// Reconcile processes a FlightRecorder CR and retrieves event/template information from Cryostat
func (r *FlightRecorderReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling FlightRecorder")

	// Fetch the FlightRecorder instance
	instance := &operatorv1beta1.FlightRecorder{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("FlightRecorder does not exist")
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

	// Obtain a client configured to communicate with Cryostat
	cryostat, err := r.GetCryostatClient(ctx, request.Namespace, instance.Spec.JMXCredentials)
	if err != nil {
		if err == common.ErrCertNotReady {
			reqLogger.Info("Waiting for CA certificate")
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return reconcile.Result{}, err
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
	reqLogger.Info("Listing event types for pod", "name", targetPod.Name, "namespace", targetPod.Namespace)
	events, err := cryostat.ListEventTypes(targetAddr)
	if err != nil {
		reqLogger.Error(err, "failed to list event types")
		return reconcile.Result{}, err
	}

	// Update Status with events
	instance.Status.Events = events

	// Retrieve list of available templates
	reqLogger.Info("Listing templates for pod", "name", targetPod.Name, "namespace", targetPod.Namespace)
	templates, err := cryostat.ListTemplates(targetAddr)
	if err != nil {
		reqLogger.Error(err, "failed to list templates")
		return reconcile.Result{}, err
	}

	// Update Status with templates
	instance.Status.Templates = templates

	err = r.Client.Status().Update(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("FlightRecorder successfully updated", "Namespace", instance.Namespace, "Name", instance.Name)
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FlightRecorderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1beta1.FlightRecorder{}).
		Complete(r)
}
