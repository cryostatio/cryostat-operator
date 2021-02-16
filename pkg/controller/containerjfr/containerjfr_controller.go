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

package containerjfr

import (
	"context"
	"fmt"
	"time"

	goerrors "errors"

	"github.com/google/go-cmp/cmp"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/route/v1"
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	resources "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/containerjfr/resource_definitions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_containerjfr")

// Add creates a new ContainerJFR Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileContainerJFR{Scheme: mgr.GetScheme(), Client: mgr.GetClient(),
		ReconcilerTLS: common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
			Client: mgr.GetClient(),
		}),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("containerjfr-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ContainerJFR
	err = c.Watch(&source.Kind{Type: &rhjmcv1beta1.ContainerJFR{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resources and requeue the owner ContainerJFR
	resources := []runtime.Object{&appsv1.Deployment{}, &corev1.Service{}, &corev1.Secret{}, &openshiftv1.Route{}, &corev1.PersistentVolumeClaim{}}

	for _, resource := range resources {
		err = c.Watch(&source.Kind{Type: resource}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &rhjmcv1beta1.ContainerJFR{},
		})
		if err != nil {
			return err
		}
	}
	// TODO watch certificates and redeploy when renewed

	return nil
}

// blank assignment to verify that ReconcileContainerJFR implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileContainerJFR{}

// ReconcileContainerJFR reconciles a ContainerJFR object
type ReconcileContainerJFR struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client client.Client
	Scheme *runtime.Scheme
	common.ReconcilerTLS
}

// Name used for Finalizer that handles ContainerJFR deletion
const cjfrFinalizer = "containerjfr.finalizer.rhjmc.redhat.com"

// Reconcile reads that state of the cluster for a ContainerJFR object and makes changes based on the state read
// and what is in the ContainerJFR.Spec
func (r *ReconcileContainerJFR) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ContainerJFR")

	// Fetch the ContainerJFR instance
	instance := &rhjmcv1beta1.ContainerJFR{}
	err := r.Client.Get(context.Background(), request.NamespacedName, instance)
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

	// OpenShift-specific
	// Check if this Recording is being deleted
	if instance.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(instance, cjfrFinalizer) {
			return r.deleteConsoleLinks(context.Background(), instance)
		}
		// Ready for deletion
		return reconcile.Result{}, nil
	}

	// Add our finalizer, so we can clean up Container JFR resources upon deletion
	if !controllerutil.ContainsFinalizer(instance, cjfrFinalizer) {
		err := common.AddFinalizer(context.Background(), r.Client, instance, cjfrFinalizer)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info("Spec", "Minimal", instance.Spec.Minimal)

	pvc := resources.NewPersistentVolumeClaimForCR(instance)
	if err := controllerutil.SetControllerReference(instance, pvc, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, &corev1.PersistentVolumeClaim{}, pvc); err != nil {
		return reconcile.Result{}, err
	}

	grafanaSecret := resources.NewGrafanaSecretForCR(instance)
	if err := controllerutil.SetControllerReference(instance, grafanaSecret, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: grafanaSecret.Name, Namespace: grafanaSecret.Namespace}, &corev1.Secret{}, grafanaSecret); err != nil {
		return reconcile.Result{}, err
	}

	jmxAuthSecret := resources.NewJmxSecretForCR(instance)
	if err := controllerutil.SetControllerReference(instance, jmxAuthSecret, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: jmxAuthSecret.Name, Namespace: jmxAuthSecret.Namespace}, &corev1.Secret{}, jmxAuthSecret); err != nil {
		return reconcile.Result{}, err
	}

	// Set up TLS using cert-manager, if available
	protocol := "http"
	var tlsConfig *resources.TLSConfig
	var routeTLS *openshiftv1.TLSConfig
	if r.IsCertManagerEnabled() {
		tlsConfig, err = r.setupTLS(context.Background(), instance)
		if err != nil {
			if err == common.ErrCertNotReady {
				return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
			}
			reqLogger.Error(err, "Failed to set up TLS for Container JFR")
			return reconcile.Result{}, err
		}

		// Get CA certificate from secret and set as destination CA in route
		caCert, err := r.GetContainerJFRCABytes(context.Background(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
		routeTLS = &openshiftv1.TLSConfig{
			Termination:              openshiftv1.TLSTerminationReencrypt,
			DestinationCACertificate: string(caCert),
		}
		protocol = "https"
	}

	serviceSpecs := &resources.ServiceSpecs{}
	if !instance.Spec.Minimal {
		grafanaSvc := resources.NewGrafanaService(instance)
		url, err := r.createService(context.Background(), instance, grafanaSvc, &grafanaSvc.Spec.Ports[0], routeTLS)
		if err != nil {
			return reconcile.Result{}, err
		}
		serviceSpecs.GrafanaURL = fmt.Sprintf("%s://%s", protocol, url)

		// check for existing minimal deployment and delete if found
		deployment := resources.NewDeploymentForCR(instance, serviceSpecs, nil)
		err = r.Client.Get(context.Background(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)
		if err == nil && len(deployment.Spec.Template.Spec.Containers) == 1 {
			reqLogger.Info("Deleting existing minimal deployment")
			err = r.Client.Delete(context.Background(), deployment)
			if err != nil && !errors.IsNotFound(err) {
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, err
			}
		}
	} else {
		// check for existing non-minimal resources and delete if found
		svc := resources.NewGrafanaService(instance)
		reqLogger.Info("Deleting existing non-minimal route", "route.Name", svc.Name)
		route := &openshiftv1.Route{}
		err = r.Client.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, route)
		if err != nil && !errors.IsNotFound(err) {
			reqLogger.Info("Non-minimal route could not be retrieved", "route.Name", svc.Name)
			return reconcile.Result{}, err
		} else if err == nil {
			err = r.Client.Delete(context.Background(), route)
			if err != nil && !errors.IsNotFound(err) {
				reqLogger.Info("Could not delete non-minimal route", "route.Name", svc.Name)
				return reconcile.Result{}, err
			}
		}

		err = r.Client.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svc)
		if err == nil {
			reqLogger.Info("Deleting existing non-minimal service", "svc.Name", svc.Name)
			err = r.Client.Delete(context.Background(), svc)
			if err != nil && !errors.IsNotFound(err) {
				reqLogger.Info("Could not delete non-minimal service")
				return reconcile.Result{}, err
			}
		}

		deployment := resources.NewDeploymentForCR(instance, serviceSpecs, nil)
		err = r.Client.Get(context.Background(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)
		if err == nil && len(deployment.Spec.Template.Spec.Containers) > 1 {
			reqLogger.Info("Deleting existing non-minimal deployment")
			err = r.Client.Delete(context.Background(), deployment)
			if err != nil && !errors.IsNotFound(err) {
				reqLogger.Info("Could not delete non-minimal deployment")
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, err
			}
		}
	}

	exporterSvc := resources.NewExporterService(instance)
	url, err := r.createService(context.Background(), instance, exporterSvc, &exporterSvc.Spec.Ports[0], routeTLS)
	if err != nil {
		return reconcile.Result{}, err
	}
	serviceSpecs.CoreHostname = url

	cmdChanSvc := resources.NewCommandChannelService(instance)
	url, err = r.createService(context.Background(), instance, cmdChanSvc, &cmdChanSvc.Spec.Ports[0], routeTLS)
	if err != nil {
		return reconcile.Result{}, err
	}
	serviceSpecs.CommandHostname = url

	deployment := resources.NewDeploymentForCR(instance, serviceSpecs, tlsConfig)
	if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err = r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, &appsv1.Deployment{}, deployment); err != nil {
		reqLogger.Error(err, "Could not create deployment")
		return reconcile.Result{}, err
	}

	// Check that secrets mounted in /truststore coincide with CRD
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)
	if err == nil {
		deploymentMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
		expectedDeploymentSpec := resources.NewDeploymentForCR(instance, serviceSpecs, tlsConfig).Spec.Template.Spec
		if !cmp.Equal(deploymentMounts, expectedDeploymentSpec.Containers[0].VolumeMounts) {
			reqLogger.Info("cert secrets mounted do not coincide with those specified in CRD, modifying deployment")
			// Modify deployment
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts = expectedDeploymentSpec.Containers[0].VolumeMounts
			deployment.Spec.Template.Spec.Volumes = expectedDeploymentSpec.Volumes
			err = r.Client.Update(context.Background(), deployment)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	// OpenShift-specific
	links, err := r.getConsoleLinks(instance)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(links) == 0 {
		link := resources.NewConsoleLink(instance, "https://"+serviceSpecs.CoreHostname)
		if err = r.Client.Create(context.Background(), link); err != nil {
			reqLogger.Error(err, "Could not create ConsoleLink")
			return reconcile.Result{}, err
		}
		reqLogger.Info("Created ConsoleLink", "linkName", link.Name)
	}

	reqLogger.Info("Skip reconcile: Deployment already exists", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileContainerJFR) createService(ctx context.Context, controller *rhjmcv1beta1.ContainerJFR, svc *corev1.Service, exposePort *corev1.ServicePort,
	tlsConfig *openshiftv1.TLSConfig) (string, error) {
	if err := controllerutil.SetControllerReference(controller, svc, r.Scheme); err != nil {
		return "", err
	}
	if err := r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &corev1.Service{}, svc); err != nil {
		return "", err
	}

	// Use edge termination by default
	if tlsConfig == nil {
		tlsConfig = &openshiftv1.TLSConfig{
			Termination:                   openshiftv1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: openshiftv1.InsecureEdgeTerminationPolicyRedirect,
		}
	}
	if exposePort != nil {
		return r.createRouteForService(controller, svc, *exposePort, tlsConfig)
	}

	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svc); err != nil {
		return "", err
	}
	if svc.Spec.ClusterIP == "" {
		return "", errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected service %s to have ClusterIP, but got empty string", svc.Name)))
	}
	if len(svc.Spec.Ports) != 1 {
		return "", errors.NewInternalError(goerrors.New(fmt.Sprintf("Expected service %s to have one Port, but got %d", svc.Name, len(svc.Spec.Ports))))
	}
	return fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port), nil
}

