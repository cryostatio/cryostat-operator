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

package tlsprofile

import (
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
)

// OpenShift API TLS version names (from configv1.TLSProtocolVersion constants).
const (
	versionTLS10 = "VersionTLS10"
	versionTLS11 = "VersionTLS11"
	versionTLS12 = "VersionTLS12"
	versionTLS13 = "VersionTLS13"
)

// TLS protocol version strings used by operands (nginx, Quarkus, PostgreSQL, SeaweedFS).
const (
	protoTLS10 = "TLSv1.0"
	protoTLS11 = "TLSv1.1"
	protoTLS12 = "TLSv1.2"
	protoTLS13 = "TLSv1.3"
)

// OperandTLSConfig holds TLS configuration in formats suitable for each operand component.
type OperandTLSConfig struct {
	// NginxProtocols is a space-separated protocol list for nginx ssl_protocols directive.
	NginxProtocols string
	// NginxCiphers is the OpenSSL cipher string for nginx ssl_ciphers directive.
	NginxCiphers string
	// QuarkusProtocols is a comma-separated protocol list for Quarkus (e.g. "TLSv1.3,TLSv1.2").
	QuarkusProtocols string
	// QuarkusCipherSuites is a comma-separated list of IANA cipher names for Quarkus.
	QuarkusCipherSuites string
	// PostgresMinVersion is the min protocol version for PostgreSQL ssl_min_protocol_version.
	PostgresMinVersion string
	// PostgresCiphers is the OpenSSL cipher string for PostgreSQL ssl_ciphers.
	PostgresCiphers string
	// SeaweedMinVersion is the TLS min version for SeaweedFS (e.g. "TLS 1.2").
	SeaweedMinVersion string
	// SeaweedCipherSuites is a comma-separated list of IANA cipher names for SeaweedFS.
	SeaweedCipherSuites string
	// AgentMinVersion is the min TLS version for the Cryostat agent in Java format (e.g. "TLSv1.2").
	AgentMinVersion string
	// AgentCipherSuites is a comma-separated list of IANA cipher names for the Cryostat agent.
	AgentCipherSuites string
}

// BuildFromSpec builds an OperandTLSConfig from a resolved TLSProfileSpec.
func BuildFromSpec(spec configv1.TLSProfileSpec) (*OperandTLSConfig, error) {
	minVersion := string(spec.MinTLSVersion)
	ciphers := spec.Ciphers

	protocols := protocolsFromMinVersion(minVersion)
	// OpenSSL's SSL_CTX_set_cipher_list (used by nginx ssl_ciphers and
	// PostgreSQL ssl_ciphers) only handles TLS 1.2 and below. TLS 1.3
	// cipher suites are configured separately and must be excluded here.
	opensslCiphers := filterTLS12Ciphers(ciphers)
	ianaCiphers := opensslToIANA(ciphers)
	if len(ciphers) > 0 && len(ianaCiphers) == 0 {
		return nil, fmt.Errorf("TLS profile contains no cipher suites recognized by the operands")
	}

	return &OperandTLSConfig{
		NginxProtocols:      strings.Join(protocols, " "),
		NginxCiphers:        strings.Join(opensslCiphers, ":"),
		QuarkusProtocols:    strings.Join(protocols, ","),
		QuarkusCipherSuites: strings.Join(ianaCiphers, ","),
		PostgresMinVersion:  minVersionToPostgres(minVersion),
		PostgresCiphers:     strings.Join(opensslCiphers, ":"),
		SeaweedMinVersion:   minVersionToSeaweed(minVersion),
		SeaweedCipherSuites: strings.Join(ianaCiphers, ","),
		AgentMinVersion:     minVersionToAgent(minVersion),
		AgentCipherSuites:   strings.Join(ianaCiphers, ","),
	}, nil
}

// protocolsFromMinVersion returns the list of TLS protocol version strings
// enabled at and above the given minimum, in descending order.
func protocolsFromMinVersion(minVersion string) []string {
	versions := []struct {
		name string
		rank int
	}{
		{protoTLS13, 4},
		{protoTLS12, 3},
		{protoTLS11, 2},
		{protoTLS10, 1},
	}

	minRank := versionRank(minVersion)
	var result []string
	for _, v := range versions {
		if v.rank >= minRank {
			result = append(result, v.name)
		}
	}
	if len(result) == 0 {
		return []string{protoTLS12, protoTLS13}
	}
	return result
}

func versionRank(v string) int {
	switch v {
	case versionTLS10:
		return 1
	case versionTLS11:
		return 2
	case versionTLS12:
		return 3
	case versionTLS13:
		return 4
	default:
		return 3
	}
}

func minVersionToPostgres(v string) string {
	switch v {
	case versionTLS10:
		return "TLSv1"
	case versionTLS11:
		return protoTLS11
	case versionTLS12:
		return protoTLS12
	case versionTLS13:
		return protoTLS13
	default:
		return protoTLS12
	}
}

// minVersionToAgent converts an OpenShift API TLS version name to the
// "TLSv1.x" protocol string used by the Cryostat agent.
func minVersionToAgent(v string) string {
	switch v {
	case versionTLS10:
		return protoTLS10
	case versionTLS11:
		return protoTLS11
	case versionTLS12:
		return protoTLS12
	case versionTLS13:
		return protoTLS13
	default:
		return protoTLS12
	}
}

