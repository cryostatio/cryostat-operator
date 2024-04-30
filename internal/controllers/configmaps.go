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
	"encoding/json"
	"fmt"

	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileLockConfigMap(ctx context.Context, cr *model.CryostatInstance) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-lock",
			Namespace: cr.InstallNamespace,
		},
	}
	return r.createOrUpdateConfigMap(ctx, cm, cr.Object)
}

type oauth2ProxyAlphaConfig struct {
	Server         alphaConfigServer         `json:"server,omitempty"`
	UpstreamConfig alphaConfigUpstreamConfig `json:"upstreamConfig,omitempty"`
	Providers      []alphaConfigProvider     `json:"providers,omitempty"`
}

type alphaConfigServer struct {
	BindAddress string `json:"BindAddress,omitempty"`
}

type alphaConfigUpstreamConfig struct {
	ProxyRawPath bool                  `json:"proxyRawPath,omitempty"`
	Upstreams    []alphaConfigUpstream `json:"upstreams,omitempty"`
}

type alphaConfigProvider struct {
	Id           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	ClientId     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

type alphaConfigUpstream struct {
	Id              string `json:"id,omitempty"`
	Path            string `json:"path,omitempty"`
	RewriteTarget   string `json:"rewriteTarget,omitempty"`
	Uri             string `json:"uri,omitempty"`
	PassHostHeader  bool   `json:"passHostHeader,omitempty"`
	ProxyWebSockets bool   `json:"proxyWebSockets,omitempty"`
}

func (r *Reconciler) reconcileOAuth2ProxyConfig(ctx context.Context, cr *model.CryostatInstance) error {
	immutable := true
	cfg := &oauth2ProxyAlphaConfig{
		Server: alphaConfigServer{BindAddress: fmt.Sprintf("http://0.0.0.0:%d", constants.AuthProxyHttpContainerPort)},
		UpstreamConfig: alphaConfigUpstreamConfig{ProxyRawPath: true, Upstreams: []alphaConfigUpstream{
			{
				Id:   "cryostat",
				Path: "/",
				Uri:  fmt.Sprintf("http://localhost:%d", constants.CryostatHTTPContainerPort),
			},
			{
				Id:   "grafana",
				Path: "/grafana/",
				Uri:  fmt.Sprintf("http://localhost:%d", constants.GrafanaContainerPort),
			},
			{
				Id:              "storage",
				Path:            "^/storage/(.*)$",
				RewriteTarget:   "/$1",
				Uri:             fmt.Sprintf("http://localhost:%d", constants.StoragePort),
				PassHostHeader:  false,
				ProxyWebSockets: false,
			},
		}},
		Providers: []alphaConfigProvider{{Id: "dummy", Name: "Unused - Sign In Below", ClientId: "CLIENT_ID", ClientSecret: "CLIENT_SECRET", Provider: "google"}},
	}

	data := make(map[string]string)
	json, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	data[resources.OAuth2ConfigFileName] = string(json)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-oauth2-proxy-cfg",
			Namespace: cr.InstallNamespace,
		},
		Immutable: &immutable,
		Data:      data,
	}

	if resources.DeployOpenShiftOAuth(cr, r.IsOpenShift) {
		return r.deleteConfigMap(ctx, cm)
	} else {
		return r.createOrUpdateConfigMap(ctx, cm, cr.Object)
	}
}

func (r *Reconciler) createOrUpdateConfigMap(ctx context.Context, cm *corev1.ConfigMap, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, cm, r.Scheme); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Config Map %s", op), "name", cm.Name, "namespace", cm.Namespace)
	return nil
}

func (r *Reconciler) deleteConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	err := r.Client.Delete(ctx, cm)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete ConfigMap", "name", cm.Name, "namespace", cm.Namespace)
		return err
	}
	r.Log.Info("ConfigMap deleted", "name", cm.Name, "namespace", cm.Namespace)
	return nil
}
