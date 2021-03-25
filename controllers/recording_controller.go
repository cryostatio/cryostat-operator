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

	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/api/v1beta1"

	"fmt"
	"net/url"
	"path"
	"time"

	jfrclient "github.com/rh-jmc-team/container-jfr-operator/controllers/client"
	common "github.com/rh-jmc-team/container-jfr-operator/controllers/common"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// RecordingReconciler reconciles a Recording object
type RecordingReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	common.Reconciler
}

// Name used for Finalizer that handles Container JFR recording deletion
const recordingFinalizer = "rhjmc.redhat.com/recording.finalizer"

// +kubebuilder:rbac:namespace=system,groups="",resources=pods;services;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:namespace=system,groups=cert-manager.io,resources=issuers;certificates,verbs=create;get;list;update;watch
// +kubebuilder:rbac:namespace=system,groups=rhjmc.redhat.com,resources=recordings;flightrecorders;containerjfrs,verbs=*
// +kubebuilder:rbac:namespace=system,groups=rhjmc.redhat.com,resources=recordings/status,verbs=get;update;patch
// +kubebuilder:rbac:namespace=system,groups=rhjmc.redhat.com,resources=recordings/finalizers,verbs=update

// Reconcile processes a Recording and communicates with Container JFR to create and manage
// a corresponding JDK Flight Recording
func (r *RecordingReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Recording")

	// Fetch the Recording instance
	instance := &rhjmcv1beta1.Recording{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Recording does not exist")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Look up FlightRecorder referenced by this Recording
	jfr, err := r.getFlightRecorder(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}
	if jfr == nil {
		// Check if this Recording is being deleted
		if instance.GetDeletionTimestamp() != nil && controllerutil.ContainsFinalizer(instance, recordingFinalizer) {
			return r.deleteWithoutLiveTarget(ctx, instance)
		}
		// No matching FlightRecorder, its corresponding Pod might have been deleted
		return reconcile.Result{}, nil
	}

	// Obtain a client configured to communicate with Container JFR
	cjfr, err := r.GetContainerJFRClient(ctx, request.Namespace, jfr.Spec.JMXCredentials)
	if err != nil {
		return r.requeueIfNotReady(err)
	}

	// Look up pod corresponding to this FlightRecorder object
	targetRef := jfr.Status.Target
	if targetRef == nil {
		// FlightRecorder status must not have been updated yet
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	targetPod := &corev1.Pod{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: targetRef.Namespace, Name: targetRef.Name}, targetPod)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Get TargetAddress for the referenced pod and port number listed in FlightRecorder
	targetAddr, err := r.GetPodTarget(targetPod, jfr.Status.Port)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Recording is being deleted
	if instance.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(instance, recordingFinalizer) {
			return r.deleteWithLiveTarget(ctx, cjfr, instance, targetAddr)
		}
		// Ready for deletion
		return reconcile.Result{}, nil
	}

	// Add our finalizer, so we can clean up Container JFR resources upon deletion
	if !controllerutil.ContainsFinalizer(instance, recordingFinalizer) {
		err := common.AddFinalizer(ctx, r.Client, instance, recordingFinalizer)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Tell Container JFR to create the recording if not already done
	if instance.Status.State == nil { // Recording hasn't been created yet
		if instance.Spec.Duration.Duration == time.Duration(0) {
			r.Log.Info("creating new continuous recording", "name", instance.Spec.Name, "eventOptions", instance.Spec.EventOptions)
			err = cjfr.StartRecording(targetAddr, instance.Spec.Name, instance.Spec.EventOptions)
		} else {
			r.Log.Info("creating new recording", "name", instance.Spec.Name, "duration", instance.Spec.Duration, "eventOptions", instance.Spec.EventOptions)
			err = cjfr.DumpRecording(targetAddr, instance.Spec.Name, int(instance.Spec.Duration.Seconds()), instance.Spec.EventOptions)
		}
		if err != nil {
			r.Log.Error(err, "failed to create new recording")
			return reconcile.Result{}, err
		}
	} else if shouldStopRecording(instance) {
		r.Log.Info("stopping recording", "name", instance.Spec.Name)
		err = cjfr.StopRecording(targetAddr, instance.Spec.Name)
		if err != nil {
			r.Log.Error(err, "failed to stop recording")
			return reconcile.Result{}, err
		}
	}

	// If the recording is found in Container JFR's list, update Recording.Status with the newest info
	r.Log.Info("Looking for recordings for pod", "pod", targetPod.Name, "namespace", targetPod.Namespace)
	// Updated Download URL, use existing URL as default
	downloadURL := instance.Status.DownloadURL
	reportURL := instance.Status.ReportURL
	descriptor, err := r.findRecordingByName(cjfr, targetAddr, instance.Spec.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	if descriptor != nil {
		state, err := validateRecordingState(descriptor.State)
		if err != nil {
			// TODO Likely an internal error, requeuing may not help. Status.Condition may be useful.
			r.Log.Error(err, "unknown recording state observed from Container JFR")
			return reconcile.Result{}, err
		}
		instance.Status.State = state
		instance.Status.StartTime = metav1.Unix(0, descriptor.StartTime*int64(time.Millisecond))
		instance.Status.Duration = metav1.Duration{
			Duration: time.Duration(descriptor.Duration) * time.Millisecond,
		}
		downloadURL = &descriptor.DownloadURL
		reportURL = &descriptor.ReportURL
	}

	// Archive completed recording if requested and not already done
	isStopped := instance.Status.State != nil && *instance.Status.State == rhjmcv1beta1.RecordingStateStopped
	if instance.Spec.Archive && isStopped {
		recording, err := r.archiveStoppedRecording(cjfr, instance, targetAddr)
		if err != nil {
			return reconcile.Result{}, err
		} else if recording == nil {
			// Unlikely, but log just in case
			r.Log.Info("Cannot find JFR URL just saved", "name", instance.Spec.Name)
		} else {
			r.Log.Info("updating download URL", "name", instance.Spec.Name, "url", &recording.DownloadURL)
			downloadURL = &recording.DownloadURL
			r.Log.Info("updating report URL", "name", instance.Spec.Name, "url", &recording.ReportURL)
			reportURL = &recording.ReportURL
		}
	}
	instance.Status.DownloadURL = downloadURL
	instance.Status.ReportURL = reportURL

	// Update Recording status
	err = r.Client.Status().Update(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Requeue if the recording is still in progress
	result := reconcile.Result{}
	if !isStopped {
		// Check progress of recording after 10 seconds
		result.RequeueAfter = 10 * time.Second
	}

	reqLogger.Info("Recording successfully updated", "Namespace", instance.Namespace, "Name", instance.Name)
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecordingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := ctrl.NewControllerManagedBy(mgr)
	c = c.For(&rhjmcv1beta1.Recording{})
	c = r.watchFlightRecorders(c, mgr.GetClient())

	return c.Complete(r)
}

func (r *RecordingReconciler) getFlightRecorder(ctx context.Context, recording *rhjmcv1beta1.Recording) (*rhjmcv1beta1.FlightRecorder, error) {
	jfrRef := recording.Spec.FlightRecorder
	if jfrRef == nil || len(jfrRef.Name) == 0 {
		// TODO set Condition for user/log error
		r.Log.Info("FlightRecorder reference missing from Recording", "name", recording.Name,
			"namespace", recording.Namespace)
		return nil, nil
	}

	// Apply FlightRecorder label if not already present
	err := r.applyFlightRecorderLabel(ctx, recording, jfrRef.Name)
	if err != nil {
		return nil, err
	}

	jfr := &rhjmcv1beta1.FlightRecorder{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: recording.Namespace, Name: jfrRef.Name}, jfr)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// TODO set Condition for user, could be legitimate if pod is deleted
			r.Log.Info("FlightRecorder referenced from Recording not found", "name", jfrRef.Name,
				"namespace", recording.Namespace)
			return nil, nil
		}
		return nil, err
	}
	return jfr, nil
}