func minVersionToSeaweed(v string) string {
	switch v {
	case versionTLS10:
		return "TLS 1.0"
	case versionTLS11:
		return "TLS 1.1"
	case versionTLS12:
		return "TLS 1.2"
	case versionTLS13:
		return "TLS 1.3"
	default:
		return "TLS 1.2"
	}
}

// opensslToIANA converts OpenSSL cipher suite names (used in OpenShift TLS profiles)
// to IANA standard names (used by Java/Quarkus and SeaweedFS).
func opensslToIANA(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if iana, ok := opensslToIANAMap[name]; ok {
			out = append(out, iana)
		}
	}
	return out
}

// filterTLS12Ciphers returns only TLS 1.2 and below cipher suite names from the
// input list. TLS 1.3 cipher suites (those starting with "TLS_" and not containing
// "_WITH_") are excluded because OpenSSL's SSL_CTX_set_cipher_list — used by nginx
// ssl_ciphers and PostgreSQL ssl_ciphers — does not handle TLS 1.3 suites. Those
// suites are configured through a separate OpenSSL API and are typically always
// available when TLS 1.3 is enabled.
//
// The naming distinction comes from RFC 8446 (TLS 1.3), which removed key exchange
// from cipher suite definitions. TLS 1.2 IANA names follow TLS_<KX>_WITH_<Cipher>_<MAC>
// (RFC 5246), while TLS 1.3 names follow TLS_<AEAD>_<Hash> with no "_WITH_". In
// practice, the OpenShift TLS profiles use OpenSSL-format names for TLS 1.2 ciphers
// (e.g. "ECDHE-RSA-AES128-GCM-SHA256") which never start with "TLS_", so the
// "_WITH_" check is a safety net for IANA-format input.
func filterTLS12Ciphers(ciphers []string) []string {
	out := make([]string, 0, len(ciphers))
	for _, c := range ciphers {
		if strings.HasPrefix(c, "TLS_") && !strings.Contains(c, "_WITH_") {
			continue
		}
		out = append(out, c)
	}
	return out
}

// opensslToIANAMap maps OpenSSL cipher suite names (used in OpenShift TLS profiles)
// to IANA standard names (used by Java/Quarkus, SeaweedFS, and the Cryostat agent).
//
// We maintain this map ourselves rather than using library-go's
// crypto.OpenSSLToIANACipherSuites because that function silently drops ciphers
// that Go's crypto/tls cannot negotiate (e.g. all DHE-RSA ciphers). Our operands
// use OpenSSL or Java TLS stacks that support a broader set of ciphers, so we
// must preserve the full list configured by the cluster admin.
//
// TestOpensslToIANAMapCoversAllProfiles verifies this map covers every cipher
// in the built-in OpenShift TLS profiles.
//
//nolint:goconst // Cipher names are lookup table keys; extracting constants would reduce readability.
var opensslToIANAMap = map[string]string{
	"ECDHE-ECDSA-AES128-GCM-SHA256": "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
	"ECDHE-RSA-AES128-GCM-SHA256":   "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
	"ECDHE-ECDSA-AES256-GCM-SHA384": "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	"ECDHE-RSA-AES256-GCM-SHA384":   "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
	"ECDHE-ECDSA-CHACHA20-POLY1305": "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
	"ECDHE-RSA-CHACHA20-POLY1305":   "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
	"DHE-RSA-AES128-GCM-SHA256":     "TLS_DHE_RSA_WITH_AES_128_GCM_SHA256",
	"DHE-RSA-AES256-GCM-SHA384":     "TLS_DHE_RSA_WITH_AES_256_GCM_SHA384",
	"DHE-RSA-CHACHA20-POLY1305":     "TLS_DHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
	"ECDHE-ECDSA-AES128-SHA256":     "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
	"ECDHE-RSA-AES128-SHA256":       "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
	"ECDHE-ECDSA-AES128-SHA":        "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
	"ECDHE-RSA-AES128-SHA":          "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
	"ECDHE-ECDSA-AES256-SHA384":     "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384",
	"ECDHE-RSA-AES256-SHA384":       "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384",
	"ECDHE-ECDSA-AES256-SHA":        "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
	"ECDHE-RSA-AES256-SHA":          "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
	"DHE-RSA-AES128-SHA256":         "TLS_DHE_RSA_WITH_AES_128_CBC_SHA256",
	"DHE-RSA-AES256-SHA256":         "TLS_DHE_RSA_WITH_AES_256_CBC_SHA256",
	"AES128-GCM-SHA256":             "TLS_RSA_WITH_AES_128_GCM_SHA256",
	"AES256-GCM-SHA384":             "TLS_RSA_WITH_AES_256_GCM_SHA384",
	"AES128-SHA256":                 "TLS_RSA_WITH_AES_128_CBC_SHA256",
	"AES256-SHA256":                 "TLS_RSA_WITH_AES_256_CBC_SHA256",
	"AES128-SHA":                    "TLS_RSA_WITH_AES_128_CBC_SHA",
	"AES256-SHA":                    "TLS_RSA_WITH_AES_256_CBC_SHA",
	"DES-CBC3-SHA":                  "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
	"TLS_AES_128_GCM_SHA256":        "TLS_AES_128_GCM_SHA256",
	"TLS_AES_256_GCM_SHA384":        "TLS_AES_256_GCM_SHA384",
	"TLS_CHACHA20_POLY1305_SHA256":  "TLS_CHACHA20_POLY1305_SHA256",
}
