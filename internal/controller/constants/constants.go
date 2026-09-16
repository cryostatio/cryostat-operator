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
	"strings"

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
	AuthStripProxyPort         int32  = 8180
	AgentCallbackContainerPort int32  = 9977
	AgentCallbackPortName      string = "cryostat-cb" // Max 15 characters
	LoopbackAddress            string = "127.0.0.1"
	OperatorNamePrefix         string = "cryostat-operator-"
	OperatorDeploymentName     string = "cryostat-operator-controller"
	HttpScheme                 string = "http"
	HttpPortName               string = HttpScheme
	HttpsScheme                string = "https"
	HttpsPortName              string = HttpsScheme
	LabelAppName               string = "cryostat"
	// CAKey is the key for a CA certificate within a TLS secret
	CAKey = certMeta.TLSCAKey
	// ALL capability to drop for restricted pod security. See:
	// https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
	CapabilityAll corev1.Capability = "ALL"

	// DatabaseSecretConnectionKey indexes the database connection password within the Cryostat database Secret
	DatabaseSecretConnectionKey = "CONNECTION_KEY"
	// DatabaseSecretEncryptionKey indexes the database encryption key within the Cryostat database Secret
	DatabaseSecretEncryptionKey = "ENCRYPTION_KEY"
	// KeyStoreFile indexes the keystore file within the Cryostat keystore Secret
	KeyStoreFile = "keystore.p12"
	// KeystorePassSecretKey indexes the keystore password within the Cryostat keystore Secret
	KeystorePassSecretKey = "KEYSTORE_PASS"
	// KeystorePassFile is the name of the file to mount the keystore password
	KeystorePassFile = "keystore.pass"

	AgentProxyConfigFilePath string = "/etc/nginx-cryostat"
	AgentProxyConfigFileName string = "nginx.conf"
	AuthStripProxyConfigPath string = "/etc/nginx-auth-strip"
	AuthStripProxyConfigFile string = "nginx.conf"

	// AgentGatewayAuthHeader carries the shared secret proving that a request was
	// forwarded by the Cryostat Agent gateway.
	AgentGatewayAuthHeader string = "X-Cryostat-Agent-Auth"
	// AgentGatewaySecretNameSuffix is appended to the name of a Cryostat CR to name
	// the Secret holding the agent gateway's provenance stamp
	AgentGatewaySecretNameSuffix string = "-agent-gateway"
	// AgentGatewaySecretKey indexes the raw shared secret within the agent gateway Secret
	AgentGatewaySecretKey string = "AGENT_GATEWAY_SECRET"
	// AgentGatewayConfFileName is the nginx include rendered from the agent gateway secret
	AgentGatewayConfFileName string = "agent-auth.conf"
	// AgentGatewaySecretMountPath is where the agent gateway Secret is mounted into the
	// agent proxy container
	AgentGatewaySecretMountPath string = "/var/run/secrets/operator.cryostat.io/agent-gateway"
	// AgentGatewaySecretVolumeName names the volume holding the agent gateway Secret
	AgentGatewaySecretVolumeName string = "agent-gateway-secret"

	// UserProxyAuthHeader carries the shared secret proving that a request was
	// forwarded by the user-path auth-strip proxy.
	UserProxyAuthHeader string = "X-Cryostat-User-Proxy-Auth"
	// UserProxySecretNameSuffix is appended to the name of a Cryostat CR to name
	// the Secret holding the auth-strip proxy's provenance stamp
	UserProxySecretNameSuffix string = "-user-proxy"
	// UserProxySecretKey indexes the raw shared secret within the user proxy Secret
	UserProxySecretKey string = "USER_PROXY_SECRET"
	// UserProxyConfFileName is the nginx include rendered from the user proxy secret
	UserProxyConfFileName string = "user-auth.conf"
	// UserProxySecretMountPath is where the user proxy Secret is mounted into the
	// auth-strip proxy container
	UserProxySecretMountPath string = "/var/run/secrets/operator.cryostat.io/user-proxy"
	// UserProxySecretVolumeName names the volume holding the user proxy Secret
	UserProxySecretVolumeName string = "user-proxy-secret"

	AgentEmptyDirBasePath = "/tmp/cryostat-agent"
	AgentJarPath          = AgentEmptyDirBasePath + "/cryostat-agent-shaded.jar"

	// Labels applied by operator to track cross-namespace ownership
	targetNamespaceCRLabelPrefix    = "operator.cryostat.io/"
	TargetNamespaceCRNameLabel      = targetNamespaceCRLabelPrefix + "name"
	TargetNamespaceCRNamespaceLabel = targetNamespaceCRLabelPrefix + "namespace"

	// Labels for agent auto-configuration
	AgentLabelPrefix                  = "cryostat.io/"
	AgentLabelCryostatName            = AgentLabelPrefix + "name"
	AgentLabelCryostatNamespace       = AgentLabelPrefix + "namespace"
	AgentLabelLogLevel                = AgentLabelPrefix + "log-level"
	AgentLabelCallbackPort            = AgentLabelPrefix + "callback-port"
	AgentLabelContainer               = AgentLabelPrefix + "container"
	AgentLabelReadOnly                = AgentLabelPrefix + "read-only"
	AgentLabelJavaOptionsVar          = AgentLabelPrefix + "java-options-var"
	AgentLabelHarvesterTemplate       = AgentLabelPrefix + "harvester-template"
	AgentLabelHarvesterPeriod         = AgentLabelPrefix + "harvester-period"
	AgentLabelHarvesterMaxFiles       = AgentLabelPrefix + "harvester-max-files"
	AgentLabelHarvesterExitMaxAge     = AgentLabelPrefix + "harvester-exit-max-age"
	AgentLabelHarvesterExitMaxSize    = AgentLabelPrefix + "harvester-exit-max-size"
	AgentLabelSmartTriggersConfigMaps = AgentLabelPrefix + "smart-triggers"

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

