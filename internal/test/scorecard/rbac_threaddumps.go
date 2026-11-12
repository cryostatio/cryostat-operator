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
	"time"

	"github.com/cryostatio/cryostat-operator/internal/cryostatapi"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// The Cryostat permission names the threaddumps ladder test asserts, beyond
// rbacThreadDumpsReadPermission (declared in rbac_isolation.go).
const (
	rbacThreadDumpsWritePermission  = "threaddumps:write"
	rbacThreadDumpsDeletePermission = "threaddumps:delete"
)

// rbacThreadDumpsLevels returns the threaddumps privilege ladder.
func rbacThreadDumpsLevels() []*rbacPrivilegeLevel {
	return newRBACPrivilegeLadder(rbacLadderVerbs{Read: "get", Write: "update", Delete: "delete"})
}

// rbacThreadDumpsRBACPermissions maps threaddumps and the supporting targets:read permission
// onto their synthetic resources.
func rbacThreadDumpsRBACPermissions() map[string]string {
	return map[string]string{
		rbacThreadDumpsReadPermission:   rbacThreadDumpsResource + ":get",
		rbacThreadDumpsWritePermission:  rbacThreadDumpsResource + ":update",
		rbacThreadDumpsDeletePermission: rbacThreadDumpsResource + ":delete",
		"targets:read":                  rbacTargetsResource + ":get",
	}
}

// rbacThreadDumpsSupportingRules grants targets:read to every level. Diagnostics.java pairs
// targets:read with the threaddumps permission under test on every target-scoped endpoint, so
// without it every level would be denied regardless of its threaddumps verbs.
func rbacThreadDumpsSupportingRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{rbacTargetsResource},
			Verbs:     []string{"get"},
		},
	}
}

// listFsThreadDumps runs the filesystem-level thread dump listing as the scorecard pod's own
// ServiceAccount.
func (r *TestResources) listFsThreadDumps(ctx context.Context, base *url.URL) ([]cryostatapi.ArchivedThreadDumpDirectory, error) {
	var dirs []cryostatapi.ArchivedThreadDumpDirectory
	if err := r.sendPrivilegedJSON(ctx, http.MethodGet, base.JoinPath("/api/beta/diagnostics/fs/threaddumps"), nil, &dirs); err != nil {
		return nil, err
	}
	return dirs, nil
}

// setupRBACThreadDumpFixture requests one thread dump for the target as the scorecard pod's
// own ServiceAccount, then polls the filesystem listing until it appears, for the ladder's
// delete assertion to act on.
//
// The write assertion does not need a pre-existing thread dump of its own: it exercises
// threaddumps:write by requesting a new one each time, the same way archivedrecordings:write
// is exercised through a PATCH that performs a write rather than a read-back.
func (r *TestResources) setupRBACThreadDumpFixture(ctx context.Context, base *url.URL, target *cryostatapi.Target) (jvmId string, threadDumpId string, err error) {
	jobBody := ""
	if err := r.sendPrivilegedJSON(ctx, http.MethodPost,
		base.JoinPath(fmt.Sprintf("/api/beta/diagnostics/targets/%d/threaddump", target.Id)), &jobBody, nil); err != nil {
		return "", "", fmt.Errorf("failed to request a thread dump: %w", err)
	}

	var dirs []cryostatapi.ArchivedThreadDumpDirectory
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		found, err := r.listFsThreadDumps(ctx, base)
		if err != nil {
			return false, err
		}
		dirs = found
		for _, dir := range dirs {
			if len(dir.ThreadDumps) > 0 {
				return true, nil
			}
		}
		r.Log += "waiting for a thread dump to appear\n"
		return false, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to produce a thread dump: %w", err)
	}
	for _, dir := range dirs {
		if len(dir.ThreadDumps) > 0 {
			r.Log += fmt.Sprintf("created a thread dump: %+v\n", dir.ThreadDumps[0])
			return dir.JvmId, dir.ThreadDumps[0].ThreadDumpId, nil
		}
	}
	return "", "", fmt.Errorf("no thread dump found after a successful poll")
}

// rbacThreadDumpsAssertions returns the threaddumps privilege matrix. There is a GraphQL
// surface for thread dumps, but this ladder is REST-only: the REST surface alone already
// proves the permission is declared and enforced, and the archivedrecordings ladder already
// demonstrates the cross-surface pattern once.
func rbacThreadDumpsAssertions(target *cryostatapi.Target, jvmId string, threadDumpId string) []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacThreadDumpsReadPermission, http.MethodGet,
				base.JoinPath("/api/beta/diagnostics/fs/threaddumps"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			body := ""
			r.checkREST(ctx, level, level.CanWrite, rbacThreadDumpsWritePermission, http.MethodPost,
				base.JoinPath(fmt.Sprintf("/api/beta/diagnostics/targets/%d/threaddump", target.Id)), &body, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanDelete, rbacThreadDumpsDeletePermission, http.MethodDelete,
				base.JoinPath("/api/beta/diagnostics/fs/threaddumps", jvmId, threadDumpId), nil, header)
		},
	}
}

// CryostatRBACThreadDumpsTest checks that the fine-grained RBAC permission mappings for
// Cryostat's thread dumps (threaddumps:read/write/delete) are enforced by the deployed
// Cryostat application - the same property CryostatRBACArchivedRecordingsTest checks for archivedrecordings,
// applied to a different resource.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACThreadDumpsTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACThreadDumpsTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACThreadDumpsTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACThreadDumpsTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacThreadDumpsLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacThreadDumpsResource, rbacThreadDumpsSupportingRules(), levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacThreadDumpsRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACThreadDumpsTestName, err.Error()))
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
		return r.fail(fmt.Sprintf("failed to set up a target for %s test: %s", CryostatRBACThreadDumpsTestName, err.Error()))
	}
	defer r.logApplication(deployment.Name)

	jvmId, threadDumpId, err := r.setupRBACThreadDumpFixture(ctx, base, target)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up a thread dump fixture for %s test: %s", CryostatRBACThreadDumpsTestName, err.Error()))
	}

	r.assertRBACMatrix(ctx, base, nil, rbacThreadDumpsResource, levels, rbacThreadDumpsAssertions(target, jvmId, threadDumpId))

	return r.TestResult
}
