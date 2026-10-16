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

var _ = Describe("BuildFromProfile", func() {
	It("should populate all fields for the Intermediate profile", func() {
		profile := &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileIntermediateType,
		}
		config, err := BuildFromProfile(profile)
		Expect(err).ToNot(HaveOccurred())
		Expect(config.NginxProtocols).ToNot(BeEmpty())
		Expect(config.QuarkusCipherSuites).ToNot(BeEmpty())
		Expect(config.PostgresMinVersion).ToNot(BeEmpty())
		Expect(config.SeaweedMinVersion).ToNot(BeEmpty())
	})
})
