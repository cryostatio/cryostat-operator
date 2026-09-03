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
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestOpensslToIANAMapCoversAllProfiles(t *testing.T) {
	for profileType, spec := range configv1.TLSProfiles {
		for _, cipher := range spec.Ciphers {
			if _, ok := opensslToIANAMap[cipher]; !ok {
				t.Errorf("opensslToIANAMap is missing cipher %q from the %s TLS profile", cipher, profileType)
			}
		}
	}
}

func TestOpensslToIANAOutputNonEmpty(t *testing.T) {
	for profileType, spec := range configv1.TLSProfiles {
		result := opensslToIANA(spec.Ciphers)
		if len(result) == 0 {
			t.Errorf("opensslToIANA returned empty for the %s TLS profile", profileType)
		}
	}
}

func TestFilterTLS12Ciphers(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		want   []string
	}{
		{
			name:  "TLS 1.3 ciphers are excluded",
			input: []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
			want:  []string{},
		},
		{
			name:  "TLS 1.2 ciphers are kept",
			input: []string{"ECDHE-RSA-AES128-GCM-SHA256", "AES128-SHA256"},
			want:  []string{"ECDHE-RSA-AES128-GCM-SHA256", "AES128-SHA256"},
		},
		{
			name:  "mixed input filters correctly",
			input: []string{"TLS_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256", "TLS_CHACHA20_POLY1305_SHA256"},
			want:  []string{"ECDHE-RSA-AES128-GCM-SHA256"},
		},
		{
			name:  "Modern profile produces empty list",
			input: configv1.TLSProfiles[configv1.TLSProfileModernType].Ciphers,
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterTLS12Ciphers(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("filterTLS12Ciphers(%v) returned %d ciphers, want %d", tt.input, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("filterTLS12Ciphers(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildFromProfileIntermediateDefaults(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileIntermediateType,
	}
	config, err := BuildFromProfile(profile)
	if err != nil {
		t.Fatalf("BuildFromProfile(Intermediate) returned error: %v", err)
	}
	if config.NginxProtocols == "" {
		t.Error("NginxProtocols is empty")
	}
	if config.QuarkusCipherSuites == "" {
		t.Error("QuarkusCipherSuites is empty")
	}
	if config.PostgresMinVersion == "" {
		t.Error("PostgresMinVersion is empty")
	}
	if config.SeaweedMinVersion == "" {
		t.Error("SeaweedMinVersion is empty")
	}
}
