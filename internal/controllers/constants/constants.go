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

package constants

import (
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
)

const (
	CryostatHTTPContainerPort int32  = 8181
	CryostatJMXContainerPort  int32  = 9091
	GrafanaContainerPort      int32  = 3000
	DatasourceContainerPort   int32  = 8080
	ReportsContainerPort      int32  = 10000
	LoopbackAddress           string = "127.0.0.1"
	OperatorNamePrefix        string = "cryostat-operator-"
	HttpPortName              string = "http"
	// CAKey is the key for a CA certificate within a TLS secret
	CAKey = certMeta.TLSCAKey
	// Hostname alias for loopback address, to be used for health checks
	HealthCheckHostname = "cryostat-health.local"
)
