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

package recording

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"time"

	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	common "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var log = logf.Log.WithName("controller_recording")

// Add creates a new Recording Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRecording{Scheme: mgr.GetScheme(), Client: mgr.GetClient(),
		Reconciler: common.NewReconciler(&common.ReconcilerConfig{
			Client: mgr.GetClient(),
		}),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("recording-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Recording
	err = c.Watch(&source.Kind{Type: &rhjmcv1beta1.Recording{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRecording implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRecording{}

// ReconcileRecording reconciles a Recording object
type ReconcileRecording struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client client.Client
	Scheme *runtime.Scheme
	common.Reconciler
}

// Name used for Finalizer that handles Container JFR recording deletion
const recordingFinalizer = "recording.finalizer.rhjmc.redhat.com"

// Reconcile reads that state of the cluster for a Recording object and makes changes based on the state read
// and what is in the Recording.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRecording) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
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
		if instance.GetDeletionTimestamp() != nil && hasRecordingFinalizer(instance) {
			// Allow deletion to proceed, since no FlightRecorder/Pod to clean up
			log.Info("no matching FlightRecorder, proceeding with recording deletion")
			r.removeRecordingFinalizer(ctx, instance)
		}
		// No matching FlightRecorder, don't requeue until FlightRecorder field is fixed
		return reconcile.Result{}, nil
	}

	// Obtain a client configured to communicate with Container JFR
	cjfr, err := r.GetContainerJFRClient(ctx, request.Namespace, jfr.Spec.JMXCredentials)
	if err != nil {
		if err == common.ErrCertNotReady {
			log.Info("Waiting for CA certificate")
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return reconcile.Result{}, err
	}

	// Check if this Recording is being deleted
	if instance.GetDeletionTimestamp() != nil && hasRecordingFinalizer(instance) {
		// Delete any persisted JFR file for this recording
		err := r.removeSavedRecording(cjfr, instance)
		if err != nil {
			reqLogger.Error(err, "failed to delete saved recording in Container JFR", "namespace",
				instance.Namespace, "name", instance.Name)
			return reconcile.Result{}, err
		}
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
		if hasRecordingFinalizer(instance) {
			// Delete in-memory recording in Container JFR
			err := removeRecording(cjfr, targetAddr, instance)
			if err != nil {
				log.Error(err, "failed to delete recording in Container JFR", "namespace", instance.Namespace,
					"name", instance.Name)
				return reconcile.Result{}, err
			}

			// Remove our finalizer only once our cleanup logic has succeeded
			err = r.removeRecordingFinalizer(ctx, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		// Ready for deletion
		return reconcile.Result{}, nil
	}

	// Add our finalizer, so we can clean up Container JFR resources upon deletion
	if !hasRecordingFinalizer(instance) {
		err := r.addRecordingFinalizer(ctx, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Tell Container JFR to create the recording if not already done
	if instance.Status.State == nil { // Recording hasn't been created yet
		if instance.Spec.Duration.Duration == time.Duration(0) {
			log.Info("creating new continuous recording", "name", instance.Spec.Name, "eventOptions", instance.Spec.EventOptions)
			err = cjfr.StartRecording(targetAddr, instance.Spec.Name, instance.Spec.EventOptions)
		} else {
			log.Info("creating new recording", "name", instance.Spec.Name, "duration", instance.Spec.Duration, "eventOptions", instance.Spec.EventOptions)
			err = cjfr.DumpRecording(targetAddr, instance.Spec.Name, int(instance.Spec.Duration.Seconds()), instance.Spec.EventOptions)
		}
		if err != nil {
			log.Error(err, "failed to create new recording")
			return reconcile.Result{}, err
		}
	} else if shouldStopRecording(instance) {
		log.Info("stopping recording", "name", instance.Spec.Name)
		err = cjfr.StopRecording(targetAddr, instance.Spec.Name)
		if err != nil {
			log.Error(err, "failed to stop recording")
			return reconcile.Result{}, err
		}
	}

	// If the recording is found in Container JFR's list, update Recording.Status with the newest info
	log.Info("Looking for recordings for pod", "pod", targetPod.Name, "namespace", targetPod.Namespace)
	// Updated Download URL, use existing URL as default
	downloadURL := instance.Status.DownloadURL
	descriptor, err := findRecordingByName(cjfr, targetAddr, instance.Spec.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	if descriptor != nil {
		state, err := validateRecordingState(descriptor.State)
		if err != nil {
			// TODO Likely an internal error, requeuing may not help. Status.Condition may be useful.
			log.Error(err, "unknown recording state observed from Container JFR")
			return reconcile.Result{}, err
		}
		instance.Status.State = state
		instance.Status.StartTime = metav1.Unix(0, descriptor.StartTime*int64(time.Millisecond))
		instance.Status.Duration = metav1.Duration{
			Duration: time.Duration(descriptor.Duration) * time.Millisecond,
		}
		downloadURL = &descriptor.DownloadURL
	}

	// Archive completed recording if requested and not already done
	isStopped := instance.Status.State != nil && *instance.Status.State == rhjmcv1beta1.RecordingStateStopped
	if instance.Spec.Archive && isStopped {
		url, err := archiveStoppedRecording(cjfr, instance, targetAddr)
		if err != nil {
			return reconcile.Result{}, err
		} else if url == nil {
			// Unlikely, but log just in case
			log.Info("Cannot find JFR URL just saved", "name", instance.Spec.Name)
		} else {
			log.Info("updating download URL", "name", instance.Spec.Name, "url", url)
			downloadURL = url
		}
	}
	instance.Status.DownloadURL = downloadURL

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

func (r *ReconcileRecording) getFlightRecorder(ctx context.Context, recording *rhjmcv1beta1.Recording) (*rhjmcv1beta1.FlightRecorder, error) {
	jfrRef := recording.Spec.FlightRecorder
	if jfrRef == nil || len(jfrRef.Name) == 0 {
		// TODO set Condition for user/log error
		log.Info("FlightRecorder reference missing from Recording", "name", recording.Name,
			"namespace", recording.Namespace)
		return nil, nil
	}

	jfr := &rhjmcv1beta1.FlightRecorder{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: recording.Namespace, Name: jfrRef.Name}, jfr)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// TODO set Condition for user, could be legitimate if pod is deleted
			log.Info("FlightRecorder referenced from Recording not found", "name", jfrRef.Name,
				"namespace", recording.Namespace)
			return nil, nil
		}
		return nil, err
	}
	return jfr, nil
}

func findDownloadURL(cjfr jfrclient.ContainerJfrClient, filename string) (*string, error) {
	// Look for our saved recording in list from Container JFR
	savedRecordings, err := cjfr.ListSavedRecordings()
	if err != nil {
		log.Error(err, "failed to list saved flight recordings")
		return nil, err
	}
	for idx, saved := range savedRecordings {
		if filename == saved.Name {
			return &savedRecordings[idx].DownloadURL, nil
		}
	}
	return nil, nil
}

func archiveStoppedRecording(cjfr jfrclient.ContainerJfrClient, recording *rhjmcv1beta1.Recording,
	target *jfrclient.TargetAddress) (*string, error) {
	// Check if existing download URL points to an archived recording
	jfrFile, err := recordingFilename(*recording.Status.DownloadURL)
	if err != nil {
		return nil, err
	}

	existing, err := findDownloadURL(cjfr, *jfrFile)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		// Use already archived recording as download URL
		return existing, nil
	}

	// Recording hasn't been archived yet, do so now
	log.Info("saving recording", "name", recording.Spec.Name)
	filename, err := cjfr.SaveRecording(target, recording.Spec.Name)
	if err != nil {
		log.Error(err, "failed to save recording", "name", recording.Spec.Name)
		return nil, err
	}

	// Look up full URL for filename returned by SaveRecording
	return findDownloadURL(cjfr, *filename)
}

func removeRecording(cjfr jfrclient.ContainerJfrClient, target *jfrclient.TargetAddress,
	recording *rhjmcv1beta1.Recording) error {
	// Check if recording exists in Container JFR's in-memory list
	recName := recording.Spec.Name
	found, err := findRecordingByName(cjfr, target, recName)
	if err != nil {
		return err
	}
	if found != nil {
		// Found matching recording, delete it
		err = cjfr.DeleteRecording(target, recName)
		if err != nil {
			return err
		}
		log.Info("recording successfully deleted", "name", recName)
	}
	return nil
}

func (r *ReconcileRecording) removeSavedRecording(cjfr jfrclient.ContainerJfrClient,
	recording *rhjmcv1beta1.Recording) error {
	if recording.Status.DownloadURL != nil {
		jfrFile, err := recordingFilename(*recording.Status.DownloadURL)
		if err != nil {
			return err
		}
		// Look for this JFR file within Container JFR's list of saved recordings
		found, err := findDownloadURL(cjfr, *jfrFile)
		if err != nil {
			return err
		}

		if found != nil {
			// JFR file exists, so delete it
			err = cjfr.DeleteSavedRecording(*jfrFile)
			if err != nil {
				return err
			}
			log.Info("saved recording successfully deleted", "file", jfrFile)
		}
	}
	return nil
}

func (r *ReconcileRecording) addRecordingFinalizer(ctx context.Context, recording *rhjmcv1beta1.Recording) error {
	log.Info("adding finalizer for recording", "namespace", recording.Namespace, "name", recording.Name)
	finalizers := append(recording.GetFinalizers(), recordingFinalizer)
	recording.SetFinalizers(finalizers)

	err := r.Client.Update(ctx, recording)
	if err != nil {
		log.Error(err, "failed to add finalizer to recording", "namespace", recording.Namespace,
			"name", recording.Name)
		return err
	}
	return nil
}

func (r *ReconcileRecording) removeRecordingFinalizer(ctx context.Context, recording *rhjmcv1beta1.Recording) error {
	finalizers := recording.GetFinalizers()
	foundIdx := -1
	for idx, finalizer := range finalizers {
		if finalizer == recordingFinalizer {
			foundIdx = idx
			break
		}
	}

	if foundIdx >= 0 {
		// Remove our finalizer from the slice
		finalizers = append(finalizers[:foundIdx], finalizers[foundIdx+1:]...)
		recording.SetFinalizers(finalizers)
		err := r.Client.Update(ctx, recording)
		if err != nil {
			log.Error(err, "failed to remove finalizer from recording", "namespace", recording.Namespace,
				"name", recording.Name)
			return err
		}
	}
	return nil
}

func findRecordingByName(cjfr jfrclient.ContainerJfrClient, target *jfrclient.TargetAddress,
	name string) (*jfrclient.RecordingDescriptor, error) {
	// Get an updated list of in-memory flight recordings
	descriptors, err := cjfr.ListRecordings(target)
	if err != nil {
		log.Error(err, "failed to list flight recordings", "name", name)
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

func hasRecordingFinalizer(recording *rhjmcv1beta1.Recording) bool {
	for _, finalizer := range recording.GetFinalizers() {
		if finalizer == recordingFinalizer {
			return true
		}
	}
	return false
}
