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
	"context"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

// FetchAndBuild fetches the cluster API Server TLS profile and builds an OperandTLSConfig.
// Returns nil if the profile is not set (defaults).
func FetchAndBuild(ctx context.Context, cl client.Client) (*OperandTLSConfig, error) {
	apiServer := &configv1.APIServer{}
	if err := cl.Get(ctx, client.ObjectKey{Name: "cluster"}, apiServer); err != nil {
		return nil, fmt.Errorf("get APIServer config: %w", err)
	}

	profile := apiServer.Spec.TLSSecurityProfile
	if profile == nil {
		profile = &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}
	}

	return BuildFromProfile(profile)
}

// BuildFromProfile builds an OperandTLSConfig from a TLSSecurityProfile.
func BuildFromProfile(profile *configv1.TLSSecurityProfile) (*OperandTLSConfig, error) {
	spec, err := resolveSpec(profile)
	if err != nil {
		return nil, err
	}

	minVersion := string(spec.MinTLSVersion)
	ciphers := spec.Ciphers

	protocols := protocolsFromMinVersion(minVersion)
	// OpenSSL's SSL_CTX_set_cipher_list (used by nginx ssl_ciphers and
	// PostgreSQL ssl_ciphers) only handles TLS 1.2 and below. TLS 1.3
	// cipher suites are configured separately and must be excluded here.
	opensslCiphers := filterTLS12Ciphers(ciphers)

	return &OperandTLSConfig{
		NginxProtocols:      strings.Join(protocols, " "),
		NginxCiphers:        strings.Join(opensslCiphers, ":"),
		QuarkusProtocols:    strings.Join(protocols, ","),
		QuarkusCipherSuites: strings.Join(opensslToIANA(ciphers), ","),
		PostgresMinVersion:  minVersionToPostgres(minVersion),
		PostgresCiphers:     strings.Join(opensslCiphers, ":"),
		SeaweedMinVersion:   minVersionToSeaweed(minVersion),
		SeaweedCipherSuites: strings.Join(opensslToIANA(ciphers), ","),
	}, nil
}

func resolveSpec(profile *configv1.TLSSecurityProfile) (*configv1.TLSProfileSpec, error) {
	switch profile.Type {
	case configv1.TLSProfileOldType,
		configv1.TLSProfileIntermediateType,
		configv1.TLSProfileModernType:
		spec, ok := configv1.TLSProfiles[profile.Type]
		if !ok {
			return nil, fmt.Errorf("unknown built-in TLS profile type: %s", profile.Type)
		}
		return spec, nil
	case configv1.TLSProfileCustomType:
		if profile.Custom == nil {
			return nil, fmt.Errorf("custom profile specified but Custom is nil")
		}
		return &profile.Custom.TLSProfileSpec, nil
	default:
		spec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		return spec, nil
	}
}

// protocolsFromMinVersion returns the list of TLS protocol version strings
// enabled at and above the given minimum, in descending order.
func protocolsFromMinVersion(minVersion string) []string {
	versions := []struct {
		name string
		rank int
	}{
		{"TLSv1.3", 4},
		{"TLSv1.2", 3},
		{"TLSv1.1", 2},
		{"TLSv1.0", 1},
	}

	minRank := versionRank(minVersion)
	var result []string
	for _, v := range versions {
		if v.rank >= minRank {
			result = append(result, v.name)
		}
	}
	if len(result) == 0 {
		return []string{"TLSv1.2", "TLSv1.3"}
	}
	return result
}

func versionRank(v string) int {
	switch v {
	case "VersionTLS10":
		return 1
	case "VersionTLS11":
		return 2
	case "VersionTLS12":
		return 3
	case "VersionTLS13":
		return 4
	default:
		return 3
	}
}

func minVersionToPostgres(v string) string {
	switch v {
	case "VersionTLS10":
		return "TLSv1"
	case "VersionTLS11":
		return "TLSv1.1"
	case "VersionTLS12":
		return "TLSv1.2"
	case "VersionTLS13":
		return "TLSv1.3"
	default:
		return "TLSv1.2"
	}
}

func minVersionToSeaweed(v string) string {
	switch v {
	case "VersionTLS10":
		return "TLS 1.0"
	case "VersionTLS11":
		return "TLS 1.1"
	case "VersionTLS12":
		return "TLS 1.2"
	case "VersionTLS13":
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

// FetchAPIServerTLSProfile fetches the APIServer resource for use by the reconciler.
func FetchAPIServerTLSProfile(ctx context.Context, cl client.Client) (*configv1.TLSSecurityProfile, error) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}
	if err := cl.Get(ctx, client.ObjectKeyFromObject(apiServer), apiServer); err != nil {
		return nil, fmt.Errorf("get APIServer config: %w", err)
	}
	return apiServer.Spec.TLSSecurityProfile, nil
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