func (r *ReconcileContainerJFR) createRouteForService(controller *rhjmcv1beta1.ContainerJFR, svc *corev1.Service, exposePort corev1.ServicePort,
	tlsConfig *openshiftv1.TLSConfig) (string, error) {
	logger := log.WithValues("Request.Namespace", svc.Namespace, "Name", svc.Name, "Kind", fmt.Sprintf("%T", &openshiftv1.Route{}))
	route := &openshiftv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
		Spec: openshiftv1.RouteSpec{
			To: openshiftv1.RouteTargetReference{
				Kind: "Service",
				Name: svc.Name,
			},
			Port: &openshiftv1.RoutePort{TargetPort: exposePort.TargetPort},
			TLS:  tlsConfig,
		},
	}
	if err := controllerutil.SetControllerReference(controller, route, r.Scheme); err != nil {
		return "", err
	}

	found := &openshiftv1.Route{}
	err := r.Client.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found")
		if err := r.Client.Create(context.Background(), route); err != nil {
			logger.Error(err, "Could not be created")
			return "", err
		}
		logger.Info("Created")
		found = route
	} else if err != nil {
		logger.Error(err, "Could not be read")
		return "", err
	}

	logger.Info("Route created", "Service.Status", fmt.Sprintf("%#v", found.Status))
	if len(found.Status.Ingress) < 1 {
		return "", errors.NewTooManyRequestsError("Ingress configuration not yet available")
	}

	return found.Status.Ingress[0].Host, nil
}

