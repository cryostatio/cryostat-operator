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
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/onsi/gomega"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	authzv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Synthetic Kubernetes resource name backing the custom entry-gate accessReview.
const rbacAccessReviewGateResource = "cryostataccessreviewgate"

// rbacAccessReviewLevels returns the two ServiceAccounts this test compares: one granted only
// the auth proxy's default entry condition (pods:get, via the shared entry Role every RBAC
// test binds), one granted the CR's custom accessReview resource instead. Both hold the same
// fine-grained archivedrecordings:read grant, so the only variable is whether they pass the
// entry gate. CanRead doubles here as "holds the custom accessReview grant, and therefore
// expected to pass the gate."
func rbacAccessReviewLevels() []*rbacPrivilegeLevel {
	return []*rbacPrivilegeLevel{
		{Name: "oldgate", Verbs: []string{}},
		{Name: "newgate", Verbs: []string{"get"}, CanRead: true},
	}
}

// rbacAccessReviewRBACPermissions maps archivedrecordings:read onto a synthetic resource, held
// by both levels via rbacAccessReviewSupportingRules - independent of the entry-gate resource
// under test.
func rbacAccessReviewRBACPermissions() map[string]string {
	return map[string]string{
		rbacArchivedRecordingsReadPermission: rbacArchivesResource + ":get",
	}
}

// rbacAccessReviewSupportingRules grants archivedrecordings:read's backing resource to every
// level, so both ServiceAccounts would pass Cryostat's fine-grained check once past the entry
// gate - isolating the gate itself as the only variable under test.
func rbacAccessReviewSupportingRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{rbacArchivesResource},
			Verbs:     []string{"get"},
		},
	}
}

// newRBACAccessReviewCryostatCR builds the CR for this test: the standard RBAC CR, plus an
// OpenShift SSO accessReview configured to a synthetic resource instead of the default
// "pods:get", so the entry gate's own configurability can be exercised the same way the rest
// of this suite exercises fine-grained permission mappings - via Kubernetes RBAC rule-matching
// against a resource name that need not exist in the API server's registry.
func (r *TestResources) newRBACAccessReviewCryostatCR() *operatorv1beta2.Cryostat {
	cr := r.newRBACCryostatCR(rbacAccessReviewRBACPermissions())
	cr.Spec.AuthorizationOptions.OpenShiftSSO = &operatorv1beta2.OpenShiftSSOConfig{
		AccessReview: &authzv1.ResourceAttributes{
			Namespace: r.Namespace,
			Verb:      "get",
			Resource:  rbacAccessReviewGateResource,
		},
	}
	return cr
}

// newNoRedirectHttpClient returns an HTTP client identical to NewHttpClient's, except it does
// not follow redirects: checkAccessReviewGate needs to see the proxy's raw 302 response, not
// the OAuth login page it points to.
func newNoRedirectHttpClient() *http.Client {
	client := NewHttpClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

// checkAccessReviewGate issues a Cryostat REST request with the given level's token and
// asserts, via Gomega, whether the auth proxy's entry gate accepted or rejected it.
//
// This is deliberately separate from checkREST. A request that fails the proxy's delegated
// SAR is not met with a 401 or 403 the way a fine-grained Cryostat permission denial is:
// origin-oauth-proxy treats a Bearer token that fails -openshift-delegate-urls with falling
// through to the proxy's normal browser login flow (a 302 redirect to the OpenShift OAuth
// authorization endpoint) rather than a flat rejection, since the proxy has no session cookie to
// fall back on either.
func (r *TestResources) checkAccessReviewGate(ctx context.Context, level *rbacPrivilegeLevel, expectPass bool, requestURL *url.URL) {
	g := gomega.NewWithT(&rbacGomegaT{r: r})
	label := fmt.Sprintf("entry gate as %s, expecting %s", level.Name, rbacExpectation(expectPass))

	header := make(http.Header)
	header.Set("Accept", "*/*")
	resp, err := SendRequestWithToken(ctx, newNoRedirectHttpClient(), http.MethodGet, requestURL.String(), nil, header, level.Token)
	if !g.Expect(err).To(gomega.Succeed(), func() string {
		return fmt.Sprintf("%s: request failed", label)
	}) {
		return
	}
	defer r.closeStream(resp.Body)

	if expectPass {
		if !g.Expect(StatusOK(resp.StatusCode)).To(gomega.BeTrue(), func() string {
			return fmt.Sprintf("%s: got %d, want 2xx; body: %s", label, resp.StatusCode, ReadError(resp))
		}) {
			return
		}
	} else {
		if !g.Expect(resp.StatusCode).To(gomega.Equal(http.StatusFound), func() string {
			return fmt.Sprintf("%s: got %d, want 302 (a redirect to the OpenShift OAuth login,"+
				" which is how this proxy signals that a Bearer token failed its delegated SAR);"+
				" body: %s", label, resp.StatusCode, ReadError(resp))
		}) {
			return
		}
		location := resp.Header.Get("Location")
		if !g.Expect(location).To(gomega.ContainSubstring("/oauth/authorize"), func() string {
			return fmt.Sprintf("%s: got a 302 to %q, want a redirect to the OAuth login endpoint", label, location)
		}) {
			return
		}
	}

	r.Log += fmt.Sprintf("%s: got %d, as expected\n", label, resp.StatusCode)
}

// rbacAccessReviewAssertions checks the entry gate outcome for both levels, then, only for the
// level expected to pass it, confirms the request proceeds to a normal fine-grained
// archivedrecordings:read evaluation. The "oldgate" level has nothing further to check once the
// gate has rejected it with a redirect to the OAuth login.
func rbacAccessReviewAssertions() []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.checkAccessReviewGate(ctx, level, level.CanRead, base.JoinPath("/api/v4/recordings"))
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			if !level.CanRead {
				return
			}
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, true, rbacArchivedRecordingsReadPermission, http.MethodGet,
				base.JoinPath("/api/v4/recordings"), nil, header)
		},
	}
}

// CryostatRBACAccessReviewTest checks that the auth proxy honors a custom
// openShiftSSO.accessReview instead of the default "get pods" entry condition: a ServiceAccount
// holding only the old default, but not the newly configured resource, is rejected - via a
// redirect to the OAuth login page, this proxy's signal for a failed delegated SAR, see
// checkAccessReviewGate - before Cryostat evaluates any fine-grained permission, while a
// ServiceAccount holding the new resource passes the gate and proceeds to a normal permission
// check.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACAccessReviewTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACAccessReviewTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACAccessReviewTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACAccessReviewTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacAccessReviewLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacAccessReviewGateResource, rbacAccessReviewSupportingRules(), levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACAccessReviewCryostatCR())
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACAccessReviewTestName, err.Error()))
	}
	err = r.StartLogs(cr)
	if err != nil {
		r.Log += fmt.Sprintf("failed to retrieve logs for the application: %s\n", err.Error())
	}

	base, err := url.Parse(cr.Status.ApplicationURL)
	if err != nil {
		return r.fail(fmt.Sprintf("application URL is invalid: %s", err.Error()))
	}
	err = r.waitTillCryostatReady(base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to reach the application: %s", err.Error()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	r.assertRBACMatrix(ctx, base, nil, rbacAccessReviewGateResource, levels, rbacAccessReviewAssertions())

	return r.TestResult
}
