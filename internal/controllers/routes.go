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
	goerrors "errors"
	"fmt"
	"net/url"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	specs.CoreURL = url
	return nil
}

func (r *Reconciler) reconcileGrafanaRoute(ctx context.Context, svc *corev1.Service, cr *model.CryostatInstance,
	tls *resource_definitions.TLSConfig, specs *resource_definitions.ServiceSpecs) error {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.InstallNamespace,
		},
	}
	if cr.Spec.Minimal {
		// Delete route if it exists
		return r.deleteRoute(ctx, route)
	}
	grafanaConfig := configureGrafanaRoute(cr)
	url, err := r.reconcileRoute(ctx, route, svc, cr, tls, grafanaConfig)
	if err != nil {
		return err
	}
	specs.GrafanaURL = url
	return nil
}

// ErrIngressNotReady is returned when Kubernetes has not yet exposed our services
// so that they may be accessed outside of the cluster
var ErrIngressNotReady = goerrors.New("ingress configuration not yet available")

func (r *Reconciler) reconcileRoute(ctx context.Context, route *routev1.Route, svc *corev1.Service,
	cr *model.CryostatInstance, tls *resource_definitions.TLSConfig, config *operatorv1beta1.NetworkConfiguration) (*url.URL, error) {
	port, err := getHTTPPort(svc)
	if err != nil {
		return nil, err
	}
	route, err = r.createOrUpdateRoute(ctx, route, cr.Instance, svc, port, tls, config)
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
	svc *corev1.Service, exposePort *corev1.ServicePort, tlsConfig *resource_definitions.TLSConfig, config *operatorv1beta1.NetworkConfiguration) (*routev1.Route, error) {
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
		route.Labels = config.Labels
		route.Annotations = config.Annotations
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

func (r *Reconciler) deleteRoute(ctx context.Context, route *routev1.Route) error {
	err := r.Client.Delete(ctx, route)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete route", "name", route.Name, "namespace", route.Namespace)
		return err
	}
	r.Log.Info("Route deleted", "name", route.Name, "namespace", route.Namespace)
	return nil
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

func configureCoreRoute(cr *model.CryostatInstance) *operatorv1beta1.NetworkConfiguration {
	var config *operatorv1beta1.NetworkConfiguration
	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.CoreConfig == nil {
		config = &operatorv1beta1.NetworkConfiguration{}
	} else {
		config = cr.Spec.NetworkOptions.CoreConfig
	}

	configureRoute(config)
	return config
}

func configureGrafanaRoute(cr *model.CryostatInstance) *operatorv1beta1.NetworkConfiguration {
	var config *operatorv1beta1.NetworkConfiguration
	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.GrafanaConfig == nil {
		config = &operatorv1beta1.NetworkConfiguration{}
	} else {
		config = cr.Spec.NetworkOptions.GrafanaConfig
	}

	configureRoute(config)
	return config
}

func configureRoute(config *operatorv1beta1.NetworkConfiguration) {
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	if config.Annotations == nil {
		config.Annotations = map[string]string{}
	}
}
