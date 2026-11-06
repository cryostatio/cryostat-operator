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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("opensslToIANAMap", func() {
	It("should cover all ciphers in every built-in TLS profile", func() {
		for profileType, spec := range configv1.TLSProfiles {
			for _, cipher := range spec.Ciphers {
				_, ok := opensslToIANAMap[cipher]
				Expect(ok).To(BeTrue(), fmt.Sprintf("opensslToIANAMap is missing cipher %q from the %s TLS profile", cipher, profileType))
			}
		}
	})

	It("should produce non-empty IANA output for every built-in TLS profile", func() {
		for profileType, spec := range configv1.TLSProfiles {
			result := opensslToIANA(spec.Ciphers)
			Expect(result).ToNot(BeEmpty(), fmt.Sprintf("opensslToIANA returned empty for the %s TLS profile", profileType))
		}
	})
})

var _ = Describe("filterTLS12Ciphers", func() {
	It("should exclude TLS 1.3 ciphers", func() {
		input := []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"}
		Expect(filterTLS12Ciphers(input)).To(BeEmpty())
	})

	It("should keep TLS 1.2 ciphers", func() {
		input := []string{"ECDHE-RSA-AES128-GCM-SHA256", "AES128-SHA256"}
		Expect(filterTLS12Ciphers(input)).To(Equal(input))
	})

	It("should filter correctly with mixed input", func() {
		input := []string{"TLS_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256", "TLS_CHACHA20_POLY1305_SHA256"}
		Expect(filterTLS12Ciphers(input)).To(Equal([]string{"ECDHE-RSA-AES128-GCM-SHA256"}))
	})

	It("should produce an empty list for the Modern profile", func() {
		input := configv1.TLSProfiles[configv1.TLSProfileModernType].Ciphers
		Expect(filterTLS12Ciphers(input)).To(BeEmpty())
	})
})

var _ = Describe("BuildFromSpec", func() {
	It("should populate all fields for the Intermediate profile", func() {
		spec := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		config, err := BuildFromSpec(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(config.NginxProtocols).ToNot(BeEmpty())
		Expect(config.QuarkusCipherSuites).ToNot(BeEmpty())
		Expect(config.PostgresMinVersion).ToNot(BeEmpty())
		Expect(config.SeaweedMinVersion).ToNot(BeEmpty())
		Expect(config.OpenShiftOAuthProxyMinVersion).ToNot(BeEmpty())
		Expect(config.OpenShiftOAuthProxyCipherSuites).ToNot(BeEmpty())
	})

	It("should expose the OpenShift oauth-proxy min version in Go version-name format", func() {
		spec := configv1.TLSProfileSpec{
			Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			MinTLSVersion: configv1.VersionTLS13,
		}
		config, err := BuildFromSpec(spec)
		Expect(err).ToNot(HaveOccurred())
		// The oauth-proxy --tls-min-version flag consumes the OpenShift API
		// version name verbatim, unlike the agent/Postgres "TLSv1.x" format.
		Expect(config.OpenShiftOAuthProxyMinVersion).To(Equal("VersionTLS13"))
		Expect(config.OpenShiftOAuthProxyCipherSuites).To(Equal("TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"))
	})

	It("should raise the OpenShift oauth-proxy min version to TLS 1.2 for the Old profile", func() {
		// The Old profile is MinTLSVersion VersionTLS10, which the oauth-proxy
		// rejects; it must be clamped to its TLS 1.2 floor.
		spec := *configv1.TLSProfiles[configv1.TLSProfileOldType]
		Expect(spec.MinTLSVersion).To(Equal(configv1.VersionTLS10))
		config, err := BuildFromSpec(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(config.OpenShiftOAuthProxyMinVersion).To(Equal("VersionTLS12"))
	})

	It("should filter out OpenShift oauth-proxy ciphers the proxy does not recognize", func() {
		// The Old profile includes 3DES, static-RSA, and CBC-SHA256/384 ciphers
		// that the oauth-proxy rejects. The result must be a non-empty subset
		// containing only forward-secret suites the proxy accepts.
		spec := *configv1.TLSProfiles[configv1.TLSProfileOldType]
		config, err := BuildFromSpec(spec)
		Expect(err).ToNot(HaveOccurred())
		result := strings.Split(config.OpenShiftOAuthProxyCipherSuites, ",")
		Expect(result).ToNot(BeEmpty())
		for _, name := range result {
			Expect(oauthProxyCipherSuiteNames).To(HaveKey(name))
		}
		Expect(config.OpenShiftOAuthProxyCipherSuites).ToNot(ContainSubstring("3DES"))
		Expect(config.OpenShiftOAuthProxyCipherSuites).ToNot(ContainSubstring("TLS_RSA_WITH"))
	})

	It("should leave the OpenShift oauth-proxy ciphers empty when none are recognized", func() {
		// A Custom profile listing only ciphers the proxy rejects must yield an
		// empty string so the operator omits the --tls-cipher-suites flag rather
		// than crashing the proxy.
		spec := configv1.TLSProfileSpec{
			Ciphers:       []string{"DES-CBC3-SHA", "AES128-SHA"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		config, err := BuildFromSpec(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(config.OpenShiftOAuthProxyCipherSuites).To(BeEmpty())
	})

	It("should return an error when all ciphers are unrecognized", func() {
		spec := configv1.TLSProfileSpec{
			Ciphers:       []string{"UNKNOWN-CIPHER-1", "UNKNOWN-CIPHER-2"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		_, err := BuildFromSpec(spec)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no cipher suites recognized"))
	})

	It("should succeed when some ciphers are unrecognized", func() {
		spec := configv1.TLSProfileSpec{
			Ciphers:       []string{"UNKNOWN-CIPHER", "ECDHE-RSA-AES128-GCM-SHA256"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		config, err := BuildFromSpec(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(config.QuarkusCipherSuites).To(Equal("TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"))
	})
})
