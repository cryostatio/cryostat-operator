// Copyright The Cryostat Authors
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
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"

	goerrors "errors"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/route/v1"
	securityv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// CryostatReconciler reconciles a Cryostat object
type CryostatReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	IsOpenShift   bool
	EventRecorder record.EventRecorder
	RESTMapper    meta.RESTMapper
	common.ReconcilerTLS
}

// Name used for Finalizer that handles Cryostat deletion
const cryostatFinalizer = "operator.cryostat.io/cryostat.finalizer"

// Environment variable to override the core application image
const coreImageTagEnv = "RELATED_IMAGE_CORE"

// Environment variable to override the JFR datasource image
const datasourceImageTagEnv = "RELATED_IMAGE_DATASOURCE"

// Environment variable to override the Grafana dashboard image
const grafanaImageTagEnv = "RELATED_IMAGE_GRAFANA"

// Environment variable to override the cryostat-reports image
const reportsImageTagEnv = "RELATED_IMAGE_REPORTS"

// Regular expression for the start of a GID range in the OpenShift
// supplemental groups SCC annotation
var supGroupRegexp = regexp.MustCompile(`^\d+`)

// The canonical name of an APIServer instance
const apiServerName = "cluster"

// Reasons for Cryostat Conditions
const (
	reasonWaitingForCert         = "WaitingForCertificate"
	reasonAllCertsReady          = "AllCertificatesReady"
	reasonCertManagerUnavailable = "CertManagerUnavailable"
	reasonCertManagerDisabled    = "CertManagerDisabled"
)

// Map Cryostat conditions to deployment conditions
type deploymentConditionTypeMap map[operatorv1beta1.CryostatConditionType]appsv1.DeploymentConditionType

var mainDeploymentConditions = deploymentConditionTypeMap{
	operatorv1beta1.ConditionTypeMainDeploymentAvailable:      appsv1.DeploymentAvailable,
	operatorv1beta1.ConditionTypeMainDeploymentProgressing:    appsv1.DeploymentProgressing,
	operatorv1beta1.ConditionTypeMainDeploymentReplicaFailure: appsv1.DeploymentReplicaFailure,
}
var reportsDeploymentConditions = deploymentConditionTypeMap{
	operatorv1beta1.ConditionTypeReportsDeploymentAvailable:      appsv1.DeploymentAvailable,
	operatorv1beta1.ConditionTypeReportsDeploymentProgressing:    appsv1.DeploymentProgressing,
	operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure: appsv1.DeploymentReplicaFailure,
}

// +kubebuilder:rbac:namespace=system,groups="",resources=pods;services;services/finalizers;endpoints;persistentvolumeclaims;events;configmaps;secrets;serviceaccounts,verbs=*
// +kubebuilder:rbac:namespace=system,groups="",resources=replicationcontrollers,verbs=get
// +kubebuilder:rbac:namespace=system,groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=create;get;list;update;watch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=selfsubjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=oauth.openshift.io,resources=oauthaccesstokens,verbs=list;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;update;watch
// +kubebuilder:rbac:namespace=system,groups=route.openshift.io,resources=routes;routes/custom-host,verbs=*
// +kubebuilder:rbac:namespace=system,groups=apps.openshift.io,resources=deploymentconfigs,verbs=get
// +kubebuilder:rbac:namespace=system,groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=*
// +kubebuilder:rbac:namespace=system,groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;create
// +kubebuilder:rbac:namespace=system,groups=cert-manager.io,resources=issuers;certificates,verbs=create;get;list;update;watch
// +kubebuilder:rbac:namespace=system,groups=operator.cryostat.io,resources=cryostats,verbs=*
// +kubebuilder:rbac:namespace=system,groups=operator.cryostat.io,resources=cryostats/status,verbs=get;update;patch
// +kubebuilder:rbac:namespace=system,groups=operator.cryostat.io,resources=cryostats/finalizers,verbs=update
// +kubebuilder:rbac:groups=console.openshift.io,resources=consolelinks,verbs=get;create;list;update;delete
// +kubebuilder:rbac:namespace=system,groups=networking.k8s.io,resources=ingresses,verbs=*

