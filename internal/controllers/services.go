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

func (r *Reconciler) reconcileAgentService(ctx context.Context, cr *model.CryostatInstance) error {
	svc := newAgentService(cr)
	config := configureAgentService(cr)

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

func (r *Reconciler) newAgentHeadlessService(cr *model.CryostatInstance, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.AgentHeadlessServiceName(r.gvk, cr),
			Namespace: namespace,
		},
	}
}

func (r *Reconciler) reconcileAgentHeadlessServices(ctx context.Context, cr *model.CryostatInstance) error {
	svcType := corev1.ServiceTypeClusterIP
	// TODO make configurable through CRD
	config := &operatorv1beta2.ServiceConfig{
		ServiceType: &svcType,
		Labels:      common.LabelsForTargetNamespaceObject(cr),
	}
	configureService(config, cr.Name, "agent")

	// Create a headless Service in each target namespace
	for _, ns := range cr.TargetNamespaces {
		svc := r.newAgentHeadlessService(cr, ns)

		err := r.createOrUpdateService(ctx, svc, nil, config, func() error {
			// Select agent auto-configuration labels
			svc.Spec.Selector = map[string]string{
				constants.AgentLabelCryostatName:      cr.Name,
				constants.AgentLabelCryostatNamespace: cr.InstallNamespace,
			}
			svc.Spec.Ports = []corev1.ServicePort{
				{
					Name:       constants.HttpPortName,
					Port:       9977, // TODO make configurable
					TargetPort: intstr.IntOrString{IntVal: 9977},
				},
			}
			svc.Spec.ClusterIP = corev1.ClusterIPNone
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Delete any RoleBindings in target namespaces that are no longer requested
	for _, ns := range toDelete(cr) {
		svc := r.newAgentHeadlessService(cr, ns)
		err := r.deleteService(ctx, svc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) finalizeAgentHeadlessServices(ctx context.Context, cr *model.CryostatInstance) error {
	for _, ns := range cr.TargetNamespaces {
		svc := r.newAgentHeadlessService(cr, ns)
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

func configureAgentService(cr *model.CryostatInstance) *operatorv1beta2.AgentServiceConfig {
	// Check CR for config
	var config *operatorv1beta2.AgentServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.AgentConfig == nil {
		config = &operatorv1beta2.AgentServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.AgentConfig.DeepCopy()
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

func configureService(config *operatorv1beta2.ServiceConfig, appLabel string, componentLabel string) {
	if config.ServiceType == nil {
		svcType := corev1.ServiceTypeClusterIP
		config.ServiceType = &svcType
	}
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
		if owner != nil {
			if err := controllerutil.SetControllerReference(owner, svc, r.Scheme); err != nil {
				return err
			}
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
