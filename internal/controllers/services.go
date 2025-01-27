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
	"fmt"
	"maps"
	"net/url"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	common "github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileCoreService(ctx context.Context, cr *model.CryostatInstance,
	tls *resource_definitions.TLSConfig, specs *resource_definitions.ServiceSpecs) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
	}
	config := configureCoreService(cr)

	err := r.createOrUpdateService(ctx, svc, cr.Object, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "cryostat",
		}
		appProtocol := "http"
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:        constants.HttpPortName,
				Port:        *config.HTTPPort,
				TargetPort:  intstr.IntOrString{IntVal: constants.AuthProxyHttpContainerPort},
				AppProtocol: &appProtocol,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	if r.IsOpenShift {
		return r.reconcileCoreRoute(ctx, svc, cr, tls, specs)
	} else {
		return r.reconcileCoreIngress(ctx, cr, specs)
	}
}

func (r *Reconciler) reconcileReportsService(ctx context.Context, cr *model.CryostatInstance,
	tls *resource_definitions.TLSConfig, specs *resource_definitions.ServiceSpecs) error {
	config := configureReportsService(cr)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-reports",
			Namespace: cr.InstallNamespace,
		},
	}

	if cr.Spec.ReportOptions == nil || cr.Spec.ReportOptions.Replicas == 0 {
		// Delete service if it exists
		return r.deleteService(ctx, svc)
	}
	err := r.createOrUpdateService(ctx, svc, cr.Object, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "reports",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       constants.HttpPortName,
				Port:       *config.HTTPPort,
				TargetPort: intstr.IntOrString{IntVal: constants.ReportsContainerPort},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Set reports URL for deployment to use
	scheme := "https"
	if tls == nil {
		scheme = "http"
	}
	specs.ReportsURL = &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", svc.Name, *config.HTTPPort),
	}
	return nil
}

func newAgentService(cr *model.CryostatInstance) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-agent",
			Namespace: cr.InstallNamespace,
		},
	}
}

func (r *Reconciler) reconcileAgentGatewayService(ctx context.Context, cr *model.CryostatInstance) error {
	svc := newAgentService(cr)
	config := configureAgentGatewayService(cr)

	return r.createOrUpdateService(ctx, svc, cr.Object, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "cryostat",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       constants.HttpPortName,
				Port:       *config.HTTPPort,
				TargetPort: intstr.IntOrString{IntVal: constants.AgentProxyContainerPort},
			},
		}
		return nil
	})
}

func (r *Reconciler) reconcileDatabaseService(ctx context.Context, cr *model.CryostatInstance, tls *resource_definitions.TLSConfig,
	specs *resource_definitions.ServiceSpecs) error {
	config := configureDatabaseService(cr)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-database",
			Namespace: cr.InstallNamespace,
		},
	}

	port := *config.DatabasePort
	err := r.createOrUpdateService(ctx, svc, cr.Object, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "database",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "jdbc",
				Port:       port,
				TargetPort: intstr.IntOrString{IntVal: constants.DatabasePort},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Set database URL for deployment to use
	scheme := "jdbc:posgtresql"
	specs.DatabaseURL = &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", svc.Name, port),
		Path:   resource_definitions.DatabaseName,
	}
	return nil
}

func (r *Reconciler) reconcileStorageService(ctx context.Context, cr *model.CryostatInstance, tls *resource_definitions.TLSConfig,
	specs *resource_definitions.ServiceSpecs) error {
	config := configureStorageService(cr, tls)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-storage",
			Namespace: cr.InstallNamespace,
		},
	}

	scheme := "http"
	if tls != nil {
		scheme = "https"
	}
	err := r.createOrUpdateService(ctx, svc, cr.Object, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "storage",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       constants.HttpPortName,
				Port:       *config.HTTPPort,
				TargetPort: intstr.IntOrString{IntVal: constants.StoragePort},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Set storage URL for deployment to use
	namedPort, err := GetNamedPort(constants.HttpPortName, svc)
	if err != nil {
		return err
	}
	specs.StorageURL = &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, namedPort.Port),
	}
	return nil
}

func (r *Reconciler) newAgentCallbackService(cr *model.CryostatInstance, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.AgentCallbackServiceName(r.gvk, cr),
			Namespace: namespace,
		},
	}
}

func (r *Reconciler) reconcileAgentCallbackServices(ctx context.Context, cr *model.CryostatInstance) error {
	config := configureAgentCallbackService(cr)

	// Create a headless Service in each target namespace
	for _, ns := range cr.TargetNamespaces {
		svc := r.newAgentCallbackService(cr, ns)

		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
			// Update labels and annotations
			common.MergeLabelsAndAnnotations(&svc.ObjectMeta, config.Labels, config.Annotations)

			// Select agent auto-configuration labels
			svc.Spec.Selector = map[string]string{
				constants.AgentLabelCryostatName:      cr.Name,
				constants.AgentLabelCryostatNamespace: cr.InstallNamespace,
			}
			// No Ports. We contact the pods directly using their container ports.
			svc.Spec.Ports = nil

			// Headless service
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			svc.Spec.ClusterIP = corev1.ClusterIPNone
			return nil
		})
		if err != nil {
			return err
		}
		r.Log.Info(fmt.Sprintf("Service %s", op), "name", svc.Name, "namespace", svc.Namespace)
	}

	// Delete any RoleBindings in target namespaces that are no longer requested
	for _, ns := range toDelete(cr) {
		svc := r.newAgentCallbackService(cr, ns)
		err := r.deleteService(ctx, svc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) finalizeAgentCallbackServices(ctx context.Context, cr *model.CryostatInstance) error {
	for _, ns := range cr.TargetNamespaces {
		svc := r.newAgentCallbackService(cr, ns)
		err := r.deleteService(ctx, svc)
		if err != nil {
			return err
		}
	}
	return nil
}

func configureCoreService(cr *model.CryostatInstance) *operatorv1beta2.CoreServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.CoreServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.CoreConfig == nil {
		config = &operatorv1beta2.CoreServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.CoreConfig.DeepCopy()
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "cryostat")

	// Apply default HTTP and JMX port if not provided
	if config.HTTPPort == nil {
		httpPort := constants.AuthProxyHttpContainerPort
		config.HTTPPort = &httpPort
	}

	return config
}

