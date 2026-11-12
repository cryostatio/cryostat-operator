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
)

// Synthetic Kubernetes resource name backing unifiedlogs.
const rbacUnifiedLogsResource = "cryostatunifiedlogs"

// The Cryostat permission names the unifiedlogs ladder test asserts.
const (
	rbacUnifiedLogsReadPermission   = "unifiedlogs:read"
	rbacUnifiedLogsWritePermission  = "unifiedlogs:write"
	rbacUnifiedLogsDeletePermission = "unifiedlogs:delete"
)

// rbacUnifiedLogsLevels returns the unifiedlogs privilege ladder.
func rbacUnifiedLogsLevels() []*rbacPrivilegeLevel {
	return newRBACPrivilegeLadder(rbacLadderVerbs{Read: "get", Write: "update", Delete: "delete"})
}

// rbacUnifiedLogsRBACPermissions maps unifiedlogs onto its synthetic resource.
//
// Unlike thread dumps and heap dumps, the filesystem-level unified log endpoints
// (UnifiedLogs.java's fs/unified-logs read, write and delete) require only the unifiedlogs
// permission itself, with no paired targets:read - so no supporting permission is needed
// here.
func rbacUnifiedLogsRBACPermissions() map[string]string {
	return map[string]string{
		rbacUnifiedLogsReadPermission:   rbacUnifiedLogsResource + ":get",
		rbacUnifiedLogsWritePermission:  rbacUnifiedLogsResource + ":update",
		rbacUnifiedLogsDeletePermission: rbacUnifiedLogsResource + ":delete",
	}
}

// listFsUnifiedLogs runs the filesystem-level unified log listing as the scorecard pod's own
// ServiceAccount.
func (r *TestResources) listFsUnifiedLogs(ctx context.Context, base *url.URL) ([]cryostatapi.ArchivedUnifiedLogDirectory, error) {
	var dirs []cryostatapi.ArchivedUnifiedLogDirectory
	if err := r.sendPrivilegedJSON(ctx, http.MethodGet, base.JoinPath("/api/beta/diagnostics/fs/unified-logs"), nil, &dirs); err != nil {
		return nil, err
	}
	return dirs, nil
}

// setupRBACUnifiedLogFixture enables a unified logging session on the target and immediately
// pulls it into storage as the scorecard pod's own ServiceAccount, producing one unified log
// artifact for the ladder's write and delete assertions to act on.
//
// Unlike thread dumps and heap dumps, there is no equivalent of "request a new one" that the
// write assertion could use on its own: enabling and pulling a session is target-scoped and
// requires targets:read alongside unifiedlogs:write, permissions the ladder's test
// ServiceAccounts do not hold. The write assertion instead PATCHes this fixture's metadata
// through the filesystem-level endpoint, which requires only unifiedlogs:write.
func (r *TestResources) setupRBACUnifiedLogFixture(ctx context.Context, base *url.URL, target *cryostatapi.Target) (jvmId string, logId string, err error) {
	enableBody := ""
	enableURL := base.JoinPath(fmt.Sprintf("/api/beta/diagnostics/targets/%d/unified-logging", target.Id))
	enableURL.RawQuery = url.Values{"what": {"gc"}, "decorators": {"time,level"}}.Encode()
	if err := r.sendPrivilegedJSON(ctx, http.MethodPost, enableURL, &enableBody, nil); err != nil {
		return "", "", fmt.Errorf("failed to enable unified logging: %w", err)
	}

	pullBody := ""
	var session cryostatapi.UnifiedLogSession
	pullURL := base.JoinPath(fmt.Sprintf("/api/beta/diagnostics/targets/%d/unified-logging/pull", target.Id))
	if err := r.sendPrivilegedJSON(ctx, http.MethodPost, pullURL, &pullBody, &session); err != nil {
		return "", "", fmt.Errorf("failed to pull a unified log: %w", err)
	}
	r.Log += fmt.Sprintf("created a unified log: %+v\n", session)

	return session.JvmId, session.LogId, nil
}

// rbacUnifiedLogsAssertions returns the unifiedlogs privilege matrix. There is a GraphQL
// surface for unified logs, but this ladder is REST-only: the REST surface alone already
// proves the permission is declared and enforced, and the archivedrecordings ladder already
// demonstrates the cross-surface pattern once.
func rbacUnifiedLogsAssertions(jvmId string, logId string) []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacUnifiedLogsReadPermission, http.MethodGet,
				base.JoinPath("/api/beta/diagnostics/fs/unified-logs"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			header.Set("Accept", "*/*")
			body := `{"labels": {"scorecard-rbac": "true"}}`
			r.checkREST(ctx, level, level.CanWrite, rbacUnifiedLogsWritePermission, http.MethodPatch,
				base.JoinPath("/api/beta/diagnostics/fs/unified-logs", jvmId, logId), &body, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanDelete, rbacUnifiedLogsDeletePermission, http.MethodDelete,
				base.JoinPath("/api/beta/diagnostics/fs/unified-logs", jvmId, logId), nil, header)
		},
	}
}

// CryostatRBACUnifiedLogsTest checks that the fine-grained RBAC permission mappings for
// Cryostat's unified logs (unifiedlogs:read/write/delete) are enforced by the deployed
// Cryostat application - the same property CryostatRBACArchivedRecordingsTest checks for archivedrecordings,
// applied to a different resource.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACUnifiedLogsTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACUnifiedLogsTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACUnifiedLogsTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACUnifiedLogsTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacUnifiedLogsLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacUnifiedLogsResource, nil, levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacUnifiedLogsRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACUnifiedLogsTestName, err.Error()))
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

	deployment, target, err := r.setupRBACAgentTarget(ctx, cr, base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up a target for %s test: %s", CryostatRBACUnifiedLogsTestName, err.Error()))
	}
	defer r.logApplication(deployment.Name)

	jvmId, logId, err := r.setupRBACUnifiedLogFixture(ctx, base, target)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up a unified log fixture for %s test: %s", CryostatRBACUnifiedLogsTestName, err.Error()))
	}

	r.assertRBACMatrix(ctx, base, nil, rbacUnifiedLogsResource, levels, rbacUnifiedLogsAssertions(jvmId, logId))

	return r.TestResult
}
