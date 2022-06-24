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

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// FIXME dedup
const (
	cryostatHTTPContainerPort int32  = 8181
	cryostatJMXContainerPort  int32  = 9091
	grafanaContainerPort      int32  = 3000
	datasourceContainerPort   int32  = 8080
	reportsContainerPort      int32  = 10000
	loopbackAddress           string = "127.0.0.1"
)

func coreService(cr *operatorv1beta1.Cryostat) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}
}

func (r *CryostatReconciler) reconcileCoreService(ctx context.Context, cr *operatorv1beta1.Cryostat) (*corev1.Service, error) {
	config := configureCoreService(cr)
	svc := coreService(cr)
	return r.createOrUpdateService(ctx, svc, cr, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "cryostat",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       *config.HTTPPort,
				TargetPort: intstr.IntOrString{IntVal: cryostatHTTPContainerPort},
			},
			{
				Name:       "jfr-jmx",
				Port:       *config.JMXPort,
				TargetPort: intstr.IntOrString{IntVal: cryostatJMXContainerPort},
			},
		}
		return nil
	})
}

func grafanaService(cr *operatorv1beta1.Cryostat) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
		},
	}
}

func (r *CryostatReconciler) reconcileGrafanaService(ctx context.Context, cr *operatorv1beta1.Cryostat) (*corev1.Service, error) {
	config := configureGrafanaService(cr)
	svc := grafanaService(cr)

	if cr.Spec.Minimal {
		// Delete service if it exists
		return r.deleteService(ctx, svc)
	}
	return r.createOrUpdateService(ctx, svc, cr, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "cryostat",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       *config.HTTPPort,
				TargetPort: intstr.IntOrString{IntVal: 3000},
			},
		}
		return nil
	})
}

func reportsService(cr *operatorv1beta1.Cryostat) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-reports",
			Namespace: cr.Namespace,
		},
	}
}

func (r *CryostatReconciler) reconcileReportsService(ctx context.Context, cr *operatorv1beta1.Cryostat) (*corev1.Service, error) {
	config := configureReportsService(cr)
	svc := reportsService(cr)
	if cr.Spec.ReportOptions.Replicas == 0 {
		// Delete service if it exists
		return r.deleteService(ctx, svc)
	}
	return r.createOrUpdateService(ctx, svc, cr, &config.ServiceConfig, func() error {
		svc.Spec.Selector = map[string]string{
			"app":       cr.Name,
			"component": "reports",
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       *config.HTTPPort,
				TargetPort: intstr.IntOrString{IntVal: reportsContainerPort},
			},
		}
		return nil
	})
}

func configureCoreService(cr *operatorv1beta1.Cryostat) *operatorv1beta1.CoreServiceConfig {
	// Check CR for config
	var config *operatorv1beta1.CoreServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.CoreConfig == nil {
		config = &operatorv1beta1.CoreServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.CoreConfig
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "cryostat")

	// Apply default HTTP and JMX port if not provided
	if config.HTTPPort == nil {
		httpPort := cryostatHTTPContainerPort
		config.HTTPPort = &httpPort
	}
	if config.JMXPort == nil {
		jmxPort := cryostatJMXContainerPort
		config.JMXPort = &jmxPort
	}

	return config
}

func configureGrafanaService(cr *operatorv1beta1.Cryostat) *operatorv1beta1.GrafanaServiceConfig {
	// Check CR for config
	var config *operatorv1beta1.GrafanaServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.GrafanaConfig == nil {
		config = &operatorv1beta1.GrafanaServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.GrafanaConfig
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "cryostat")

	// Apply default HTTP port if not provided
	if config.HTTPPort == nil {
		httpPort := grafanaContainerPort
		config.HTTPPort = &httpPort
	}

	return config
}

func configureReportsService(cr *operatorv1beta1.Cryostat) *operatorv1beta1.ReportsServiceConfig {
	// Check CR for config
	var config *operatorv1beta1.ReportsServiceConfig
	if cr.Spec.ServiceOptions == nil || cr.Spec.ServiceOptions.ReportsConfig == nil {
		config = &operatorv1beta1.ReportsServiceConfig{}
	} else {
		config = cr.Spec.ServiceOptions.ReportsConfig
	}

	// Apply common service defaults
	configureService(&config.ServiceConfig, cr.Name, "reports")

	// Apply default HTTP port if not provided
	if config.HTTPPort == nil {
		httpPort := reportsContainerPort
		config.HTTPPort = &httpPort
	}

	return config
}

func configureService(config *operatorv1beta1.ServiceConfig, appLabel string, componentLabel string) {
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
}

func (r *CryostatReconciler) createOrUpdateService(ctx context.Context, svc *corev1.Service, owner metav1.Object,
	config *operatorv1beta1.ServiceConfig, delegate controllerutil.MutateFn) (*corev1.Service, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		// Update labels and annotations
		svc.Labels = config.Labels
		svc.Annotations = config.Annotations
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
		return nil, err
	}
	r.Log.Info(fmt.Sprintf("Service %s", op), "name", svc.Name, "namespace", svc.Namespace)
	return svc, nil
}

func (r *CryostatReconciler) deleteService(ctx context.Context, svc *corev1.Service) (*corev1.Service, error) {
	err := r.Client.Delete(context.Background(), svc)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete service", "name", svc.Name, "namespace", svc.Namespace)
		return nil, err
	}
	r.Log.Info("Service deleted", "name", svc.Name, "namespace", svc.Namespace)
	return svc, nil
}