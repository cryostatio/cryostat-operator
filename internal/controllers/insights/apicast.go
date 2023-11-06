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

package insights

import (
	"bytes"
	"text/template"
)

type apiCastConfigParams struct {
	FrontendDomains       string
	BackendInsightsDomain string
	HeaderValue           string
	ProxyDomain           string
}

var apiCastConfigTemplate = template.Must(template.New("").Parse(`{
  "services": [
    {
      "id": "1",
      "backend_version": "1",
      "proxy": {
        "hosts": [{{ .FrontendDomains }}],
        "api_backend": "https://{{ .BackendInsightsDomain }}:443/",
        "backend": { "endpoint": "http://127.0.0.1:8081", "host": "backend" },
        "policy_chain": [
          {
            "name": "default_credentials",
            "version": "builtin",
            "configuration": {
              "auth_type": "user_key",
              "user_key": "dummy_key"
            }
          },
          {{- if .ProxyDomain }}
          {
            "name": "apicast.policy.http_proxy",
            "configuration": {
              "https_proxy": "http://{{ .ProxyDomain }}/",
              "http_proxy": "http://{{ .ProxyDomain }}/"
            }
          },
          {{- end }}
          {
            "name": "headers",
            "version": "builtin",
            "configuration": {
              "request": [
                {
                  "op": "set",
                  "header": "Authorization",
                  "value_type": "plain",
                  "value": "{{ .HeaderValue }}"
                }
              ]
            }
          },
          {
            "name": "apicast.policy.apicast"
          }
        ],
        "proxy_rules": [
          {
            "http_method": "POST",
            "pattern": "/",
            "metric_system_name": "hits",
            "delta": 1,
            "parameters": [],
            "querystring_parameters": {}
          }
        ]
      }
    }
  ]
}`))

func getAPICastConfig(params *apiCastConfigParams) (*string, error) {
	buf := &bytes.Buffer{}
	err := apiCastConfigTemplate.Execute(buf, params)
	if err != nil {
		return nil, err
	}
	result := buf.String()
	return &result, nil
}