// Reconcile processes a Cryostat CR and manages a Cryostat installation accordingly
func (r *CryostatReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reqLogger.Info("Reconciling Cryostat")

	// Fetch the Cryostat instance
	instance := &operatorv1beta1.Cryostat{}
	err := r.Client.Get(context.Background(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Cryostat instance not found")
			return reconcile.Result{}, nil
		}
		reqLogger.Error(err, "Error reading Cryostat instance")
		return reconcile.Result{}, err
	}

	// Check if this Cryostat is being deleted
	if instance.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(instance, cryostatFinalizer) {
			err = r.deleteClusterRoleBinding(ctx, instance)
			if err != nil {
				return reconcile.Result{}, err
			}

			// OpenShift-specific
			if r.IsOpenShift {
				err = r.deleteConsoleLink(ctx, instance)
				if err != nil {
					return reconcile.Result{}, err
				}

				err = r.deleteCorsAllowedOrigins(ctx, instance.Status.ApplicationURL, instance)
				if err != nil {
					return reconcile.Result{}, err
				}
			}

			err = common.RemoveFinalizer(ctx, r.Client, instance, cryostatFinalizer)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		// Ready for deletion
		return reconcile.Result{}, nil
	}

	// Add our finalizer, so we can clean up Cryostat resources upon deletion
	if !controllerutil.ContainsFinalizer(instance, cryostatFinalizer) {
		err := common.AddFinalizer(context.Background(), r.Client, instance, cryostatFinalizer)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info("Spec", "Minimal", instance.Spec.Minimal)

	shouldCreatePvc := !(instance.Spec.StorageOptions != nil && instance.Spec.StorageOptions.EmptyDir != nil && instance.Spec.StorageOptions.EmptyDir.Enabled)
	if shouldCreatePvc {
		pvc := resources.NewPersistentVolumeClaimForCR(instance)
		if err := controllerutil.SetControllerReference(instance, pvc, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
		if err = r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, &corev1.PersistentVolumeClaim{}, pvc); err != nil {
			return reconcile.Result{}, err
		}
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
	var tlsConfig *resources.TLSConfig
	var routeTLS *openshiftv1.TLSConfig
	if r.IsCertManagerEnabled(instance) {
		tlsConfig, err = r.setupTLS(context.Background(), instance)
		if err != nil {
			if err == common.ErrCertNotReady {
				condErr := r.updateCondition(ctx, instance, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
					reasonWaitingForCert, "Waiting for certificates to become ready.")
				if condErr != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
			}
			if err == errCertManagerMissing {
				r.updateCondition(ctx, instance, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
					reasonCertManagerUnavailable, eventCertManagerUnavailableMsg)
			}
			reqLogger.Error(err, "Failed to set up TLS for Cryostat")
			return reconcile.Result{}, err
		}

		// Get CA certificate from secret and set as destination CA in route
		caCert, err := r.GetCryostatCABytes(context.Background(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
		routeTLS = &openshiftv1.TLSConfig{
			Termination:              openshiftv1.TLSTerminationReencrypt,
			DestinationCACertificate: string(caCert),
		}

		err = r.updateCondition(ctx, instance, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
			reasonAllCertsReady, "All certificates for Cryostat components are ready.")
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		err = r.updateCondition(ctx, instance, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
			reasonCertManagerDisabled, "TLS setup has been disabled.")
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Create RBAC resources for Cryostat
	err = r.createRBAC(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	serviceSpecs := &resources.ServiceSpecs{}
	if !instance.Spec.Minimal {
		grafanaSvc := resources.NewGrafanaService(instance)
		svcUrl, err := r.createService(context.Background(), instance, grafanaSvc, &grafanaSvc.Spec.Ports[0], routeTLS)
		if err != nil {
			return requeueIfIngressNotReady(reqLogger, err)
		}
		serviceSpecs.GrafanaURL = svcUrl

		// check for existing minimal deployment and delete if found
		deployment := &appsv1.Deployment{}
		err = r.Client.Get(context.Background(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
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
		if r.IsOpenShift {
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
		} else {
			reqLogger.Info("Deleting existing non-minimal ingress", "ingress.Name", svc.Name)
			ingress := &netv1.Ingress{}
			err = r.Client.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, ingress)
			if err != nil && !errors.IsNotFound(err) {
				reqLogger.Info("Non-minimal ingress could not be retrieved", "ingress.Name", svc.Name)
				return reconcile.Result{}, err
			} else if err == nil {
				err = r.Client.Delete(context.Background(), ingress)
				if err != nil && !errors.IsNotFound(err) {
					reqLogger.Info("Could not delete non-minimal ingress", "ingress.Name", svc.Name)
					return reconcile.Result{}, err
				}
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

		deployment := &appsv1.Deployment{}
		err = r.Client.Get(context.Background(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
		if err == nil && len(deployment.Spec.Template.Spec.Containers) > 1 {
			reqLogger.Info("Deleting existing non-minimal deployment")
			err = r.Client.Delete(context.Background(), deployment)
			if err != nil && !errors.IsNotFound(err) {
				reqLogger.Info("Could not delete non-minimal deployment")
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, err
			}
		}
	}

	coreSvc := resources.NewCoreService(instance)
	svcUrl, err := r.createService(context.Background(), instance, coreSvc, &coreSvc.Spec.Ports[0], routeTLS)
	if err != nil {
		return requeueIfIngressNotReady(reqLogger, err)
	}
	serviceSpecs.CoreURL = svcUrl

	imageTags := r.getImageTags()
	fsGroup, err := r.getFSGroup(ctx, instance.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	reportsResult, err := r.reconcileReports(ctx, reqLogger, instance, tlsConfig, imageTags, serviceSpecs)
	if err != nil {
		return reportsResult, err
	}

	deployment := resources.NewDeploymentForCR(instance, serviceSpecs, imageTags, tlsConfig, *fsGroup, r.IsOpenShift)
	podTemplate := deployment.Spec.Template.DeepCopy()
	if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Update pod template spec to propagate any changes from Cryostat CR
		deployment.Spec.Template.Spec = podTemplate.Spec
		return nil
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info(fmt.Sprintf("Deployment %s", op))

	if serviceSpecs.CoreURL != nil {
		instance.Status.ApplicationURL = serviceSpecs.CoreURL.String()
		err = r.Client.Status().Update(context.Background(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// OpenShift-specific
	if r.IsOpenShift {
		err := r.createConsoleLink(ctx, instance, serviceSpecs.CoreURL.String())
		if err != nil {
			return reconcile.Result{}, err
		}

		if instance.Status.ApplicationURL != "" {
			err = r.addCorsAllowedOriginIfNotPresent(ctx, instance.Status.ApplicationURL, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// Check deployment status and update conditions
	err = r.updateConditionsFromDeployment(ctx, instance, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
		mainDeploymentConditions)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Successfully reconciled deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CryostatReconciler) SetupWithManager(mgr ctrl.Manager) error {
	c := ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1beta1.Cryostat{})

	// Watch for changes to secondary resources and requeue the owner Cryostat
	resources := []client.Object{&appsv1.Deployment{}, &corev1.Service{}, &corev1.Secret{}, &corev1.PersistentVolumeClaim{}}
	if r.IsOpenShift {
		resources = append(resources, &openshiftv1.Route{})
	}
	// TODO watch certificates and redeploy when renewed

	for _, resource := range resources {
		c = c.Watches(&source.Kind{Type: resource}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &operatorv1beta1.Cryostat{},
		})
	}

	return c.Complete(r)
}

func (r *CryostatReconciler) reconcileReports(ctx context.Context, reqLogger logr.Logger, instance *operatorv1beta1.Cryostat,
	tls *resources.TLSConfig, imageTags *resources.ImageTags, serviceSpecs *resources.ServiceSpecs) (reconcile.Result, error) {
	reqLogger.Info("Spec", "Reports", instance.Spec.ReportOptions)

	if instance.Spec.ReportOptions == nil {
		instance.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{Replicas: 0}
	}
	desired := instance.Spec.ReportOptions.Replicas

	deployment := resources.NewDeploymentForReports(instance, imageTags, tls)
	if desired == 0 {
		svc := resources.NewReportService(instance)
		if err := r.Client.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		if err := r.Client.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}

		removeConditionIfPresent(instance, operatorv1beta1.ConditionTypeReportsDeploymentAvailable,
			operatorv1beta1.ConditionTypeReportsDeploymentProgressing,
			operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure)
		err := r.Client.Status().Update(ctx, instance)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if desired > 0 {
		svc := resources.NewReportService(instance)
		if err := controllerutil.SetControllerReference(instance, svc, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}
		if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &corev1.Service{}, svc); err != nil {
			return reconcile.Result{}, err
		}

		if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
			return reconcile.Result{}, err
		}

		podTemplate := deployment.Spec.Template.DeepCopy()
		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
			deployment.Spec.Template.Spec = podTemplate.Spec
			deployment.Spec.Replicas = &desired
			return nil
		})
		if err != nil {
			return reconcile.Result{}, err
		}

		scheme := "https"
		if tls == nil {
			scheme = "http"
		}
		serviceSpecs.ReportsURL = &url.URL{
			Scheme: scheme,
			Host:   svc.Name + ":" + strconv.Itoa(int(svc.Spec.Ports[0].Port)),
		}
		reqLogger.Info(fmt.Sprintf("Reports Deployment %s", op))

		// Check deployment status and update conditions
		err = r.updateConditionsFromDeployment(ctx, instance, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace},
			reportsDeploymentConditions)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *CryostatReconciler) createService(ctx context.Context, controller *operatorv1beta1.Cryostat, svc *corev1.Service, exposePort *corev1.ServicePort,
	tlsConfig *openshiftv1.TLSConfig) (*url.URL, error) {
	if err := controllerutil.SetControllerReference(controller, svc, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &corev1.Service{}, svc); err != nil {
		return nil, err
	}

	// Use edge termination by default
	if tlsConfig == nil {
		tlsConfig = &openshiftv1.TLSConfig{
			Termination:                   openshiftv1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: openshiftv1.InsecureEdgeTerminationPolicyRedirect,
		}
	}
	if r.IsOpenShift {
		return r.createRouteForService(ctx, controller, svc, *exposePort, tlsConfig)
	} else {
		if controller.Spec.NetworkOptions == nil {
			return nil, nil
		}
		networkConfig, err := getNetworkConfig(controller, svc)
		if err != nil {
			return nil, err
		}
		if networkConfig == nil || networkConfig.IngressSpec == nil {
			return nil, nil
		}
		return r.createIngressForService(controller, svc, networkConfig)
	}
}

// ErrIngressNotReady is returned when Kubernetes has not yet exposed our services
// so that they may be accessed outside of the cluster
var ErrIngressNotReady = goerrors.New("Ingress configuration not yet available")

func (r *CryostatReconciler) createRouteForService(ctx context.Context, cr *operatorv1beta1.Cryostat,
	svc *corev1.Service, exposePort corev1.ServicePort, tlsConfig *openshiftv1.TLSConfig) (*url.URL, error) {
	logger := r.Log.WithValues("Request.Namespace", svc.Namespace, "Name", svc.Name, "Kind", fmt.Sprintf("%T", &openshiftv1.Route{}))
	route := &openshiftv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(cr, route, r.Scheme); err != nil {
		return nil, err
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		// Update Route spec
		route.Spec = openshiftv1.RouteSpec{
			To: openshiftv1.RouteTargetReference{
				Kind: "Service",
				Name: svc.Name,
			},
			Port: &openshiftv1.RoutePort{TargetPort: exposePort.TargetPort},
			TLS:  tlsConfig,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	logger.Info(fmt.Sprintf("Route %s", op), "Service.Status", fmt.Sprintf("%#v", route.Status))
	if len(route.Status.Ingress) < 1 {
		return nil, ErrIngressNotReady
	}

	return &url.URL{
		Scheme: getProtocol(tlsConfig),
		Host:   route.Status.Ingress[0].Host,
	}, nil
}

func (r *CryostatReconciler) createIngressForService(controller *operatorv1beta1.Cryostat, svc *corev1.Service,
	networkConfig *operatorv1beta1.NetworkConfiguration) (*url.URL, error) {
	logger := r.Log.WithValues("Request.Namespace", svc.Namespace, "Name", svc.Name, "Kind", fmt.Sprintf("%T", &netv1.Ingress{}))

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svc.Name,
			Namespace:   svc.Namespace,
			Annotations: networkConfig.Annotations,
			Labels:      networkConfig.Labels,
		},
		Spec: *networkConfig.IngressSpec,
	}
	if err := controllerutil.SetControllerReference(controller, ingress, r.Scheme); err != nil {
		return nil, err
	}

	found := &netv1.Ingress{}
	err := r.Client.Get(context.Background(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Not found")
		if err := r.Client.Create(context.Background(), ingress); err != nil {
			logger.Error(err, "Could not be created")
			return nil, err
		}
		logger.Info("Created")
		found = ingress
	} else if err != nil {
		logger.Error(err, "Could not be read")
		return nil, err
	}

	logger.Info("Ingress created", "Service.Status", fmt.Sprintf("%#v", found.Status))
	host := ""
	if networkConfig.IngressSpec.Rules != nil && networkConfig.IngressSpec.Rules[0].Host != "" {
		host = networkConfig.IngressSpec.Rules[0].Host
	}

	scheme := "http"
	if networkConfig.IngressSpec.TLS != nil {
		scheme = "https"
	}
	return &url.URL{
		Scheme: scheme,
		Host:   host,
	}, nil
}

func (r *CryostatReconciler) createObjectIfNotExists(ctx context.Context, ns types.NamespacedName, found client.Object, toCreate client.Object) error {
	logger := r.Log.WithValues("Request.Namespace", ns.Namespace, "Name", ns.Name, "Kind", fmt.Sprintf("%T", toCreate))
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

func (r *CryostatReconciler) createRBAC(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	// Create ServiceAccount
	sa, err := resources.NewServiceAccountForCR(cr, r.IsOpenShift)
	if err != nil {
		return err
	}
	newSA := sa.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if err := controllerutil.SetControllerReference(cr, sa, r.Scheme); err != nil {
			return err
		}
		// TODO just replace the labels and annotations we manage, once we allow the user to configure
		// ServiceAccount annotations/labels in the CR, we can simply overwrite them all
		for key, val := range newSA.GetAnnotations() {
			metav1.SetMetaDataAnnotation(&sa.ObjectMeta, key, val)
		}
		if sa.Labels == nil {
			sa.Labels = map[string]string{}
		}
		for key, val := range newSA.GetLabels() {
			// TODO use metav1.SetMetaDataLabel when updating client-go, replace above initialization
			sa.Labels[key] = val
		}
		// Pod needs SA token, do not allow to be disabled
		sa.AutomountServiceAccountToken = newSA.AutomountServiceAccountToken
		// Secrets, ImagePullSecrets are modified by Kubernetes/OpenShift
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("ServiceAccount %s", op), "name", sa.Name, "namespace", sa.Namespace)

	// Create Role
	role := resources.NewRoleForCR(cr)
	if err := controllerutil.SetControllerReference(cr, role, r.Scheme); err != nil {
		return err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace},
		&rbacv1.Role{}, role); err != nil {
		return err
	}

	// Create RoleBinding
	binding := resources.NewRoleBindingForCR(cr)
	if err := controllerutil.SetControllerReference(cr, binding, r.Scheme); err != nil {
		return err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace},
		&rbacv1.RoleBinding{}, binding); err != nil {
		return err
	}

	// Create ClusterRoleBinding
	clusterBinding := resources.NewClusterRoleBindingForCR(cr)
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: clusterBinding.Name},
		&rbacv1.ClusterRoleBinding{}, clusterBinding); err != nil {
		return err
	}
	// ClusterRoleBinding can't be owned by namespaced CR, clean up using finalizer

	return nil
}

func (r *CryostatReconciler) deleteClusterRoleBinding(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)

	clusterBinding := resources.NewClusterRoleBindingForCR(cr)
	err := r.Delete(ctx, clusterBinding)
	if err != nil {
		if kerrors.IsNotFound(err) {
			reqLogger.Info("ClusterRoleBinding not found, proceeding with deletion", "bindingName", clusterBinding.Name)
			return nil
		}
		reqLogger.Error(err, "failed to delete ClusterRoleBinding", "bindingName", clusterBinding.Name)
		return err
	}
	reqLogger.Info("deleted ClusterRoleBinding", "bindingName", clusterBinding.Name)
	return nil
}

func (r *CryostatReconciler) getImageTags() *resources.ImageTags {
	return &resources.ImageTags{
		CoreImageTag:       r.getEnvOrDefault(coreImageTagEnv, resources.DefaultCoreImageTag),
		DatasourceImageTag: r.getEnvOrDefault(datasourceImageTagEnv, resources.DefaultDatasourceImageTag),
		GrafanaImageTag:    r.getEnvOrDefault(grafanaImageTagEnv, resources.DefaultGrafanaImageTag),
		ReportsImageTag:    r.getEnvOrDefault(reportsImageTagEnv, resources.DefaultReportsImageTag),
	}
}

func (r *CryostatReconciler) getEnvOrDefault(name string, defaultVal string) string {
	val := r.GetEnv(name)
	if len(val) > 0 {
		return val
	}
	return defaultVal
}

// fsGroup to use when not constrained
const defaultFSGroup int64 = 18500

func (r *CryostatReconciler) getFSGroup(ctx context.Context, namespace string) (*int64, error) {
	if r.IsOpenShift {
		// Check namespace for supplemental groups annotation
		ns := &corev1.Namespace{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		if err != nil {
			return nil, err
		}

		supGroups, found := ns.Annotations[securityv1.SupplementalGroupsAnnotation]
		if found {
			return parseSupGroups(supGroups)
		}
	}
	fsGroup := defaultFSGroup
	return &fsGroup, nil
}

func parseSupGroups(supGroups string) (*int64, error) {
	// Extract the start value from the annotation
	match := supGroupRegexp.FindString(supGroups)
	if len(match) == 0 {
		return nil, fmt.Errorf("no group ID found in %s annotation",
			securityv1.SupplementalGroupsAnnotation)
	}
	gid, err := strconv.ParseInt(match, 10, 64)
	if err != nil {
		return nil, err
	}
	return &gid, nil
}

func (r *CryostatReconciler) createConsoleLink(ctx context.Context, cr *operatorv1beta1.Cryostat, url string) error {
	link := resources.NewConsoleLink(cr, url)
	return r.createObjectIfNotExists(ctx, types.NamespacedName{Name: link.Name}, &consolev1.ConsoleLink{}, link)
}

func (r *CryostatReconciler) deleteConsoleLink(ctx context.Context, cr *operatorv1beta1.Cryostat) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)
	link := resources.NewConsoleLink(cr, "")
	err := r.Client.Delete(ctx, link)
	if err != nil {
		if kerrors.IsNotFound(err) {
			reqLogger.Info("ConsoleLink not found, proceeding with deletion", "linkName", link.Name)
			return nil
		}
		reqLogger.Error(err, "failed to delete ConsoleLink", "linkName", link.Name)
		return err
	}
	reqLogger.Info("deleted ConsoleLink", "linkName", link.Name)
	return nil
}

func (r *CryostatReconciler) addCorsAllowedOriginIfNotPresent(ctx context.Context, allowedOrigin string, cr *operatorv1beta1.Cryostat) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)
	apiServer := &configv1.APIServer{}
	err := r.Client.Get(context.Background(), types.NamespacedName{Name: apiServerName}, apiServer)
	if err != nil {
		reqLogger.Error(err, "Failed to get APIServer config")
		return err
	}

	allowedOriginAsRegex := regexp.QuoteMeta(allowedOrigin)

	for _, origin := range apiServer.Spec.AdditionalCORSAllowedOrigins {
		if origin == allowedOriginAsRegex {
			return nil
		}
	}

	apiServer.Spec.AdditionalCORSAllowedOrigins = append(
		apiServer.Spec.AdditionalCORSAllowedOrigins,
		allowedOriginAsRegex,
	)

	err = r.Client.Update(ctx, apiServer)
	if err != nil {
		reqLogger.Error(err, "Failed to update APIServer CORS allowed origins")
		return err
	}

	return nil
}

func (r *CryostatReconciler) deleteCorsAllowedOrigins(ctx context.Context, allowedOrigin string, cr *operatorv1beta1.Cryostat) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)
	apiServer := &configv1.APIServer{}
	err := r.Client.Get(context.Background(), types.NamespacedName{Name: apiServerName}, apiServer)
	if err != nil {
		reqLogger.Error(err, "Failed to get APIServer config")
		return err
	}

	allowedOriginAsRegex := regexp.QuoteMeta(allowedOrigin)

	for i, origin := range apiServer.Spec.AdditionalCORSAllowedOrigins {
		if origin == allowedOriginAsRegex {
			apiServer.Spec.AdditionalCORSAllowedOrigins = append(
				apiServer.Spec.AdditionalCORSAllowedOrigins[:i],
				apiServer.Spec.AdditionalCORSAllowedOrigins[i+1:]...)
			break
		}
	}

	err = r.Client.Update(ctx, apiServer)
	if err != nil {
		reqLogger.Error(err, "Failed to remove Cryostat origin from APIServer CORS allowed origins")
		return err
	}

	return nil
}

func (r *CryostatReconciler) updateCondition(ctx context.Context, cr *operatorv1beta1.Cryostat,
	condType operatorv1beta1.CryostatConditionType, status metav1.ConditionStatus, reason string, message string) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    string(condType),
		Status:  status,
		Reason:  reason,
		Message: message,
	})
	err := r.Client.Status().Update(ctx, cr)
	if err != nil {
		reqLogger.Error(err, "failed to update condition", "type", condType)
	}
	return err
}

