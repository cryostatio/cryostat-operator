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