func (r *RecordingReconciler) findSavedRecording(cjfr jfrclient.ContainerJfrClient, filename string) (*jfrclient.SavedRecording, error) {
	// Look for our saved recording in list from Container JFR
	savedRecordings, err := cjfr.ListSavedRecordings()
	if err != nil {
		r.Log.Error(err, "failed to list saved flight recordings")
		return nil, err
	}
	for idx, saved := range savedRecordings {
		if filename == saved.Name {
			return &savedRecordings[idx], nil
		}
	}
	return nil, nil
}

func (r *RecordingReconciler) archiveStoppedRecording(cjfr jfrclient.ContainerJfrClient, recording *rhjmcv1beta1.Recording,
	target *jfrclient.TargetAddress) (*jfrclient.SavedRecording, error) {
	// Check if existing download URL points to an archived recording
	jfrFile, err := recordingFilename(*recording.Status.DownloadURL)
	if err != nil {
		return nil, err
	}

	savedRecording, err := r.findSavedRecording(cjfr, *jfrFile)
	if err != nil {
		return nil, err
	}
	if savedRecording != nil {
		// Use already archived recording
		return savedRecording, nil
	}

	// Recording hasn't been archived yet, do so now
	r.Log.Info("saving recording", "name", recording.Spec.Name)
	filename, err := cjfr.SaveRecording(target, recording.Spec.Name)
	if err != nil {
		r.Log.Error(err, "failed to save recording", "name", recording.Spec.Name)
		return nil, err
	}

	// Look up full URL for filename returned by SaveRecording
	return r.findSavedRecording(cjfr, *filename)
}

