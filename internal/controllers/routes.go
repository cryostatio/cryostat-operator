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
	goerrors "errors"
	"fmt"
	"net/url"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	common "github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newCoreRoute(cr *model.CryostatInstance) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
	}
}

func (r *Reconciler) reconcileCoreRoute(ctx context.Context, svc *corev1.Service, cr *model.CryostatInstance,
	tls *resource_definitions.TLSConfig, specs *resource_definitions.ServiceSpecs) error {
	route := newCoreRoute(cr)
	coreConfig := configureCoreRoute(cr)
	url, err := r.reconcileRoute(ctx, route, svc, cr, tls, coreConfig)
	if err != nil {
		return err
	}
	specs.AuthProxyURL = url
	specs.CoreURL = url
	return nil
}

// ErrIngressNotReady is returned when Kubernetes has not yet exposed our services
// so that they may be accessed outside of the cluster
var ErrIngressNotReady = goerrors.New("ingress configuration not yet available")

func (r *Reconciler) reconcileRoute(ctx context.Context, route *routev1.Route, svc *corev1.Service,
	cr *model.CryostatInstance, tls *resource_definitions.TLSConfig, config *operatorv1beta2.NetworkConfiguration) (*url.URL, error) {
	port, err := getHTTPPort(svc)
	if err != nil {
		return nil, err
	}
	route, err = r.createOrUpdateRoute(ctx, route, cr.Object, svc, port, tls, config)
	if err != nil {
		return nil, err
	}

	if len(route.Status.Ingress) < 1 {
		r.Log.Info("Waiting for route to become available", "name", route.Name, "namespace", route.Namespace)
		return nil, ErrIngressNotReady
	}

	return &url.URL{
		Scheme: getProtocol(route),
		Host:   route.Status.Ingress[0].Host,
	}, nil
}

func (r *Reconciler) createOrUpdateRoute(ctx context.Context, route *routev1.Route, owner metav1.Object,
	svc *corev1.Service, exposePort *corev1.ServicePort, tlsConfig *resource_definitions.TLSConfig, config *operatorv1beta2.NetworkConfiguration) (*routev1.Route, error) {
	// Use edge termination by default
	var routeTLS *routev1.TLSConfig
	if tlsConfig == nil {
		routeTLS = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		}
	} else {
		routeTLS = &routev1.TLSConfig{
			Termination:              routev1.TLSTerminationReencrypt,
			DestinationCACertificate: string(tlsConfig.CACert),
		}
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		// Set labels and annotations from CR
		common.MergeLabelsAndAnnotations(&route.ObjectMeta, config.Labels, config.Annotations)

		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, route, r.Scheme); err != nil {
			return err
		}
		// Update Route spec
		route.Spec.To.Kind = "Service"
		route.Spec.To.Name = svc.Name
		route.Spec.Port = &routev1.RoutePort{TargetPort: exposePort.TargetPort}
		route.Spec.TLS = routeTLS
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.Log.Info(fmt.Sprintf("Route %s", op), "name", route.Name, "namespace", route.Namespace)
	return route, nil
}

func getProtocol(route *routev1.Route) string {
	if route.Spec.TLS == nil {
		return "http"
	}
	return "https"
}

func getHTTPPort(svc *corev1.Service) (*corev1.ServicePort, error) {
	for _, port := range svc.Spec.Ports {
		if port.Name == constants.HttpPortName {
			return &port, nil
		}
	}
	// Shouldn't happen
	return nil, fmt.Errorf("no \"%s\"port in %s service in %s namespace", constants.HttpPortName, svc.Name, svc.Namespace)
}

func configureCoreRoute(cr *model.CryostatInstance) *operatorv1beta2.NetworkConfiguration {
	var config *operatorv1beta2.NetworkConfiguration
	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.CoreConfig == nil {
		config = &operatorv1beta2.NetworkConfiguration{}
	} else {
		config = cr.Spec.NetworkOptions.CoreConfig
	}

	configureRoute(config, cr.Name, "cryostat")
	return config
}

func configureRoute(config *operatorv1beta2.NetworkConfiguration, appLabel string, componentLabel string) {
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
