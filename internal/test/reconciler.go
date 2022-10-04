// Copyright The Cryostat Authors
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package test

import (
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/onsi/gomega"
)

// TestReconcilerConfig groups parameters used to create a test Reconciler
type TestReconcilerConfig struct {
	Client                client.Client
	TLS                   bool
	EnvDisableTLS         *bool
	EnvCoreImageTag       *string
	EnvDatasourceImageTag *string
	EnvGrafanaImageTag    *string
	EnvReportsImageTag    *string
}

func NewTestReconcilerTLS(config *TestReconcilerConfig) common.ReconcilerTLS {
	return common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
		Client:  config.Client,
		OSUtils: newTestOSUtils(config),
	})
}

type testOSUtils struct {
	envs       map[string]string
	numPassGen int
}

func newTestOSUtils(config *TestReconcilerConfig) *testOSUtils {
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
	if config.EnvReportsImageTag != nil {
		envs["RELATED_IMAGE_REPORTS"] = *config.EnvReportsImageTag
	}
	return &testOSUtils{envs: envs}
}

func (o *testOSUtils) GetFileContents(path string) ([]byte, error) {
	gomega.Expect(path).To(gomega.Equal("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	return []byte("myToken"), nil
}

func (o *testOSUtils) GetEnv(name string) string {
	return o.envs[name]
}

func (o *testOSUtils) GenPasswd(length int) string {
	o.numPassGen++
	return fmt.Sprintf("pass%d", o.numPassGen)
}
