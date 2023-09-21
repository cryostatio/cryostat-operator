// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	common "github.com/cryostatio/cryostat-operator/internal/controllers/common"
	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	openshiftv1 "github.com/openshift/api/route/v1"
	securityv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcilerConfig contains common configuration parameters for
// CommonReconciler implementations
type ReconcilerConfig struct {
	client.Client
	Log                    logr.Logger
	Scheme                 *runtime.Scheme
	IsOpenShift            bool
	IsCertManagerInstalled bool
	EventRecorder          record.EventRecorder
	RESTMapper             meta.RESTMapper
	common.ReconcilerTLS
}

// CommonReconciler is an interface for shared behaviour
// between the ClusterCryostat and Cryostat reconcilers
type CommonReconciler interface {
	reconcile.Reconciler
	GetConfig() *ReconcilerConfig
}

type Reconciler struct {
	*ReconcilerConfig
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

func (r *Reconciler) reconcileCryostat(ctx context.Context, cr *model.CryostatInstance) (ctrl.Result, error) {
	result, err := r.reconcile(ctx, cr)
	return result, r.checkConflicts(cr, err)
}

func (r *Reconciler) reconcile(ctx context.Context, cr *model.CryostatInstance) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)

	// Check if this Cryostat is being deleted
	if cr.Object.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(cr.Object, cryostatFinalizer) {
			// Perform finalizer logic related to RBAC objects
			err := r.finalizeRBAC(ctx, cr)
			if err != nil {
				return reconcile.Result{}, err
			}

			// OpenShift-specific
			err = r.finalizeOpenShift(ctx, cr)
			if err != nil {
				return reconcile.Result{}, err
			}

			err = common.RemoveFinalizer(ctx, r.Client, cr.Object, cryostatFinalizer)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		// Ready for deletion
		return reconcile.Result{}, nil
	}

	// Add our finalizer, so we can clean up Cryostat resources upon deletion
	if !controllerutil.ContainsFinalizer(cr.Object, cryostatFinalizer) {
		err := common.AddFinalizer(ctx, r.Client, cr.Object, cryostatFinalizer)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger.Info("Spec", "Minimal", cr.Spec.Minimal)

	// Create lock config map or fail if owned by another CR
	err := r.reconcileLockConfigMap(ctx, cr)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.reconcilePVC(ctx, cr)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.reconcileSecrets(ctx, cr)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set up TLS using cert-manager, if available
	var tlsConfig *resources.TLSConfig
	if r.IsCertManagerEnabled(cr) {
		tlsConfig, err = r.setupTLS(ctx, cr)
		if err != nil {
			if err == common.ErrCertNotReady {
				condErr := r.updateCondition(ctx, cr, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
					reasonWaitingForCert, "Waiting for certificates to become ready.")
				if condErr != nil {
					return reconcile.Result{}, err
				}
				return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
			}
			if err == errCertManagerMissing {
				r.updateCondition(ctx, cr, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionFalse,
					reasonCertManagerUnavailable, eventCertManagerUnavailableMsg)
			}
			reqLogger.Error(err, "Failed to set up TLS for Cryostat")
			return reconcile.Result{}, err
		}

		err = r.updateCondition(ctx, cr, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
			reasonAllCertsReady, "All certificates for Cryostat components are ready.")
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		err = r.updateCondition(ctx, cr, operatorv1beta1.ConditionTypeTLSSetupComplete, metav1.ConditionTrue,
			reasonCertManagerDisabled, "TLS setup has been disabled.")
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Reconcile RBAC resources for Cryostat
	err = r.reconcileRBAC(ctx, cr)
	if err != nil {
		return reconcile.Result{}, err
	}

	serviceSpecs := &resources.ServiceSpecs{}
	err = r.reconcileGrafanaService(ctx, cr, tlsConfig, serviceSpecs)
	if err != nil {
		return requeueIfIngressNotReady(reqLogger, err)
	}
	err = r.reconcileCoreService(ctx, cr, tlsConfig, serviceSpecs)
	if err != nil {
		return requeueIfIngressNotReady(reqLogger, err)
	}

	imageTags := r.getImageTags()
	fsGroup, err := r.getFSGroup(ctx, cr.InstallNamespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	reportsResult, err := r.reconcileReports(ctx, reqLogger, cr, tlsConfig, imageTags, serviceSpecs)
	if err != nil {
		return reportsResult, err
	}

	deployment := resources.NewDeploymentForCR(cr, serviceSpecs, imageTags, tlsConfig, *fsGroup, r.IsOpenShift)
	err = r.createOrUpdateDeployment(ctx, deployment, cr.Object)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Update CR Status
	if serviceSpecs.CoreURL != nil {
		cr.Status.ApplicationURL = serviceSpecs.CoreURL.String()
	}
	*cr.TargetNamespaceStatus = cr.TargetNamespaces
	err = r.Client.Status().Update(ctx, cr.Object)
	if err != nil {
		return reconcile.Result{}, err
	}

	// OpenShift-specific
	err = r.reconcileOpenShift(ctx, cr)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Check deployment status and update conditions
	err = r.updateConditionsFromDeployment(ctx, cr, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace},
		mainDeploymentConditions)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Successfully reconciled Cryostat")
	return reconcile.Result{}, nil
}

func (r *Reconciler) setupWithManager(mgr ctrl.Manager, obj client.Object,
	impl reconcile.Reconciler) error {
	c := ctrl.NewControllerManagedBy(mgr).
		For(obj)

	// Watch for changes to secondary resources and requeue the owner Cryostat
	resources := []client.Object{&appsv1.Deployment{}, &corev1.Service{}, &corev1.Secret{}, &corev1.PersistentVolumeClaim{},
		&corev1.ServiceAccount{}, &rbacv1.Role{}, &rbacv1.RoleBinding{}, &netv1.Ingress{}}
	if r.IsOpenShift {
		resources = append(resources, &openshiftv1.Route{})
	}
	// Can only check this at startup
	if r.IsCertManagerInstalled {
		resources = append(resources, &certv1.Issuer{}, &certv1.Certificate{})
	}

	for _, resource := range resources {
		c = c.Owns(resource)
	}

	return c.Complete(impl)
}

func (r *Reconciler) reconcileReports(ctx context.Context, reqLogger logr.Logger, cr *model.CryostatInstance,
	tls *resources.TLSConfig, imageTags *resources.ImageTags, serviceSpecs *resources.ServiceSpecs) (reconcile.Result, error) {
	reqLogger.Info("Spec", "Reports", cr.Spec.ReportOptions)

	desired := int32(0)
	if cr.Spec.ReportOptions != nil {
		desired = cr.Spec.ReportOptions.Replicas
	}

	err := r.reconcileReportsService(ctx, cr, tls, serviceSpecs)
	if err != nil {
		return reconcile.Result{}, err
	}
	deployment := resources.NewDeploymentForReports(cr, imageTags, tls, r.IsOpenShift)
	if desired == 0 {
		if err := r.Client.Delete(ctx, deployment); err != nil && !kerrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}

		removeConditionIfPresent(cr, operatorv1beta1.ConditionTypeReportsDeploymentAvailable,
			operatorv1beta1.ConditionTypeReportsDeploymentProgressing,
			operatorv1beta1.ConditionTypeReportsDeploymentReplicaFailure)
		err := r.Client.Status().Update(ctx, cr.Object)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if desired > 0 {
		err = r.createOrUpdateDeployment(ctx, deployment, cr.Object)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Check deployment status and update conditions
		err = r.updateConditionsFromDeployment(ctx, cr, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace},
			reportsDeploymentConditions)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) getImageTags() *resources.ImageTags {
	return &resources.ImageTags{
		CoreImageTag:       r.getEnvOrDefault(coreImageTagEnv, DefaultCoreImageTag),
		DatasourceImageTag: r.getEnvOrDefault(datasourceImageTagEnv, DefaultDatasourceImageTag),
		GrafanaImageTag:    r.getEnvOrDefault(grafanaImageTagEnv, DefaultGrafanaImageTag),
		ReportsImageTag:    r.getEnvOrDefault(reportsImageTagEnv, DefaultReportsImageTag),
	}
}

func (r *Reconciler) getEnvOrDefault(name string, defaultVal string) string {
	val := r.GetEnv(name)
	if len(val) > 0 {
		return val
	}
	return defaultVal
}

// fsGroup to use when not constrained
const defaultFSGroup int64 = 18500

func (r *Reconciler) getFSGroup(ctx context.Context, namespace string) (*int64, error) {
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

func (r *Reconciler) updateCondition(ctx context.Context, cr *model.CryostatInstance,
	condType operatorv1beta1.CryostatConditionType, status metav1.ConditionStatus, reason string, message string) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)
	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:    string(condType),
		Status:  status,
		Reason:  reason,
		Message: message,
	})
	err := r.Client.Status().Update(ctx, cr.Object)
	if err != nil {
		reqLogger.Error(err, "failed to update condition", "type", condType)
	}
	return err
}

