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

type OAuth2ProxyAlphaConfig struct {
	Server         AlphaConfigServer         `json:"server,omitempty"`
	UpstreamConfig AlphaConfigUpstreamConfig `json:"upstreamConfig,omitempty"`
	Providers      []AlphaConfigProvider     `json:"providers,omitempty"`
}

type AlphaConfigServer struct {
	BindAddress string `json:"BindAddress,omitempty"`
}

type AlphaConfigUpstreamConfig struct {
	ProxyRawPath bool                  `json:"proxyRawPath,omitempty"`
	Upstreams    []AlphaConfigUpstream `json:"upstreams,omitempty"`
}

type AlphaConfigProvider struct {
	Id           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	ClientId     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

type AlphaConfigUpstream struct {
	Id              string `json:"id,omitempty"`
	Path            string `json:"path,omitempty"`
	RewriteTarget   string `json:"rewriteTarget,omitempty"`
	Uri             string `json:"uri,omitempty"`
	PassHostHeader  bool   `json:"passHostHeader,omitempty"`
	ProxyWebSockets bool   `json:"proxyWebSockets,omitempty"`
}

func (r *Reconciler) reconcileOauth2ProxyConfig(ctx context.Context, cr *model.CryostatInstance) error {
	if resources.DeployOpenShiftOAuth(cr, r.IsOpenShift) {
		// this is only needed when deploying oauth2_proxy, not when deploying openshift-oauth-proxy
		// TODO detect the case where we previously deployed oauth2_proxy and are switching to
		// openshift-oauth-proxy, so we need to delete the old config map
		return nil
	}
	immutable := true

	cfg := &OAuth2ProxyAlphaConfig{
		Server: AlphaConfigServer{BindAddress: fmt.Sprintf("http://0.0.0.0:%d", constants.AuthProxyHttpContainerPort)},
		UpstreamConfig: AlphaConfigUpstreamConfig{ProxyRawPath: true, Upstreams: []AlphaConfigUpstream{
			{
				Id:   "cryostat",
				Path: "/",
				Uri:  "http://localhost:8181",
			},
			{
				Id:   "grafana",
				Path: "/grafana/",
				Uri:  "http://localhost:3000",
			},
			{
				Id:              "storage",
				Path:            "^/storage/(.*)$",
				RewriteTarget:   "/$1",
				Uri:             "http://localhost:8333",
				PassHostHeader:  false,
				ProxyWebSockets: false,
			},
		}},
		Providers: []AlphaConfigProvider{{Id: "dummy", Name: "Unused - Sign In Below", ClientId: "CLIENT_ID", ClientSecret: "CLIENT_SECRET", Provider: "google"}},
	}

	data := make(map[string]string)
	json, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	data[resources.Oauth2ConfigFileName(cr)] = string(json)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-oauth2-proxy-cfg",
			Namespace: cr.InstallNamespace,
		},
		Immutable: &immutable,
		Data:      data,
	}
	return r.createOrUpdateConfigMap(ctx, cm, cr.Object)
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
