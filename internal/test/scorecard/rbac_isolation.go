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

	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
)

// Synthetic Kubernetes resource names backing the non-JFR permissions the isolation test
// maps but never grants to the ServiceAccount under test.
const (
	rbacThreadDumpsResource = "cryostatthreaddumps"
	rbacHeapDumpsResource   = "cryostatheapdumps"
	rbacAuditResource       = "cryostataudit"
)

// The Cryostat permission names the isolation test asserts are denied.
const (
	rbacThreadDumpsReadPermission = "threaddumps:read"
	rbacHeapDumpsReadPermission   = "heapdumps:read"
	rbacAuditReadPermission       = "audit:read"
)

// rbacIsolationLevels returns the single ServiceAccount under test: granted full JFR access
// (activerecordings and archivedrecordings read/write/delete, targets:read, via the same
// synthetic archives resource and supporting rules the archivedrecordings ladder test uses)
// and nothing else.
//
// This is not a ladder - there is exactly one level - but it reuses rbacPrivilegeLevel and
// the existing subject-setup/assertion machinery, since the JFR half of the test is
// identical to the archivedrecordings "full" rung.
func rbacIsolationLevels() []*rbacPrivilegeLevel {
	return []*rbacPrivilegeLevel{
		{
			Name:      "jfr-only",
			Verbs:     []string{"get", "update", "delete"},
			CanRead:   true,
			CanWrite:  true,
			CanDelete: true,
		},
	}
}

// rbacIsolationRBACPermissions extends the archivedrecordings mapping with threaddumps:read,
// heapdumps:read and audit:read, each mapped onto its own synthetic resource that the
// "jfr-only" ServiceAccount is never bound to.
//
// Mapping these three explicitly, rather than leaving them to the global catch-all, is
// deliberate: it proves the three permissions are evaluated independently of the JFR grant,
// rather than merely reproducing the catch-all deny that the "none" rung of every ladder test
// already covers.
func rbacIsolationRBACPermissions() map[string]string {
	permissions := archivedRecordingsRBACPermissions()
	permissions[rbacThreadDumpsReadPermission] = rbacThreadDumpsResource + ":get"
	permissions[rbacHeapDumpsReadPermission] = rbacHeapDumpsResource + ":get"
	permissions[rbacAuditReadPermission] = rbacAuditResource + ":get"
	return permissions
}

// rbacIsolationAssertions checks that the "jfr-only" ServiceAccount is denied the three
// non-JFR permissions the CR maps but never grants it. Unlike archivedRecordingsAssertions,
// these do not vary with a level's CanRead/CanWrite/CanDelete fields - the expected outcome
// is always deny, since there is exactly one level and it holds none of the three.
//
// threaddumps:read and heapdumps:read are each checked via a filesystem-level listing
// endpoint (GET .../diagnostics/fs/threaddumps, GET .../diagnostics/fs/heapdumps). Both are
// gated by a single Quarkus @PermissionsAllowed value, so there is no risk of an incidental
// second permission (such as targets:read, which this ServiceAccount does hold) satisfying
// the check instead - unlike the target-scoped thread/heap dump endpoints, whose
// @PermissionsAllowed lists include targets:read alongside the dump permission.
//
// audit:read is checked via GET /api/beta/audit/revisions, one of the three endpoints that
// return real audit metadata (revision number, timestamp, username). That endpoint's
// @PermissionsAllowed lists around nineteen permissions with inclusive = true, which Quarkus
// Security documents as requiring ALL listed permissions to be held (not merely one) - so the
// "jfr-only" ServiceAccount's three JFR permissions are far from sufficient on their own,
// regardless of the explicit audit:read mapping above. The two audit endpoints that do not
// return audit metadata (.../audit/targets/{jvmId}, .../audit/target_lineage/{jvmId}), gated
// by targets:read instead of audit:read, are deliberately not exercised here.
func rbacIsolationAssertions() []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, false, rbacThreadDumpsReadPermission, http.MethodGet,
				base.JoinPath("/api/beta/diagnostics/fs/threaddumps"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, false, rbacHeapDumpsReadPermission, http.MethodGet,
				base.JoinPath("/api/beta/diagnostics/fs/heapdumps"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, false, rbacAuditReadPermission, http.MethodGet,
				base.JoinPath("/api/beta/audit/revisions"), nil, header)
		},
	}
}

// CryostatRBACIsolationTest checks that granting a ServiceAccount full access to JFR
// (activerecordings and archivedrecordings) does not leak into unrelated permission
// families: thread dumps, heap dumps, and the audit log.
//
// This is a different property from the one CryostatRBACArchivedRecordingsTest checks. CryostatRBACArchivedRecordingsTest proves
// that a single resource's privilege ladder works end to end; this test proves that a grant
// on one resource family stays confined to it, which the ladder test's "none" rung cannot
// show by itself, since it never holds any permission to begin with.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACIsolationTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACIsolationTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACIsolationTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACIsolationTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacIsolationLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacArchivesResource, archivedRecordingsSupportingRules(), levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacIsolationRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACIsolationTestName, err.Error()))
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

	fixture, err := r.setupRBACFixture(ctx, cr, base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up recordings for %s test: %s", CryostatRBACIsolationTestName, err.Error()))
	}

	assertions := append(archivedRecordingsAssertions(), rbacIsolationAssertions()...)
	r.assertRBACMatrix(ctx, base, fixture, rbacArchivesResource, levels, assertions)

	return r.TestResult
}
