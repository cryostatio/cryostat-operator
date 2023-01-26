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

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileCoreIngress(ctx context.Context, cr *model.CryostatInstance,
	specs *resource_definitions.ServiceSpecs) error {
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
	}

	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.CoreConfig == nil ||
		cr.Spec.NetworkOptions.CoreConfig.IngressSpec == nil {
		// User has not requested an Ingress, delete if it exists
		return r.deleteIngress(ctx, ingress)
	}

	url, err := r.reconcileIngress(ctx, ingress, cr, cr.Spec.NetworkOptions.CoreConfig)
	if err != nil {
		return err
	}
	specs.CoreURL = url
	return nil
}

func (r *Reconciler) reconcileGrafanaIngress(ctx context.Context, cr *model.CryostatInstance,
	specs *resource_definitions.ServiceSpecs) error {
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.InstallNamespace,
		},
	}

	if cr.Spec.Minimal || cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.GrafanaConfig == nil ||
		cr.Spec.NetworkOptions.GrafanaConfig.IngressSpec == nil {
		// User has either chosen a minimal deployment or not requested
		// an Ingress, delete if it exists
		return r.deleteIngress(ctx, ingress)
	}
	url, err := r.reconcileIngress(ctx, ingress, cr, cr.Spec.NetworkOptions.GrafanaConfig)
	if err != nil {
		return err
	}
	specs.GrafanaURL = url
	return nil
}

func (r *Reconciler) reconcileIngress(ctx context.Context, ingress *netv1.Ingress, cr *model.CryostatInstance,
	config *operatorv1beta1.NetworkConfiguration) (*url.URL, error) {
	ingress, err := r.createOrUpdateIngress(ctx, ingress, cr.Instance, config)
	if err != nil {
		return nil, err
	}

	if ingress.Spec.Rules == nil || len(ingress.Spec.Rules[0].Host) == 0 {
		return nil, nil
	}

	host := ingress.Spec.Rules[0].Host
	scheme := "http"
	if ingress.Spec.TLS != nil {
		scheme = "https"
	}
	return &url.URL{
		Scheme: scheme,
		Host:   host,
	}, nil
}

func (r *Reconciler) createOrUpdateIngress(ctx context.Context, ingress *netv1.Ingress, owner metav1.Object,
	config *operatorv1beta1.NetworkConfiguration) (*netv1.Ingress, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		// Set labels and annotations from CR
		ingress.Labels = config.Labels
		ingress.Annotations = config.Annotations
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, ingress, r.Scheme); err != nil {
			return err
		}
		// Update Ingress spec
		ingress.Spec = *config.IngressSpec
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.Log.Info(fmt.Sprintf("Ingress %s", op), "name", ingress.Name, "namespace", ingress.Namespace)
	return ingress, nil
}

func (r *Reconciler) deleteIngress(ctx context.Context, ingress *netv1.Ingress) error {
	err := r.Client.Delete(ctx, ingress)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete ingress", "name", ingress.Name, "namespace", ingress.Namespace)
		return err
	}
	r.Log.Info("Ingress deleted", "name", ingress.Name, "namespace", ingress.Namespace)
	return nil
}
