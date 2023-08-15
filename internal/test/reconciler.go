package test

import (
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/onsi/gomega"
)

// TestReconcilerConfig groups parameters used to create a test Reconciler
type TestReconcilerConfig struct {
	Client                client.Client
	EnvDisableTLS         *bool
	EnvCoreImageTag       *string
	EnvDatasourceImageTag *string
	EnvGrafanaImageTag    *string
	EnvReportsImageTag    *string
	GeneratedPasswords    []string
}

func NewTestReconcilerTLS(config *TestReconcilerConfig) common.ReconcilerTLS {
	return common.NewReconcilerTLS(&common.ReconcilerTLSConfig{
		Client:  config.Client,
		OSUtils: newTestOSUtils(config),
	})
}

type testOSUtils struct {
	envs       map[string]string
	passwords  []string
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
	return &testOSUtils{envs: envs, passwords: config.GeneratedPasswords}
}

func (o *testOSUtils) GetFileContents(path string) ([]byte, error) {
	gomega.Expect(path).To(gomega.Equal("/var/run/secrets/kubernetes.io/serviceaccount/token"))
	return []byte("myToken"), nil
}

func (o *testOSUtils) GetEnv(name string) string {
	return o.envs[name]
}

func (o *testOSUtils) GenPasswd(length int) string {
	gomega.Expect(o.numPassGen < len(o.passwords)).To(gomega.BeTrue())
	password := o.passwords[o.numPassGen]
	o.numPassGen++
	return password
}