func (r *RecordingReconciler) removeRecording(cjfr jfrclient.ContainerJfrClient, target *jfrclient.TargetAddress,
	recording *rhjmcv1beta1.Recording) error {
	// Check if recording exists in Container JFR's in-memory list
	recName := recording.Spec.Name
	found, err := r.findRecordingByName(cjfr, target, recName)
	if err != nil {
		return err
	}
	if found != nil {
		// Found matching recording, delete it
		err = cjfr.DeleteRecording(target, recName)
		if err != nil {
			return err
		}
		r.Log.Info("recording successfully deleted", "name", recName)
	}
	return nil
}

func (r *RecordingReconciler) removeSavedRecording(cjfr jfrclient.ContainerJfrClient,
	recording *rhjmcv1beta1.Recording) error {
	if recording.Status.DownloadURL != nil {
		jfrFile, err := recordingFilename(*recording.Status.DownloadURL)
		if err != nil {
			return err
		}
		// Look for this JFR file within Container JFR's list of saved recordings
		found, err := r.findSavedRecording(cjfr, *jfrFile)
		if err != nil {
			return err
		}

		if found != nil {
			// JFR file exists, so delete it
			err = cjfr.DeleteSavedRecording(*jfrFile)
			if err != nil {
				return err
			}
			r.Log.Info("saved recording successfully deleted", "file", jfrFile)
		}
	}
	return nil
}

func (r *RecordingReconciler) applyFlightRecorderLabel(ctx context.Context, recording *rhjmcv1beta1.Recording,
	jfrName string) error {
	// Set label if not present or contains the wrong FlightRecorder name
	labels := recording.GetLabels()
	if labels[rhjmcv1beta1.RecordingLabel] != jfrName {
		// Initialize labels map if recording has no labels
		if labels == nil {
			labels = map[string]string{}
		}
		labels[rhjmcv1beta1.RecordingLabel] = jfrName
		recording.SetLabels(labels)

		err := r.Client.Update(ctx, recording)
		if err != nil {
			return err
		}
		r.Log.Info("added label for recording", "namespace", recording.Namespace, "name", recording.Name)
	}
	return nil
}

func (r *RecordingReconciler) deleteWithoutLiveTarget(ctx context.Context, recording *rhjmcv1beta1.Recording) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", recording.Namespace, "Request.Name", recording.Name)
	reqLogger.Info("no matching FlightRecorder, proceeding with recording deletion")

	// Obtain a client configured to communicate with Container JFR without JMX credentials
	cjfr, err := r.GetContainerJFRClient(ctx, recording.Namespace, nil)
	if err != nil {
		return r.requeueIfNotReady(err)
	}

	// Delete any persisted JFR file for this recording
	err = r.removeSavedRecording(cjfr, recording)
	if err != nil {
		reqLogger.Error(err, "failed to delete saved recording in Container JFR")
		return reconcile.Result{}, err
	}

	// Allow deletion to proceed, since no FlightRecorder/Pod to clean up
	err = common.RemoveFinalizer(ctx, r.Client, recording, recordingFinalizer)
	return reconcile.Result{}, err
}

