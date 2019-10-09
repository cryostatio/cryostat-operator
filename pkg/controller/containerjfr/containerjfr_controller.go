package containerjfr

import (
	"context"
	"fmt"

	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_containerjfr")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ContainerJFR Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileContainerJFR{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("containerjfr-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ContainerJFR
	err = c.Watch(&source.Kind{Type: &rhjmcv1alpha1.ContainerJFR{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner ContainerJFR
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &rhjmcv1alpha1.ContainerJFR{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileContainerJFR implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileContainerJFR{}

// ReconcileContainerJFR reconciles a ContainerJFR object
type ReconcileContainerJFR struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ContainerJFR object and makes changes based on the state read
// and what is in the ContainerJFR.Spec
func (r *ReconcileContainerJFR) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ContainerJFR")

	// Fetch the ContainerJFR instance
	instance := &rhjmcv1alpha1.ContainerJFR{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("ContainerJFR instance not found")
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Error reading ContainerJFR instance")
		return reconcile.Result{}, err
	}

	pvc := newPersistentVolumeClaimForCR(instance)
	if err = r.createObjectIfNotExists(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, &corev1.PersistentVolumeClaim{}, pvc); err != nil {
		return reconcile.Result{}, err
	}

	pod := newPodForCR(instance)
	if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.createObjectIfNotExists(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, &corev1.Pod{}, pod); err != nil {
		reqLogger.Error(err, "Could not create pod")
		return reconcile.Result{}, err
	}

	grafana := newGrafanaServiceForPod(instance)
	if err = r.createObjectIfNotExists(context.TODO(), types.NamespacedName{Name: grafana.Name, Namespace: grafana.Namespace}, &corev1.Service{}, grafana); err != nil {
		return reconcile.Result{}, err
	}

	datasource := newJfrDatasourceServiceForPod(instance)
	if err = r.createObjectIfNotExists(context.TODO(), types.NamespacedName{Name: datasource.Name, Namespace: datasource.Namespace}, &corev1.Service{}, datasource); err != nil {
		return reconcile.Result{}, err
	}

	exporter := newExporterServiceForPod(instance)
	if err = r.createObjectIfNotExists(context.TODO(), types.NamespacedName{Name: exporter.Name, Namespace: exporter.Namespace}, &corev1.Service{}, exporter); err != nil {
		return reconcile.Result{}, err
	}

	cmdChan := newCommandChannelServiceForPod(instance)
	if err = r.createObjectIfNotExists(context.TODO(), types.NamespacedName{Name: cmdChan.Name, Namespace: cmdChan.Namespace}, &corev1.Service{}, cmdChan); err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
	return reconcile.Result{}, nil
}

func newPersistentVolumeClaimForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.PersistentVolumeClaim {
	storageClassName := ""
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": *resource.NewQuantity(500*1024*1024, resource.BinarySI),
				},
			},
		},
	}
}

func newPodForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "container-jfr-operator",
			Volumes: []corev1.Volume{
				{
					Name: cr.Name,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: cr.Name,
						},
					},
				},
			},
			Containers: []corev1.Container{
				newCoreContainer(cr),
				newGrafanaContainer(cr),
				newJfrDatasourceContainer(cr),
			},
		},
	}
}

func newCoreContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name,
		Image: "quay.io/rh-jmc-team/container-jfr:0.4.4-debug",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cr.Name,
				MountPath: "flightrecordings",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8181,
			},
			{
				ContainerPort: 9090,
			},
			{
				ContainerPort: 9091,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "CONTAINER_JFR_WEB_PORT",
				Value: "8181",
			},
			{
				Name:  "CONTAINER_JFR_EXT_WEB_PORT",
				Value: "80",
			},
			{
				Name: "CONTAINER_JFR_WEB_HOST",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr",
						},
						Key: "CONTAINER_JFR_WEB_HOST",
					},
				},
			},
			{
				Name:  "CONTAINER_JFR_LISTEN_PORT",
				Value: "9090",
			},
			{
				Name:  "CONTAINER_JFR_EXT_LISTEN_PORT",
				Value: "80",
			},
			{
				Name: "CONTAINER_JFR_LISTEN_HOST",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr-command",
						},
						Key: "CONTAINER_JFR_LISTEN_HOST",
					},
				},
			},
			{
				Name: "GRAFANA_DASHBOARD_URL",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr-grafana",
						},
						Key: "GRAFANA_DASHBOARD_URL",
					},
				},
			},
			{
				Name: "GRAFANA_DATASOURCE_URL",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr-jfr-datasource",
						},
						Key: "GRAFANA_DATASOURCE_URL",
					},
				},
			},
		},
	}
}

func newGrafanaContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-grafana",
		Image: "docker.io/grafana/grafana:6.2.2",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3000,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "GF_INSTALL_PLUGINS",
				Value: "grafana-simple-json-datasource",
			},
		},
	}
}

func newJfrDatasourceContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-jfr-datasource",
		Image: "quay.io/rh-jmc-team/jfr-datasource",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8080,
			},
		},
		Env: []corev1.EnvVar{},
	}
}

func newExporterServiceForPod(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
			},
			Annotations: map[string]string{
				"fabric8.io/expose": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "8181-tcp",
					Port:       8181,
					TargetPort: intstr.IntOrString{IntVal: 8181},
				},
				{
					Name:       "9091-tcp",
					Port:       9091,
					TargetPort: intstr.IntOrString{IntVal: 9091},
				},
			},
		},
	}
}

func newCommandChannelServiceForPod(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-command",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
			},
			Annotations: map[string]string{
				"fabric8.io/expose": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "9090-tcp",
					Port:       9090,
					TargetPort: intstr.IntOrString{IntVal: 9090},
				},
			},
		},
	}
}

func newGrafanaServiceForPod(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "grafana",
			},
			Annotations: map[string]string{
				"fabric8.io/expose": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "3000-tcp",
					Port:       3000,
					TargetPort: intstr.IntOrString{IntVal: 3000},
				},
			},
		},
	}
}

func newJfrDatasourceServiceForPod(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-jfr-datasource",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "grafana",
			},
			Annotations: map[string]string{
				"fabric8.io/expose": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "8080-tcp",
					Port:       8080,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				},
			},
		},
	}
}

func (r *ReconcileContainerJFR) createObjectIfNotExists(ctx context.Context, ns types.NamespacedName, found runtime.Object, toCreate runtime.Object) error {
	logger := log.WithValues("Request.Namespace", ns.Namespace, "Name", ns.Name, "Kind", fmt.Sprintf("%T", toCreate))
	err := r.client.Get(ctx, ns, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found")
		if err := r.client.Create(ctx, toCreate); err != nil {
			logger.Error(err, "Could not be created")
			return err
		} else {
			logger.Info("Created")
		}
	} else if err != nil {
		logger.Error(err, "Could not be read")
		return err
	}
	logger.Info("Already exists")
	return nil
}