func (r *Reconciler) updateConditionsFromDeployment(ctx context.Context, cr *model.CryostatInstance,
	deployKey types.NamespacedName, mapping deploymentConditionTypeMap) error {
	reqLogger := r.Log.WithValues("Request.Namespace", cr.InstallNamespace, "Request.Name", cr.Name)

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
	err = r.Client.Status().Update(ctx, cr.Object)
	if err != nil {
		reqLogger.Error(err, "failed to update conditions for deployment", "deployment", deploy.Name)
	}
	return err
}

var errSelectorModified error = errors.New("deployment selector has been modified")

func (r *Reconciler) createOrUpdateDeployment(ctx context.Context, deploy *appsv1.Deployment, owner metav1.Object) error {
	deployCopy := deploy.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		// Merge any required labels and annotations
		common.MergeLabelsAndAnnotations(&deploy.ObjectMeta, deployCopy.Labels, deployCopy.Annotations)
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, deploy, r.Scheme); err != nil {
			return err
		}
		// Immutable, only updated when the deployment is created
		if deploy.CreationTimestamp.IsZero() {
			deploy.Spec.Selector = deployCopy.Spec.Selector
		} else if !cmp.Equal(deploy.Spec.Selector, deployCopy.Spec.Selector) {
			// Return error so deployment can be recreated
			return errSelectorModified
		}
		// Set the replica count and update strategy
		deploy.Spec.Replicas = deployCopy.Spec.Replicas
		deploy.Spec.Strategy = deployCopy.Spec.Strategy

		// Update pod template spec to propagate any changes from Cryostat CR
		deploy.Spec.Template.Spec = deployCopy.Spec.Template.Spec
		// Update pod template metadata
		common.MergeLabelsAndAnnotations(&deploy.Spec.Template.ObjectMeta, deployCopy.Spec.Template.Labels,
			deployCopy.Spec.Template.Annotations)
		return nil
	})
	if err != nil {
		if err == errSelectorModified {
			return r.recreateDeployment(ctx, deployCopy, owner)
		}
		return err
	}
	r.Log.Info(fmt.Sprintf("Deployment %s", op), "name", deploy.Name, "namespace", deploy.Namespace)
	return nil
}