func (r *RecordingReconciler) deleteWithLiveTarget(ctx context.Context, cjfr jfrclient.ContainerJfrClient,
	recording *rhjmcv1beta1.Recording, target *jfrclient.TargetAddress) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", recording.Namespace, "Request.Name", recording.Name)
	// Delete any persisted JFR file for this recording
	err := r.removeSavedRecording(cjfr, recording)
	if err != nil {
		reqLogger.Error(err, "failed to delete saved recording in Container JFR")
		return reconcile.Result{}, err
	}

	// Delete in-memory recording in Container JFR
	err = r.removeRecording(cjfr, target, recording)
	if err != nil {
		reqLogger.Error(err, "failed to delete recording in Container JFR")
		return reconcile.Result{}, err
	}

	// Remove our finalizer only once our cleanup logic has succeeded
	err = common.RemoveFinalizer(ctx, r.Client, recording, recordingFinalizer)
	return reconcile.Result{}, err
}

func (r *RecordingReconciler) watchFlightRecorders(builder *builder.Builder, cl client.Client) *builder.Builder {
	ctx := context.Background()
	jfrPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
	}

	mapFunc := func(obj client.Object) []reconcile.Request {
		// Look up all recordings that reference the changed FlightRecorder
		recordings := &rhjmcv1beta1.RecordingList{}
		selector := labels.SelectorFromSet(labels.Set{
			rhjmcv1beta1.RecordingLabel: obj.GetName(),
		})
		err := cl.List(ctx, recordings, &client.ListOptions{
			Namespace:     obj.GetNamespace(),
			LabelSelector: selector,
		})
		if err != nil {
			r.Log.Error(err, "Failed to list Recordings", "namespace", obj.GetNamespace(),
				"selector", selector.String())
		}

		// Reconcile each recording that was found
		requests := make([]reconcile.Request, len(recordings.Items))
		for idx, recording := range recordings.Items {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: recording.Namespace,
					Name:      recording.Name,
				},
			}
			requests[idx] = request
		}
		return requests
	}

	return builder.Watches(
		&source.Kind{Type: &rhjmcv1beta1.FlightRecorder{}},
		handler.EnqueueRequestsFromMapFunc(mapFunc),
	).WithEventFilter(
		jfrPredicate,
	)
}

func (r *RecordingReconciler) findRecordingByName(cjfr jfrclient.ContainerJfrClient, target *jfrclient.TargetAddress,
	name string) (*jfrclient.RecordingDescriptor, error) {
	// Get an updated list of in-memory flight recordings
	descriptors, err := cjfr.ListRecordings(target)
	if err != nil {
		r.Log.Error(err, "failed to list flight recordings", "name", name)
		return nil, err
	}

	for idx, recording := range descriptors {
		if recording.Name == name {
			return &descriptors[idx], nil
		}
	}
	return nil, nil
}

func validateRecordingState(state string) (*rhjmcv1beta1.RecordingState, error) {
	convState := rhjmcv1beta1.RecordingState(state)
	switch convState {
	case rhjmcv1beta1.RecordingStateCreated,
		rhjmcv1beta1.RecordingStateRunning,
		rhjmcv1beta1.RecordingStateStopping,
		rhjmcv1beta1.RecordingStateStopped:
		return &convState, nil
	}
	return nil, fmt.Errorf("unknown recording state %s", state)
}

func recordingFilename(recordingURL string) (*string, error) {
	// Grab JFR file base name
	downloadURL, err := url.Parse(recordingURL)
	if err != nil {
		return nil, err
	}
	jfrFile := path.Base(downloadURL.Path)
	return &jfrFile, nil
}

func shouldStopRecording(recording *rhjmcv1beta1.Recording) bool {
	// Need to know user's request, and current state of recording
	requested := recording.Spec.State
	current := recording.Status.State
	if requested == nil || current == nil {
		return false
	}

	// Should stop if user wants recording stopped and we're not already doing/done so
	return *requested == rhjmcv1beta1.RecordingStateStopped && *current != rhjmcv1beta1.RecordingStateStopped &&
		*current != rhjmcv1beta1.RecordingStateStopping
}

func (r *RecordingReconciler) requeueIfNotReady(err error) (reconcile.Result, error) {
	if err == common.ErrCertNotReady {
		r.Log.Info("Waiting for CA certificate")
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return reconcile.Result{}, err
}
