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
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/api/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/controllers/common"
	resources "github.com/rh-jmc-team/container-jfr-operator/controllers/common/resource_definitions"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
)

// EndpointsReconciler reconciles a Endpoints object
type EndpointsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	common.Reconciler
}

// +kubebuilder:rbac:groups=rhjmc.redhat.com,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rhjmc.redhat.com,resources=endpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rhjmc.redhat.com,resources=endpoints/finalizers,verbs=update
func (r *EndpointsReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Endpoints")

	// Fetch the Endpoints instance
	ep := &corev1.Endpoints{}
	err := r.Client.Get(ctx, request.NamespacedName, ep)
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

	for _, subset := range ep.Subsets {
		// Check if this subset appears to be compatible with Container JFR
		jmxPort := getServiceJMXPort(subset)

		if jmxPort != nil {
			for _, address := range subset.Addresses {
				target := address.TargetRef
				if target != nil && target.Kind == "Pod" {
					err := r.handlePodAddress(ctx, target, ep, jmxPort, reqLogger)
					if err != nil {
						return reconcile.Result{}, err
					}
				}
			}
		}
	}

	reqLogger.Info("Endpoints successfully reconciled", "Namespace", request.Namespace, "Name", request.Name)
	return reconcile.Result{}, nil
}

func (r *EndpointsReconciler) handlePodAddress(ctx context.Context, target *corev1.ObjectReference,
	ep *corev1.Endpoints, jmxPort *int32, reqLogger logr.Logger) error {
	// Check if this FlightRecorder already exists
	found := &rhjmcv1beta1.FlightRecorder{}
	jfrName := target.Name

	err := r.Client.Get(ctx, types.NamespacedName{Name: jfrName, Namespace: target.Namespace}, found)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}

		// If this Endpoints is for Container JFR itself, fill in the JMX authentication credentials
		// that the operator generated
		jmxAuth, err := r.getJMXCredentials(ctx, ep)
		if err != nil {
			return err
		}

		reqLogger.Info("Creating a new FlightRecorder", "Namespace", target.Namespace, "Name", jfrName)
		err = r.createNewFlightRecorder(ctx, target, jmxPort, jmxAuth)
		if err != nil {
			return err
		}
	}
	return nil
}

const defaultContainerJFRPort int32 = 9091
const jmxServicePortName = "jfr-jmx"

func getServiceJMXPort(subset corev1.EndpointSubset) *int32 {
	var portNum, fallbackPortNum *int32
	for idx, port := range subset.Ports {
		if port.Name == jmxServicePortName {
			portNum = &subset.Ports[idx].Port
		} else if port.Port == defaultContainerJFRPort {
			fallbackPortNum = &subset.Ports[idx].Port
		}
	}
	if portNum == nil && fallbackPortNum != nil {
		portNum = fallbackPortNum
	}
	return portNum
}

func (r *EndpointsReconciler) createNewFlightRecorder(ctx context.Context, target *corev1.ObjectReference, jmxPort *int32,
	jmxAuth *rhjmcv1beta1.JMXAuthSecret) error {
	pod := &corev1.Pod{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, pod)
	if err != nil {
		return err
	}

	// Define a new FlightRecorder object for this Pod
	jfr, err := r.newFlightRecorderForPod(target, pod, *jmxPort, jmxAuth)
	if err != nil {
		return err
	}

	// Set Pod instance as the owner
	ownerRef := metav1.OwnerReference{
		APIVersion: pod.APIVersion,
		Kind:       pod.Kind,
		UID:        pod.UID,
		Name:       pod.Name,
	}
	jfr.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	err = r.Client.Create(ctx, jfr)
	if err != nil {
		return err
	}
	// Update FlightRecorder Status
	err = r.Client.Status().Update(ctx, jfr)
	if err != nil {
		return err
	}

	return nil
}

// newFlightRecorderForPod returns a FlightRecorder with the same name/namespace as the target
func (r *EndpointsReconciler) newFlightRecorderForPod(target *corev1.ObjectReference, pod *corev1.Pod,
	jmxPort int32, jmxAuth *rhjmcv1beta1.JMXAuthSecret) (*rhjmcv1beta1.FlightRecorder, error) {
	// Inherit "app" label from endpoints
	appLabel := pod.Name // Use endpoints name as fallback
	if label, pres := pod.Labels["app"]; pres {
		appLabel = label
	}
	labels := map[string]string{
		"app": appLabel,
	}

	// Use label selector matching the name of this FlightRecorder
	selector := &metav1.LabelSelector{}
	selector = metav1.AddLabelToSelector(selector, rhjmcv1beta1.RecordingLabel, target.Name)

	return &rhjmcv1beta1.FlightRecorder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      target.Name,
			Namespace: target.Namespace,
			Labels:    labels,
		},
		Spec: rhjmcv1beta1.FlightRecorderSpec{
			RecordingSelector: selector,
			JMXCredentials:    jmxAuth,
		},
		Status: rhjmcv1beta1.FlightRecorderStatus{
			Events:    []rhjmcv1beta1.EventInfo{},
			Templates: []rhjmcv1beta1.TemplateInfo{},
			Target:    target,
			Port:      jmxPort,
		},
	}, nil
}

func (r *EndpointsReconciler) getJMXCredentials(ctx context.Context, ep *corev1.Endpoints) (*rhjmcv1beta1.JMXAuthSecret, error) {
	// Look up the ContainerJFR CR in this namespace
	cjfr, err := r.FindContainerJFR(ctx, ep.Namespace)
	if err != nil {
		return nil, err
	}

	// Get service corresponding to this Endpoints
	svc := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: ep.Name, Namespace: ep.Namespace}, svc)
	if err != nil {
		return nil, err
	}

	// Is the service owned by the ContainerJFR CR
	var result *rhjmcv1beta1.JMXAuthSecret
	if metav1.IsControlledBy(svc, cjfr) {
		// Look up JMX auth secret created for this ContainerJFR
		secret := &corev1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: cjfr.Name + resources.JMXSecretNameSuffix,
			Namespace: cjfr.Namespace}, secret)
		if err != nil {
			return nil, err
		}

		// Found the JMX auth secret, fill in corresponding values for FlightRecorder
		userKey := resources.JMXSecretUserKey
		passKey := resources.JMXSecretPassKey
		result = &rhjmcv1beta1.JMXAuthSecret{
			SecretName:  secret.Name,
			UsernameKey: &userKey,
			PasswordKey: &passKey,
		}
	}

	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Endpoints{}).
		Complete(r)
}
