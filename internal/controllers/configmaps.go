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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

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
	return r.createOrUpdateConfigMap(ctx, cm, cr.Object, func() error {
		return nil
	})
}

type oauth2ProxyAlphaConfig struct {
	Server         alphaConfigServer         `json:"server,omitempty"`
	UpstreamConfig alphaConfigUpstreamConfig `json:"upstreamConfig,omitempty"`
	Providers      []alphaConfigProvider     `json:"providers,omitempty"`
}

type alphaConfigServer struct {
	BindAddress       string   `json:"BindAddress,omitempty"`
	SecureBindAddress string   `json:"SecureBindAddress,omitempty"`
	TLS               proxyTLS `json:"TLS,omitempty"`
}

type proxyTLS struct {
	Key  tlsSecretSource `json:"Key,omitempty"`
	Cert tlsSecretSource `json:"Cert,omitempty"`
}

type tlsSecretSource struct {
	FromFile string `json:"fromFile,omitempty"`
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

func (r *Reconciler) reconcileOAuth2ProxyConfig(ctx context.Context, cr *model.CryostatInstance, tls *resources.TLSConfig) error {
	bindHost := "0.0.0.0"
	immutable := true
	cfg := &oauth2ProxyAlphaConfig{
		Server: alphaConfigServer{},
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

	if tls != nil {
		cfg.Server.SecureBindAddress = fmt.Sprintf("https://%s:%d", bindHost, constants.AuthProxyHttpContainerPort)
		cfg.Server.TLS = proxyTLS{
			Key: tlsSecretSource{
				FromFile: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.CryostatSecret, corev1.TLSPrivateKeyKey),
			},
			Cert: tlsSecretSource{
				FromFile: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.CryostatSecret, corev1.TLSCertKey),
			},
		}
	} else {
		cfg.Server.BindAddress = fmt.Sprintf("http://%s:%d", bindHost, constants.AuthProxyHttpContainerPort)
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

	if r.IsOpenShift {
		return r.deleteConfigMap(ctx, cm)
	} else {
		return r.createOrUpdateConfigMap(ctx, cm, cr.Object, func() error {
			return nil
		})
	}
}

type nginxConfParams struct {
	// Hostname of the server
	ServerName string
	// Whether TLS is enabled
	TLSEnabled bool
	// Path to certificate for HTTPS
	TLSCertFile string
	// Path to private key for HTTPS
	TLSKeyFile string
	// Path to CA certificate
	CACertFile string
	// Diffie-Hellman parameters file
	DHParamFile string
	// Nginx proxy container port
	ContainerPort int32
	// Nginx health container port
	HealthPort int32
	// Cryostat HTTP container port
	CryostatPort int32
	// Only these path prefixes will be proxied, others will return 404
	AllowedPathPrefixes []string
}

// Reference: https://ssl-config.mozilla.org
var nginxConfTemplate = template.Must(template.New("").Parse(`worker_processes auto;
error_log stderr notice;
pid /run/nginx.pid;

# Load dynamic modules. See /usr/share/doc/nginx/README.dynamic.
include /usr/share/nginx/modules/*.conf;

events {
	worker_connections 1024;
}

http {
	log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
	                  '$status $body_bytes_sent "$http_referer" '
	                  '"$http_user_agent" "$http_x_forwarded_for"';

	access_log  /dev/stdout  main;

	sendfile            on;
	tcp_nopush          on;
	keepalive_timeout   65;
	types_hash_max_size 4096;

	include             /etc/nginx/mime.types;
	default_type        application/octet-stream;

	server {
		server_name {{ .ServerName }};

		{{ if .TLSEnabled -}}
		listen {{ .ContainerPort }} ssl;
		listen [::]:{{ .ContainerPort }} ssl;

		ssl_certificate {{ .TLSCertFile }};
		ssl_certificate_key {{ .TLSKeyFile }};

		ssl_session_timeout 5m;
		ssl_session_cache shared:SSL:20m;
		ssl_session_tickets off;

		ssl_dhparam {{ .DHParamFile }};

		# intermediate configuration
		ssl_protocols TLSv1.2 TLSv1.3;
		ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384:DHE-RSA-CHACHA20-POLY1305;
		ssl_prefer_server_ciphers off;

		# HSTS (ngx_http_headers_module is required) (63072000 seconds)
		add_header Strict-Transport-Security "max-age=63072000" always;

		# OCSP stapling
		ssl_stapling on;
		ssl_stapling_verify on;

		ssl_trusted_certificate {{ .CACertFile }};

		# Client certificate authentication
		ssl_client_certificate {{ .CACertFile }};
		ssl_verify_client on;

		{{- else -}}

		listen {{ .ContainerPort }};
		listen [::]:{{ .ContainerPort }};

		{{- end }}

		{{ range .AllowedPathPrefixes -}}
		location {{ . }} {
			proxy_pass http://127.0.0.1:{{ $.CryostatPort }}$request_uri;
		}

		{{ end -}}

		location / {
			return 404;
		}
	}

	# Heatlh Check
	server {
		listen {{ .HealthPort }};
		listen [::]:{{ .HealthPort }};

		location = /healthz {
			return 200;
		}

		location / {
			return 404;
		}
	}
}`))

const (
	dhFileName = "dhparam.pem"
	// From https://ssl-config.mozilla.org/ffdhe2048.txt
	dhParams = `-----BEGIN DH PARAMETERS-----
MIIBCAKCAQEA//////////+t+FRYortKmq/cViAnPTzx2LnFg84tNpWp4TZBFGQz
+8yTnc4kmz75fS/jY2MMddj2gbICrsRhetPfHtXV/WVhJDP1H18GbtCFY2VVPe0a
87VXE15/V8k1mE8McODmi3fipona8+/och3xWKE2rec1MKzKT0g6eXq8CrGCsyT7
YdEIqUuyyOP7uWrat2DX9GgdT0Kj3jlN9K5W7edjcrsZCwenyO4KbXCeAvzhzffi
7MA0BM0oNC9hkXL+nOmFg/+OTxIy7vKBg8P+OxtMb61zO7X8vC7CIAXFjvGDfRaD
ssbzSibBsu/6iGtCOGEoXJf//////////wIBAg==
-----END DH PARAMETERS-----`
)

func (r *Reconciler) reconcileAgentProxyConfig(ctx context.Context, cr *model.CryostatInstance, tls *resources.TLSConfig) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-agent-proxy",
			Namespace: cr.InstallNamespace,
		},
	}

	data := map[string]string{}
	buf := &bytes.Buffer{}
	params := &nginxConfParams{
		ServerName:    fmt.Sprintf("%s-agent.%s.svc", cr.Name, cr.InstallNamespace),
		ContainerPort: constants.AgentProxyContainerPort,
		HealthPort:    constants.AgentProxyHealthPort,
		CryostatPort:  constants.CryostatHTTPContainerPort,
		AllowedPathPrefixes: []string{
			"/api/v2.2/discovery/",
			"/api/v2.2/credentials/",
			"/api/beta/recordings/",
			"/health/",
		},
	}
	if tls != nil {
		params.TLSEnabled = true
		params.TLSCertFile = fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.AgentProxySecret, corev1.TLSCertKey)
		params.TLSKeyFile = fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.AgentProxySecret, corev1.TLSPrivateKeyKey)
		params.CACertFile = fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s/%s", tls.AgentProxySecret, constants.CAKey)
		params.DHParamFile = fmt.Sprintf("%s/%s", constants.AgentProxyConfigFilePath, dhFileName)

		// Add Diffie-Hellman parameters to config map
		data[dhFileName] = dhParams
	}

	// Create an nginx.conf where:
	// 1. If TLS is enabled, requires client certificate authentication against our CA
	// 2. Proxies only those API endpoints required by the agent
	err := nginxConfTemplate.Execute(buf, params)
	if err != nil {
		return err
	}

	// Add generated nginx.conf to config map
	data[constants.AgentProxyConfigFileName] = buf.String()

	return r.createOrUpdateConfigMap(ctx, cm, cr.Object, func() error {
		cm.Data = data
		return nil
	})
}

func (r *Reconciler) createOrUpdateConfigMap(ctx context.Context, cm *corev1.ConfigMap, owner metav1.Object,
	delegate controllerutil.MutateFn) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, cm, r.Scheme); err != nil {
			return err
		}
		return delegate()
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
