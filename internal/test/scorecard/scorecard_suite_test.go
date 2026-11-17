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

package scorecard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScorecard(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scorecard Suite")
}

func testCryostatAPI(handler http.HandlerFunc) *CryostatRESTClientset {
	GinkgoHelper()
	// REST requests load a Kubernetes bearer token even when talking to this fake API.
	config := filepath.Join(GinkgoT().TempDir(), "kubeconfig")
	Expect(os.WriteFile(config, []byte(`apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: test
    user: test
clusters:
- name: test
  cluster:
    server: https://unused.invalid
users:
- name: test
  user:
    token: scorecard-test
`), 0600)).To(Succeed())
	GinkgoT().Setenv("KUBECONFIG", config)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer GinkgoRecover()
		handler(w, request)
	}))
	DeferCleanup(server.Close)
	base, err := url.Parse(server.URL)
	Expect(err).ToNot(HaveOccurred())
	return newTestResources("test", "test").NewCryostatRESTClientset(base)
}