func (r *CryostatReconciler) updateConditionsFromDeployment(ctx context.Context, cr *operatorv1beta1.Cryostat,
	deployKey types.NamespacedName, mapping deploymentConditionTypeMap) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.Namespace, "Request.Name", cr.Name)

	// Get deployment's latest conditions
	deploy := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: deployKey.Name, Namespace: deployKey.Namespace}, deploy)
	if err != nil {
		return err
	}

	// Associate deployment conditions with Cryostat conditions
	for condType, deployCondType := range mapping {
		condition := findDeployCondition(deploy.Status.Conditions, deployCondType)
		if condition == nil {
			removeConditionIfPresent(cr, condType)
		} else {
			meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
				Type:    string(condType),
				Status:  metav1.ConditionStatus(condition.Status),
				Reason:  condition.Reason,
				Message: condition.Message,
			})
		}
	}
	err = r.Client.Status().Update(ctx, cr)
	if err != nil {
		reqLogger.Error(err, "failed to update conditions for deployment", "deployment", deploy.Name)
	}
	return err
}

func getProtocol(tlsConfig *openshiftv1.TLSConfig) string {
	if tlsConfig == nil {
		return "http"
	}
	return "https"
}

func requeueIfIngressNotReady(log logr.Logger, err error) (reconcile.Result, error) {
	if err == ErrIngressNotReady {
		log.Info(err.Error())
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return reconcile.Result{}, err
}

func getNetworkConfig(controller *operatorv1beta1.Cryostat, svc *corev1.Service) (*operatorv1beta1.NetworkConfiguration, error) {
	if svc.Name == controller.Name {
		return controller.Spec.NetworkOptions.CoreConfig, nil
	} else if svc.Name == controller.Name+"-grafana" {
		return controller.Spec.NetworkOptions.GrafanaConfig, nil
	} else {
		return nil, goerrors.New("Service name not recognized")
	}
}

func removeConditionIfPresent(cr *operatorv1beta1.Cryostat, condType ...operatorv1beta1.CryostatConditionType) {
	for _, ct := range condType {
		found := meta.FindStatusCondition(cr.Status.Conditions, string(ct))
		if found != nil {
			meta.RemoveStatusCondition(&cr.Status.Conditions, string(ct))
		}
	}
}

func findDeployCondition(conditions []appsv1.DeploymentCondition, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for _, condition := range conditions {
		if condition.Type == condType {
			return &condition
		}
	}
	return nil
}
