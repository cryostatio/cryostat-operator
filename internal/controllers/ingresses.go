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
	coreConfig := configureCoreIngress(cr)
	url, err := r.reconcileIngress(ctx, ingress, cr, coreConfig)
	if err != nil {
		return err
	}
	specs.AuthProxyURL = url
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

	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.GrafanaConfig == nil ||
		cr.Spec.NetworkOptions.GrafanaConfig.IngressSpec == nil {
		// User has not requested an Ingress, delete if it exists
		return r.deleteIngress(ctx, ingress)
	}
	grafanaConfig := configureGrafanaIngress(cr)
	url, err := r.reconcileIngress(ctx, ingress, cr, grafanaConfig)
	if err != nil {
		return err
	}
	specs.GrafanaURL = url
	return nil
}

func (r *Reconciler) reconcileIngress(ctx context.Context, ingress *netv1.Ingress, cr *model.CryostatInstance,
	config *operatorv1beta2.NetworkConfiguration) (*url.URL, error) {
	ingress, err := r.createOrUpdateIngress(ctx, ingress, cr.Object, config)
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
	config *operatorv1beta2.NetworkConfiguration) (*netv1.Ingress, error) {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		// Set labels and annotations from CR
		common.MergeLabelsAndAnnotations(&ingress.ObjectMeta, config.Labels, config.Annotations)

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

func configureCoreIngress(cr *model.CryostatInstance) *operatorv1beta2.NetworkConfiguration {
	var config *operatorv1beta2.NetworkConfiguration
	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.CoreConfig == nil {
		config = &operatorv1beta2.NetworkConfiguration{}
	} else {
		config = cr.Spec.NetworkOptions.CoreConfig
	}

	configureIngress(config, cr.Name, "cryostat")
	return config
}

func configureGrafanaIngress(cr *model.CryostatInstance) *operatorv1beta2.NetworkConfiguration {
	var config *operatorv1beta2.NetworkConfiguration
	if cr.Spec.NetworkOptions == nil || cr.Spec.NetworkOptions.GrafanaConfig == nil {
		config = &operatorv1beta2.NetworkConfiguration{}
	} else {
		config = cr.Spec.NetworkOptions.GrafanaConfig
	}

	configureIngress(config, cr.Name, "cryostat")
	return config
}

func configureIngress(config *operatorv1beta2.NetworkConfiguration, appLabel string, componentLabel string) {
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

func (r *Reconciler) deleteIngress(ctx context.Context, ingress *netv1.Ingress) error {
	err := r.Client.Delete(ctx, ingress)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete ingress", "name", ingress.Name, "namespace", ingress.Namespace)
		return err
	}
	r.Log.Info("Ingress deleted", "name", ingress.Name, "namespace", ingress.Namespace)
	return nil
}
