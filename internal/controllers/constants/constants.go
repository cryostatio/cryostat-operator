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
	corev1 "k8s.io/api/core/v1"
)

// Generates constants from environment variables at build time
//go:generate go run ../../tools/const_generator.go

const (
	AuthProxyHttpContainerPort int32  = 4180
	CryostatHTTPContainerPort  int32  = 8181
	GrafanaContainerPort       int32  = 3000
	DatasourceContainerPort    int32  = 8989
	ReportsContainerPort       int32  = 10000
	StoragePort                int32  = 8333
	DatabasePort               int32  = 5432
	AgentProxyContainerPort    int32  = 8282
	AgentProxyHealthPort       int32  = 8281
	AgentCallbackContainerPort int32  = 9977
	AgentCallbackPortName      string = "cryostat-cb" // Max 15 characters
	LoopbackAddress            string = "127.0.0.1"
	OperatorNamePrefix         string = "cryostat-operator-"
	OperatorDeploymentName     string = "cryostat-operator-controller"
	HttpPortName               string = "http"
	HttpsPortName              string = "https"
	// CAKey is the key for a CA certificate within a TLS secret
	CAKey = certMeta.TLSCAKey
	// ALL capability to drop for restricted pod security. See:
	// https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
	CapabilityAll corev1.Capability = "ALL"

	// DatabaseSecretConnectionKey indexes the database connection password within the Cryostat database Secret
	DatabaseSecretConnectionKey = "CONNECTION_KEY"
	// DatabaseSecretEncryptionKey indexes the database encryption key within the Cryostat database Secret
	DatabaseSecretEncryptionKey = "ENCRYPTION_KEY"
	// KeystorePassSecretKey indexes the keystore password within the Cryostat keystore Secret
	KeystorePassSecretKey = "keystore.pass"

	AgentProxyConfigFilePath string = "/etc/nginx-cryostat"
	AgentProxyConfigFileName string = "nginx.conf"

	// Labels applied by operator to track cross-namespace ownership
	targetNamespaceCRLabelPrefix    = "operator.cryostat.io/"
	TargetNamespaceCRNameLabel      = targetNamespaceCRLabelPrefix + "name"
	TargetNamespaceCRNamespaceLabel = targetNamespaceCRLabelPrefix + "namespace"

	// Labels for agent auto-configuration
	agentLabelPrefix               = "cryostat.io/"
	AgentLabelCryostatName         = agentLabelPrefix + "name"
	AgentLabelCryostatNamespace    = agentLabelPrefix + "namespace"
	AgentLabelLogLevel             = agentLabelPrefix + "log-level"
	AgentLabelCallbackPort         = agentLabelPrefix + "callback-port"
	AgentLabelContainer            = agentLabelPrefix + "container"
	AgentLabelReadOnly             = agentLabelPrefix + "read-only"
	AgentLabelJavaOptionsVar       = agentLabelPrefix + "java-options-var"
	AgentLabelHarvesterTemplate    = agentLabelPrefix + "harvester-template"
	AgentLabelHarvesterExitMaxAge  = agentLabelPrefix + "harvester-exit-max-age"
	AgentLabelHarvesterExitMaxSize = agentLabelPrefix + "harvester-exit-max-size"

	CryostatCATLSCommonName     = "cryostat-ca-cert-manager"
	CryostatTLSCommonName       = "cryostat"
	DatabaseTLSCommonName       = "cryostat-db"
	StorageTLSCommonName        = "cryostat-storage"
	ReportsTLSCommonName        = "cryostat-reports"
	AgentsTLSCommonName         = "cryostat-agent"
	AgentAuthProxyTLSCommonName = "cryostat-agent-proxy"

	// OpenShift Console Plugin constants
	ConsolePluginName               = "cryostat-plugin"
	ConsoleServiceAccountName       = "cryostat-plugin"
	ConsoleServiceName              = "cryostat-plugin"
	ConsoleServicePort        int32 = 9443
	ConsoleProxyName                = "cryostat-plugin-proxy"
	ConsoleCRName                   = "cluster"
	ClusterVersionName              = "version"
)
