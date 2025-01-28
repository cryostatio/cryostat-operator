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

package console_test

import (
	"context"
	"go/build"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftoperatorv1 "github.com/openshift/api/operator/v1"

	//+kubebuilder:scaffold:imports

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var k8sScheme *runtime.Scheme
var logger logr.Logger

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Console Suite")
}

var _ = BeforeSuite(func() {
	logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(logger)

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")

	openshiftModVersion := os.Getenv("OPENSHIFT_API_MOD_VERSION")
	Expect(openshiftModVersion).ToNot(BeEmpty(), "OPENSHIFT_API_MOD_VERSION environment variable must be set")
	openshiftPrefix := []string{build.Default.GOPATH, "pkg", "mod", "github.com", "openshift",
		"api@" + openshiftModVersion}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(append(openshiftPrefix, "console", "v1", "zz_generated.crd-manifests", "90_consoleplugins.crd.yaml")...),
			filepath.Join(append(openshiftPrefix, "operator", "v1", "zz_generated.crd-manifests", "0000_50_console_01_consoles.crd.yaml")...),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sScheme = runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		scheme.AddToScheme,
		consolev1.AddToScheme,
		openshiftoperatorv1.AddToScheme,
	)
	err = sb.AddToScheme(k8sScheme)
	Expect(err).ToNot(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: k8sScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
