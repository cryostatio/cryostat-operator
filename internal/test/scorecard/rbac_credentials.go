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

	"github.com/cryostatio/cryostat-operator/internal/cryostatapi"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	rbacv1 "k8s.io/api/rbac/v1"
)

// Synthetic Kubernetes resource name backing credentials.
const rbacCredentialsResource = "cryostatcredentials"

// The Cryostat permission names the credentials ladder test asserts.
const (
	rbacCredentialsReadPermission   = "credentials:read"
	rbacCredentialsWritePermission  = "credentials:write"
	rbacCredentialsDeletePermission = "credentials:delete"
)

// rbacCredentialsLevels returns the credentials privilege ladder.
func rbacCredentialsLevels() []*rbacPrivilegeLevel {
	return newRBACPrivilegeLadder(rbacLadderVerbs{Read: "get", Write: "update", Delete: "delete"})
}

// rbacCredentialsRBACPermissions maps credentials and the supporting targets:read permission
// onto their synthetic resources.
func rbacCredentialsRBACPermissions() map[string]string {
	return map[string]string{
		rbacCredentialsReadPermission:   rbacCredentialsResource + ":get",
		rbacCredentialsWritePermission:  rbacCredentialsResource + ":update",
		rbacCredentialsDeletePermission: rbacCredentialsResource + ":delete",
		"targets:read":                  rbacTargetsResource + ":get",
	}
}

// rbacCredentialsSupportingRules grants targets:read to every level. Credentials.java pairs
// targets:read with credentials:read on the list/get endpoints, so without it every level
// would be denied regardless of its credentials verbs.
func rbacCredentialsSupportingRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{rbacTargetsResource},
			Verbs:     []string{"get"},
		},
	}
}

// newRBACCredential builds the form-encoded body for a stored credential that will never
// match any real target, so the fixture never actually gets used to establish a JMX
// connection.
func newRBACCredential(matchExpression string) *cryostatapi.Credential {
	return &cryostatapi.Credential{
		UserName:        "scorecard-rbac",
		Password:        "scorecard-rbac",
		MatchExpression: matchExpression,
	}
}

// setupRBACCredentialFixture creates one stored credential as the scorecard pod's own
// ServiceAccount, for the ladder's delete assertion to act on.
//
// The write assertion does not need a pre-existing credential of its own: it exercises
// credentials:write by creating a new one each time, the same way archivedrecordings:write is
// exercised through a PATCH that performs a write rather than a read-back.
func (r *TestResources) setupRBACCredentialFixture(ctx context.Context, base *url.URL) (uint32, error) {
	credential := newRBACCredential(`target.alias == "scorecard-rbac-no-such-target"`)
	body := credential.ToFormData()
	var created cryostatapi.CredentialResponse
	if err := r.sendPrivilegedRequest(ctx, http.MethodPost, base.JoinPath("/api/v4/credentials"),
		"application/x-www-form-urlencoded", &body, &created); err != nil {
		return 0, fmt.Errorf("failed to create a Credential fixture: %w", err)
	}
	r.Log += fmt.Sprintf("created Credential %d\n", created.Id)
	return created.Id, nil
}

// rbacCredentialsAssertions returns the credentials privilege matrix. There is no GraphQL
// surface for credentials, so this is REST-only.
func rbacCredentialsAssertions(credentialId uint32) []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacCredentialsReadPermission, http.MethodGet,
				base.JoinPath("/api/v4/credentials"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Content-Type", "application/x-www-form-urlencoded")
			header.Set("Accept", "*/*")
			// A distinct, never-matching expression per level, so a level's own repeated
			// writes across the matrix never collide with another level's.
			credential := newRBACCredential(fmt.Sprintf(`target.alias == "scorecard-rbac-no-such-target-%s"`, level.Name))
			body := credential.ToFormData()
			r.checkREST(ctx, level, level.CanWrite, rbacCredentialsWritePermission, http.MethodPost,
				base.JoinPath("/api/v4/credentials"), &body, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanDelete, rbacCredentialsDeletePermission, http.MethodDelete,
				base.JoinPath(fmt.Sprintf("/api/v4/credentials/%d", credentialId)), nil, header)
		},
	}
}

// CryostatRBACCredentialsTest checks that the fine-grained RBAC permission mappings for
// Cryostat's stored credentials (credentials:read/write/delete) are enforced by the deployed
// Cryostat application - the same property CryostatRBACArchivedRecordingsTest checks for
// archivedrecordings, applied to a different resource.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACCredentialsTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACCredentialsTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACCredentialsTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACCredentialsTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacCredentialsLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacCredentialsResource, rbacCredentialsSupportingRules(), levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacCredentialsRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACCredentialsTestName, err.Error()))
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

	credentialId, err := r.setupRBACCredentialFixture(ctx, base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up a Credential fixture for %s test: %s", CryostatRBACCredentialsTestName, err.Error()))
	}

	r.assertRBACMatrix(ctx, base, nil, rbacCredentialsResource, levels, rbacCredentialsAssertions(credentialId))

	return r.TestResult
}