func (r *Reconciler) recreateDeployment(ctx context.Context, deploy *appsv1.Deployment, owner metav1.Object) error {
	// Delete and recreate deployment
	err := r.deleteDeployment(ctx, deploy)
	if err != nil {
		return err
	}
	return r.createOrUpdateDeployment(ctx, deploy, owner)
}

func (r *Reconciler) deleteDeployment(ctx context.Context, deploy *appsv1.Deployment) error {
	err := r.Client.Delete(ctx, deploy)
	if err != nil && !kerrors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete deployment", "name", deploy.Name, "namespace", deploy.Namespace)
		return err
	}
	r.Log.Info("Deployment deleted", "name", deploy.Name, "namespace", deploy.Namespace)
	return nil
}

const eventNameConflictReason = "CryostatNameConflict"

func (r *Reconciler) checkConflicts(cr *model.CryostatInstance, err error) error {
	if err == nil {
		return nil
	}
	alreadyOwned, ok := err.(*controllerutil.AlreadyOwnedError)
	if !ok {
		return err
	}
	r.Log.Error(err, "Could not process custom resource")
	metaType, err := meta.TypeAccessor(alreadyOwned.Object)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("This instance needs to manage the %s %s in namespace %s, but it is already owned by %s %s. "+
		"Please choose a different name for your instance.",
		metaType.GetKind(), alreadyOwned.Object.GetName(), alreadyOwned.Object.GetNamespace(),
		alreadyOwned.Owner.Kind, alreadyOwned.Owner.Name)
	r.EventRecorder.Event(cr.Object, corev1.EventTypeWarning, eventNameConflictReason, msg)
	// Log the event message as well
	r.Log.Info(msg)
	return alreadyOwned
}

func requeueIfIngressNotReady(log logr.Logger, err error) (reconcile.Result, error) {
	if err == ErrIngressNotReady {
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return reconcile.Result{}, err
}

func removeConditionIfPresent(cr *model.CryostatInstance, condType ...operatorv1beta1.CryostatConditionType) {
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
