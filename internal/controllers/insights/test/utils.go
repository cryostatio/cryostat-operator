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

package test

import (
	"strconv"
)

// TestUtilsConfig groups parameters used to create a test OSUtils
type TestUtilsConfig struct {
	EnvInsightsEnabled       *bool
	EnvInsightsProxyImageTag *string
	EnvInsightsBackendDomain *string
	EnvInsightsProxyDomain   *string
	EnvNamespace             *string
}

type testOSUtils struct {
	envs map[string]string
}

func NewTestOSUtils(config *TestUtilsConfig) *testOSUtils {
	envs := map[string]string{}
	if config.EnvInsightsEnabled != nil {
		envs["INSIGHTS_ENABLED"] = strconv.FormatBool(*config.EnvInsightsEnabled)
	}
	if config.EnvInsightsProxyImageTag != nil {
		envs["RELATED_IMAGE_INSIGHTS_PROXY"] = *config.EnvInsightsProxyImageTag
	}
	if config.EnvInsightsBackendDomain != nil {
		envs["INSIGHTS_BACKEND_DOMAIN"] = *config.EnvInsightsBackendDomain
	}
	if config.EnvInsightsProxyDomain != nil {
		envs["INSIGHTS_PROXY_DOMAIN"] = *config.EnvInsightsProxyDomain
	}
	if config.EnvNamespace != nil {
		envs["NAMESPACE"] = *config.EnvNamespace
	}
	return &testOSUtils{envs: envs}
}

func (o *testOSUtils) GetFileContents(path string) ([]byte, error) {
	// Unused
	return nil, nil
}

func (o *testOSUtils) GetEnv(name string) string {
	return o.envs[name]
}

func (o *testOSUtils) GenPasswd(length int) string {
	// Unused
	return ""
}
