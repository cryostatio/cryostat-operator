package flightrecorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	jfrclient "github.com/rh-jmc-team/container-jfr-operator/pkg/client"
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

var log = logf.Log.WithName("controller_flightrecorder")

// Add creates a new FlightRecorder Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileFlightRecorder{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("flightrecorder-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource FlightRecorder
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha1.FlightRecorder{}}, &handler.EnqueueRequestForObject{})
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
	client    client.Client
	scheme    *runtime.Scheme
	jfrClient *jfrclient.ContainerJfrClient
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

	// Keep client open to Container JFR as long as it doesn't fail
	if r.jfrClient == nil {
		jfrClient, err := r.connectToContainerJFR(ctx, request.Namespace)
		if err != nil {
			// Need Container JFR in order to reconcile anything, requeue until it appears
			return reconcile.Result{}, err
		}
		r.jfrClient = jfrClient
	}

	// Fetch the FlightRecorder instance
	instance := &rhjmcv1alpha1.FlightRecorder{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
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

	// Look up service corresponding to this FlightRecorder object
	targetRef := instance.Status.Target
	if targetRef == nil {
		// FlightRecorder status must not have been updated yet
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	targetSvc := &corev1.Service{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: targetRef.Namespace, Name: targetRef.Name}, targetSvc)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Have Container JFR connect to the target JVM
	clusterIP, err := getClusterIP(targetSvc)
	if err != nil {
		return reconcile.Result{}, err
	}
	jmxPort := instance.Spec.Port
	err = r.jfrClient.Connect(*clusterIP, jmxPort)
	if err != nil {
		log.Error(err, "failed to connect to target JVM")
		r.closeClient()
		return reconcile.Result{}, err
	}
	defer r.disconnectClient()

	// Check for any new recording requests in this FlightRecorder's spec
	// and instruct Container JFR to create corresponding recordings
	log.Info("Syncing recording requests for service", "service", targetSvc.Name, "namespace", targetSvc.Namespace,
		"host", *clusterIP, "port", jmxPort)
	for _, request := range instance.Spec.Requests {
		log.Info("Creating new recording", "name", request.Name, "duration", request.Duration, "eventOptions", request.EventOptions)
		err := r.jfrClient.DumpRecording(request.Name, int(request.Duration.Seconds()), request.EventOptions)
		if err != nil {
			log.Error(err, "failed to create new recording")
			r.closeClient() // TODO maybe track an error state in the client instead of relying on calling this everywhere
			return reconcile.Result{}, err
		}
	}

	// Get an updated list of in-memory flight recordings
	log.Info("Listing recordings for service", "service", targetSvc.Name, "namespace", targetSvc.Namespace,
		"host", *clusterIP, "port", jmxPort)
	descriptors, err := r.jfrClient.ListRecordings()
	if err != nil {
		log.Error(err, "failed to list flight recordings")
		r.closeClient()
		return reconcile.Result{}, err
	}

	reqLogger.Info("Updating FlightRecorder", "Namespace", instance.Namespace, "Name", instance.Name)
	// Remove any recording requests from the spec that are now showing in Container JFR's list
	newRequests := []rhjmcv1alpha1.RecordingRequest{}
	for _, req := range instance.Spec.Requests {
		for _, desc := range descriptors {
			if req.Name != desc.Name {
				newRequests = append(newRequests, req)
				break
			}
		}
	}
	instance.Spec.Requests = newRequests
	err = r.client.Update(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update recording info in Status with info received from Container JFR
	recordings := createRecordingInfo(descriptors)

	// TODO Download URLs returned by Container JFR's 'list' command currently
	// work when it is connected to the target JVM. To work around this,
	// we archive the recording to persistent storage and update the download
	// URL to point to that saved file.
	toUpdate := map[string]*rhjmcv1alpha1.RecordingInfo{}
	for idx, newInfo := range recordings {
		oldInfo := findRecordingByName(instance.Status.Recordings, newInfo.Name)
		// Recording completed since last observation
		if !newInfo.Active && (oldInfo == nil || oldInfo.Active) {
			filename, err := r.jfrClient.SaveRecording(newInfo.Name)
			if err != nil {
				log.Error(err, "failed to save recording", "name", newInfo.Name)
				return reconcile.Result{}, err
			}
			toUpdate[*filename] = &recordings[idx]
		} else if oldInfo != nil && len(oldInfo.DownloadURL) > 0 {
			// Use previously obtained download URL
			recordings[idx].DownloadURL = oldInfo.DownloadURL
		}
	}

	if len(toUpdate) > 0 {
		savedRecordings, err := r.jfrClient.ListSavedRecordings()
		if err != nil {
			return reconcile.Result{}, err
		}
		// Update download URLs using list of saved recordings
		for _, saved := range savedRecordings {
			if info, pres := toUpdate[saved.Name]; pres {
				log.Info("updating download URL", "name", info.Name, "url", saved.DownloadURL)
				info.DownloadURL = saved.DownloadURL
			}
		}
	}

	instance.Status.Recordings = recordings
	err = r.client.Status().Update(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Requeue if any recordings are still in progress
	result := reconcile.Result{}
	for _, recording := range recordings {
		if recording.Active {
			// Check progress of recordings after 10 seconds
			result = reconcile.Result{RequeueAfter: 10 * time.Second}
			break
		}
	}

	reqLogger.Info("FlightRecorder successfully updated", "Namespace", instance.Namespace, "Name", instance.Name)
	return result, nil
}

func findRecordingByName(recordings []rhjmcv1alpha1.RecordingInfo, name string) *rhjmcv1alpha1.RecordingInfo {
	for idx, recording := range recordings {
		if recording.Name == name {
			return &recordings[idx]
		}
	}
	return nil
}

func (r *ReconcileFlightRecorder) connectToContainerJFR(ctx context.Context, namespace string) (*jfrclient.ContainerJfrClient, error) {
	// Query the "clienturl" endpoint of Container JFR for the command URL
	clientURL, err := r.getClientURL(ctx, namespace)
	if err != nil {
		return nil, err
	}
	tok, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, err
	}
	strTok := string(tok)
	config := &jfrclient.Config{ServerURL: clientURL, AccessToken: &strTok, TLSVerify: !strings.EqualFold(os.Getenv("TLS_VERIFY"), "false")}
	jfrClient, err := jfrclient.Create(config)
	if err != nil {
		return nil, err
	}
	return jfrClient, nil
}

func getClusterIP(svc *corev1.Service) (*string, error) {
	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == corev1.ClusterIPNone {
		return nil, fmt.Errorf("ClusterIP unavailable for %s", svc.Name)
	}
	return &clusterIP, nil
}

func createRecordingInfo(descriptors []jfrclient.RecordingDescriptor) []rhjmcv1alpha1.RecordingInfo {
	infos := make([]rhjmcv1alpha1.RecordingInfo, len(descriptors))
	for i, descriptor := range descriptors {
		// Consider any recording not stopped to be "active"
		active := descriptor.State != jfrclient.RecordingStateStopped
		startTime := metav1.Unix(0, descriptor.StartTime*int64(time.Millisecond))
		duration := metav1.Duration{
			Duration: time.Duration(descriptor.Duration) * time.Millisecond,
		}
		info := rhjmcv1alpha1.RecordingInfo{
			Name:      descriptor.Name,
			Active:    active,
			StartTime: startTime,
			Duration:  duration,
		}
		infos[i] = info
	}
	return infos
}

func (r *ReconcileFlightRecorder) getClientURL(ctx context.Context, namespace string) (*url.URL, error) {
	cjfrSvc := &corev1.Service{}
	// TODO Get service namespace/name from ContainerJFR CR
	cjfrSvcName := "containerjfr"
	err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cjfrSvcName}, cjfrSvc)
	if err != nil {
		return nil, err
	}
	clusterIP, err := getClusterIP(cjfrSvc)
	if err != nil {
		return nil, err
	}
	webServerPort, err := getWebServerPort(cjfrSvc)
	if err != nil {
		return nil, err
	}
	host := fmt.Sprintf("http://%s:%d/clienturl", *clusterIP, webServerPort)
	resp, err := http.Get(host)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Find "clientUrl" JSON property in repsonse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	clientURLHolder := struct {
		ClientURL string `json:"clientUrl"`
	}{}
	err = json.Unmarshal(body, &clientURLHolder)
	if err != nil {
		return nil, err
	}
	return url.Parse(clientURLHolder.ClientURL)
}

func getWebServerPort(svc *corev1.Service) (int32, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == "export" {
			return port.Port, nil
		}
	}
	return 0, errors.New("ContainerJFR service had no port named \"export\"")
}

func (r *ReconcileFlightRecorder) disconnectClient() {
	if r.jfrClient != nil {
		err := r.jfrClient.Disconnect()
		if err != nil {
			log.Error(err, "failed to disconnect from target JVM")
			r.closeClient()
		}
	}
}

func (r *ReconcileFlightRecorder) closeClient() {
	r.jfrClient.Close()
	r.jfrClient = nil
}
