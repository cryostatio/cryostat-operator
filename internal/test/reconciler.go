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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/onsi/gomega"
)

// TestReconcilerConfig groups parameters used to create a test Reconciler
type TestReconcilerConfig struct {
	Client                         client.Client
	EnvDisableTLS                  *bool
	EnvOAuth2ProxyImageTag         *string
	EnvOpenShiftOAuthProxyImageTag *string
	EnvCoreImageTag                *string
	EnvDatasourceImageTag          *string
	EnvStorageImageTag             *string
	EnvDatabaseImageTag            *string
	EnvGrafanaImageTag             *string
	EnvReportsImageTag             *string
	EnvAgentProxyImageTag          *string
	EnvAgentInitImageTag           *string
	GeneratedPasswords             []string
	ControllerBuilder              *TestCtrlBuilder
	CertManagerMissing             bool
}

func NewTestReconcilerTLS(config *TestReconcilerConfig) common.ReconcilerTLS {
	return common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
		Client: config.Client,
		OS:     NewTestOSUtils(config),
	})
}

type testOSUtils struct {
	envs       map[string]string
	passwords  []string
	numPassGen int
}

func NewTestOSUtils(config *TestReconcilerConfig) common.OSUtils {
	envs := map[string]string{}
	if config.EnvDisableTLS != nil {
		envs["DISABLE_SERVICE_TLS"] = strconv.FormatBool(*config.EnvDisableTLS)
	}
	if config.EnvCoreImageTag != nil {
		envs["RELATED_IMAGE_CORE"] = *config.EnvCoreImageTag
	}
	if config.EnvDatasourceImageTag != nil {
		envs["RELATED_IMAGE_DATASOURCE"] = *config.EnvDatasourceImageTag
	}
	if config.EnvGrafanaImageTag != nil {
		envs["RELATED_IMAGE_GRAFANA"] = *config.EnvGrafanaImageTag
	}
	if config.EnvStorageImageTag != nil {
		envs["RELATED_IMAGE_STORAGE"] = *config.EnvStorageImageTag
	}
	if config.EnvDatabaseImageTag != nil {
		envs["RELATED_IMAGE_DATABASE"] = *config.EnvDatabaseImageTag
	}
	if config.EnvReportsImageTag != nil {
		envs["RELATED_IMAGE_REPORTS"] = *config.EnvReportsImageTag
	}
	if config.EnvOAuth2ProxyImageTag != nil {
		envs["RELATED_IMAGE_OAUTH2_PROXY"] = *config.EnvOAuth2ProxyImageTag
	}
	if config.EnvOpenShiftOAuthProxyImageTag != nil {
		envs["RELATED_IMAGE_OPENSHIFT_OAUTH_PROXY"] = *config.EnvOpenShiftOAuthProxyImageTag
	}
	if config.EnvAgentProxyImageTag != nil {
		envs["RELATED_IMAGE_AGENT_PROXY"] = *config.EnvAgentProxyImageTag
	}
	if config.EnvAgentInitImageTag != nil {
		envs["RELATED_IMAGE_AGENT_INIT"] = *config.EnvAgentInitImageTag
	}
	return &testOSUtils{envs: envs, passwords: config.GeneratedPasswords}
}

func (o *testOSUtils) GetFileContents(path string) ([]byte, error) {
	gomega.Expect(path).To(gomega.Equal("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	return []byte("myToken"), nil
}

func (o *testOSUtils) GetEnv(name string) string {
	return o.envs[name]
}

func (o *testOSUtils) GetEnvOrDefault(name string, defaultVal string) string {
	val := o.GetEnv(name)
	if len(val) > 0 {
		return val
	}
	return defaultVal
}

func (o *testOSUtils) GenPasswd(length int) string {
	gomega.Expect(o.numPassGen < len(o.passwords)).To(gomega.BeTrue())
	password := o.passwords[o.numPassGen]
	o.numPassGen++
	return password
}