func (r *ReconcileContainerJFR) createObjectIfNotExists(ctx context.Context, ns types.NamespacedName, found runtime.Object, toCreate runtime.Object) error {
	logger := log.WithValues("Request.Namespace", ns.Namespace, "Name", ns.Name, "Kind", fmt.Sprintf("%T", toCreate))
	err := r.Client.Get(ctx, ns, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found")
		if err := r.Client.Create(ctx, toCreate); err != nil {
			logger.Error(err, "Could not be created")
			return err
		} else {
			logger.Info("Created")
			found = toCreate
		}
	} else if err != nil {
		logger.Error(err, "Could not be read")
		return err
	}
	logger.Info("Already exists")
	return nil
}

func (r *ReconcileContainerJFR) getConsoleLinks(cr *rhjmcv1beta1.ContainerJFR) ([]consolev1.ConsoleLink, error) {
	links := &consolev1.ConsoleLinkList{}
	linkLabels := labels.Set{
		resources.ConsoleLinkNSLabel:   cr.Namespace,
		resources.ConsoleLinkNameLabel: cr.Name,
	}
	err := r.Client.List(context.Background(), links, &client.ListOptions{
		LabelSelector: linkLabels.AsSelectorPreValidated(),
	})
	if err != nil {
		return nil, err
	}
	return links.Items, nil
}

func (r *ReconcileContainerJFR) deleteConsoleLinks(ctx context.Context, cr *rhjmcv1beta1.ContainerJFR) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)
	links, err := r.getConsoleLinks(cr)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Should just be one, but use loop just in case
	for _, link := range links {
		err := r.Client.Delete(ctx, &link)
		if err != nil {
			reqLogger.Error(err, "failed to delete ConsoleLink", "linkName", link.Name)
			return reconcile.Result{}, err
		}
		reqLogger.Info("deleted ConsoleLink", "linkName", link.Name)
	}

	// Remove finalizer upon success
	err = common.RemoveFinalizer(ctx, r.Client, cr, cjfrFinalizer)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