// ProvenanceHeaders lists every header whose *value* is a trust decision: each one is a
// shared secret stamped by exactly one in-pod proxy hop and blanked by every other, so
// that neither hop can mint the other's principal.
//
// This slice is the single source of truth for both the stamp lists and the strip lists
// rendered into the agent gateway's nginx.conf and the auth-strip proxy's nginx.conf.
// The two are halves of one decision and must not be allowed to drift: adding a header
// here strips it at every hop that does not stamp it, by construction.
var ProvenanceHeaders = []string{
	AgentGatewayAuthHeader,
	UserProxyAuthHeader,
}

// OAuthProxyIdentityHeaders lists the headers by which the OAuth proxies assert *who the
// caller is*. Unlike the X-Forwarded-For/Host/Port/Proto family, which describes the
// connection and is parsed by Quarkus, every header here is an identity claim.
//
// Only the auth-strip proxy may assert them, and it re-sources each one from its own
// $http_* so that a client cannot supply its own. The agent gateway blanks all of them:
// an agent has no user identity to assert, and a forwarded claim would at best confuse an
// audit record and at worst be read as identity by a future endpoint.
//
// Both hops render from this one slice so the two lists cannot drift apart. That drift is
// what this list exists to prevent: the gateway previously blanked only the first two,
// leaving the rest to pass through from the agent untouched.
var OAuthProxyIdentityHeaders = []string{
	"X-Forwarded-User",
	"X-Forwarded-Access-Token",
	"X-Forwarded-Email",
	"X-Forwarded-Preferred-Username",
	"X-Forwarded-Groups",
}

// NginxHTTPVariable returns the nginx $http_* variable holding the client-supplied value of
// the given header, e.g. "X-Forwarded-Preferred-Username" -> "$http_x_forwarded_preferred_username".
// nginx derives these by lowercasing the header name and replacing dashes with underscores.
func NginxHTTPVariable(header string) string {
	return "$http_" + strings.ReplaceAll(strings.ToLower(header), "-", "_")
}

// ClearedProvenanceHeaders returns the provenance headers that a proxy hop must blank,
// given the one header that this hop itself stamps. Pass the empty string for a hop that
// stamps nothing.
//
// Note that clearing to "" is not the same as removing: nginx omits a header set to the
// empty string, but a receiver testing presence rather than equality against the secret
// would still be wrong. The Cryostat-side comparison is against the full secret value,
// so a blank never matches.
func ClearedProvenanceHeaders(stamped string) []string {
	cleared := make([]string, 0, len(ProvenanceHeaders))
	for _, header := range ProvenanceHeaders {
		if header != stamped {
			cleared = append(cleared, header)
		}
	}
	return cleared
}
