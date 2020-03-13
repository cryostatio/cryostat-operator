package recording

import (
	"context"
	"fmt"
	"time"

	rhjmcv1alpha2 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha2"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
	common "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_recording")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Recording Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRecording{scheme: mgr.GetScheme(),
		CommonReconciler: &common.CommonReconciler{
			Client: mgr.GetClient(),
		},
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
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha2.Recording{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileRecording implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileRecording{}

// ReconcileRecording reconciles a Recording object
type ReconcileRecording struct {
	scheme *runtime.Scheme
	*common.CommonReconciler
}

// Reconcile reads that state of the cluster for a Recording object and makes changes based on the state read
// and what is in the Recording.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileRecording) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Recording")

	cjfr, err := r.FindContainerJFR(ctx, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Keep client open to Container JFR as long as it doesn't fail
	if r.JfrClient == nil {
		jfrClient, err := r.ConnectToContainerJFR(ctx, cjfr.Namespace, cjfr.Name)
		if err != nil {
			// Need Container JFR in order to reconcile anything, requeue until it appears
			return reconcile.Result{}, err
		}
		r.JfrClient = jfrClient
	}

	// Fetch the Recording instance
	instance := &rhjmcv1alpha2.Recording{}
	err = r.Client.Get(ctx, request.NamespacedName, instance)
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

	jfrRef := instance.Spec.FlightRecorder
	if jfrRef == nil || len(jfrRef.Name) == 0 {
		// TODO set Condition for user/log error
		// Don't requeue until user fixes Recording
		return reconcile.Result{}, nil
	}

	// Look up FlightRecorder referenced by this Recording
	jfr := &rhjmcv1alpha2.FlightRecorder{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: jfrRef.Name}, jfr)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// TODO set Condition for user, could be legitimate if service is deleted
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Look up service corresponding to this FlightRecorder object
	targetRef := jfr.Status.Target
	if targetRef == nil {
		// FlightRecorder status must not have been updated yet
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	targetSvc := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: targetRef.Namespace, Name: targetRef.Name}, targetSvc)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Tell Container JFR to connect to the target service
	jfrclient.ClientLock.Lock()
	defer jfrclient.ClientLock.Unlock()
	err = r.ConnectToService(targetSvc, jfr.Status.Port)
	if err != nil {
		return reconcile.Result{}, err
	}
	defer r.DisconnectClient()

	// Tell Container JFR to create the recording if not already done
	if instance.Status.State == nil { // Recording hasn't been created yet
		if instance.Spec.Duration.Duration == time.Duration(0) {
			log.Info("creating new continuous recording", "name", instance.Spec.Name, "eventOptions", instance.Spec.EventOptions)
			err = r.JfrClient.StartRecording(instance.Spec.Name, instance.Spec.EventOptions)
		} else {
			log.Info("creating new recording", "name", instance.Spec.Name, "duration", instance.Spec.Duration, "eventOptions", instance.Spec.EventOptions)
			err = r.JfrClient.DumpRecording(instance.Spec.Name, int(instance.Spec.Duration.Seconds()), instance.Spec.EventOptions)
		}
		if err != nil {
			log.Error(err, "failed to create new recording")
			r.CloseClient() // TODO maybe track an error state in the client instead of relying on calling this everywhere
			return reconcile.Result{}, err
		}
	} else if shouldStopRecording(instance) {
		log.Info("stopping recording", "name", instance.Spec.Name)
		err = r.JfrClient.StopRecording(instance.Spec.Name)
		if err != nil {
			log.Error(err, "failed to stop recording")
			r.CloseClient() // TODO maybe track an error state in the client instead of relying on calling this everywhere
			return reconcile.Result{}, err
		}
	}

	// Get an updated list of in-memory flight recordings
	log.Info("Listing recordings for service", "service", targetSvc.Name, "namespace", targetSvc.Namespace)
	descriptors, err := r.JfrClient.ListRecordings()
	if err != nil {
		log.Error(err, "failed to list flight recordings")
		r.CloseClient()
		return reconcile.Result{}, err
	}

	// If the recording is found in Container JFR's list, update Recording.Status with the newest info
	descriptor := findRecordingByName(descriptors, instance.Spec.Name)
	if descriptor != nil {
		state, err := validateRecordingState(descriptor.State)
		if err != nil {
			// TODO inform user?
			return reconcile.Result{}, err
		}
		instance.Status.State = state
		instance.Status.StartTime = metav1.Unix(0, descriptor.StartTime*int64(time.Millisecond))
		instance.Status.Duration = metav1.Duration{
			Duration: time.Duration(descriptor.Duration) * time.Millisecond,
		}
	}

	// TODO Download URLs returned by Container JFR's 'list' command currently
	// work when it is connected to the target JVM. To work around this,
	// we only include links to recordings that have been archived in persistent
	// storage.

	// Archive completed recording if requested and not already done
	isStopped := instance.Status.State != nil && *instance.Status.State == rhjmcv1alpha2.RecordingStateStopped
	if instance.Spec.Archive && instance.Status.DownloadURL == nil && isStopped {
		filename, err := r.JfrClient.SaveRecording(instance.Spec.Name)
		if err != nil {
			log.Error(err, "failed to save recording", "name", instance.Spec.Name)
			r.CloseClient()
			return reconcile.Result{}, err
		}

		downloadURL, err := r.getDownloadURL(instance.Spec.Name, *filename)
		if err != nil {
			return reconcile.Result{}, err
		}
		instance.Status.DownloadURL = downloadURL
	}

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

func (r *ReconcileRecording) getDownloadURL(recordingName string, filename string) (*string, error) {
	// Look for our saved recording in list from Container JFR
	savedRecordings, err := r.JfrClient.ListSavedRecordings()
	if err != nil {
		log.Error(err, "failed to list saved flight recordings")
		r.CloseClient()
		return nil, err
	}
	for idx, saved := range savedRecordings {
		if filename == saved.Name {
			log.Info("updating download URL", "name", recordingName, "url", saved.DownloadURL)
			return &savedRecordings[idx].DownloadURL, nil
		}
	}
	return nil, nil
}

func findRecordingByName(descriptors []jfrclient.RecordingDescriptor, name string) *jfrclient.RecordingDescriptor {
	for idx, recording := range descriptors {
		if recording.Name == name {
			return &descriptors[idx]
		}
	}
	return nil
}

func validateRecordingState(state string) (*rhjmcv1alpha2.RecordingState, error) {
	convState := rhjmcv1alpha2.RecordingState(state)
	switch convState {
	case rhjmcv1alpha2.RecordingStateCreated,
		rhjmcv1alpha2.RecordingStateRunning,
		rhjmcv1alpha2.RecordingStateStopping,
		rhjmcv1alpha2.RecordingStateStopped:
		return &convState, nil
	}
	return nil, fmt.Errorf("unknown recording state %s", state)
}

func shouldStopRecording(recording *rhjmcv1alpha2.Recording) bool {
	// Need to know user's request, and current state of recording
	requested := recording.Spec.State
	current := recording.Status.State
	if requested == nil || current == nil {
		return false
	}

	// Should stop if user wants recording stopped and we're not already doing/done so
	return *requested == rhjmcv1alpha2.RecordingStateStopped && *current != rhjmcv1alpha2.RecordingStateStopped &&
		*current != rhjmcv1alpha2.RecordingStateStopping
}