func configureReportsService(cr *model.CryostatInstance) *operatorv1beta2.ReportsServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.ReportsServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.ReportsConfig == nil {
		config = &operatorv1beta2.ReportsServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.ReportsConfig.DeepCopy()
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "reports")

	// Apply default HTTP port if not provided
	if config.HTTPPort == nil {
		httpPort := constants.ReportsContainerPort
		config.HTTPPort = &httpPort
	}

	return config
}

func configureDatabaseService(cr *model.CryostatInstance) *operatorv1beta2.DatabaseServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.DatabaseServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.DatabaseConfig == nil {
		config = &operatorv1beta2.DatabaseServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.DatabaseConfig.DeepCopy()
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "database")

	// Apply default HTTP port if not provided
	if config.DatabasePort == nil {
		dbPort := constants.DatabasePort
		config.DatabasePort = &dbPort
	}

	return config
}

func configureStorageService(cr *model.CryostatInstance, tls *resource_definitions.TLSConfig) *operatorv1beta2.StorageServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.StorageServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.StorageConfig == nil {
		config = &operatorv1beta2.StorageServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.StorageConfig.DeepCopy()
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "storage")

	// Apply default HTTP port if not provided
	if config.HTTPPort == nil {
		httpPort := constants.StoragePort
		config.HTTPPort = &httpPort
	}

	return config
}

func configureAgentGatewayService(cr *model.CryostatInstance) *operatorv1beta2.AgentGatewayServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.AgentGatewayServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.AgentGatewayConfig == nil {
		config = &operatorv1beta2.AgentGatewayServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.AgentGatewayConfig.DeepCopy()
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "cryostat-agent-gateway")

	// Apply default HTTP port if not provided
	if config.HTTPPort == nil {
		httpPort := constants.AgentProxyContainerPort
		config.HTTPPort = &httpPort
	}

	return config
}

func configureAgentCallbackService(cr *model.CryostatInstance) *operatorv1beta2.AgentCallbackServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.AgentCallbackServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.AgentCallbackConfig == nil {
		config = &operatorv1beta2.AgentCallbackServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.AgentCallbackConfig.DeepCopy()
	}

	// Apply common service defaults
	configureMetadata(&config.ResourceMetadata, cr.Name, "cryostat-agent-callback")
	// Add target namespace labels used by our controller watches
	maps.Copy(config.ResourceMetadata.Labels, common.LabelsForTargetNamespaceObject(cr))

	return config
}

func configureService(config *operatorv1beta2.ServiceConfig, appLabel string, componentLabel string) {
	if config.ServiceType == nil {
		svcType := corev1.ServiceTypeClusterIP
		config.ServiceType = &svcType
	}
	configureMetadata(&config.ResourceMetadata, appLabel, componentLabel)
}

func configureMetadata(config *operatorv1beta2.ResourceMetadata, appLabel string, componentLabel string) {
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	if config.Annotations == nil {
		config.Annotations = map[string]string{}
	}

	// Add required labels, overriding any user-specified labels with the same keys
	config.Labels["app"] = appLabel
	config.Labels["component"] = componentLabel
	config.Labels["app.kubernetes.io/name"] = "cryostat"
	config.Labels["app.kubernetes.io/instance"] = appLabel
	config.Labels["app.kubernetes.io/component"] = componentLabel
	config.Labels["app.kubernetes.io/part-of"] = "cryostat"
}

func (r *Reconciler) createOrUpdateService(ctx context.Context, svc *corev1.Service, owner metav1.Object,
	config *operatorv1beta2.ServiceConfig, delegate controllerutil.MutateFn) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		// Update labels and annotations
		common.MergeLabelsAndAnnotations(&svc.ObjectMeta, config.Labels, config.Annotations)

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
			return err
		}

		// Update the service type
		svc.Spec.Type = *config.ServiceType
		// Call the delegate for service-specific mutations
		return delegate()
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Service %s", op), "name", svc.Name, "namespace", svc.Namespace)
	return nil
}

func (r *Reconciler) deleteService(ctx context.Context, svc *corev1.Service) error {
	err := r.Client.Delete(ctx, svc)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete service", "name", svc.Name, "namespace", svc.Namespace)
		return err
	}
	r.Log.Info("Service deleted", "name", svc.Name, "namespace", svc.Namespace)
	return nil
}

func GetHTTPPort(svc *corev1.Service) (*corev1.ServicePort, error) {
	return GetNamedPort("http", svc)
}

func GetNamedPort(portName string, svc *corev1.Service) (*corev1.ServicePort, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == portName {
			return &port, nil
		}
	}
	// Shouldn't happen
	return nil, fmt.Errorf("no \"%s\" port in %s service in %s namespace", constants.HttpPortName, svc.Name, svc.Namespace)
}
