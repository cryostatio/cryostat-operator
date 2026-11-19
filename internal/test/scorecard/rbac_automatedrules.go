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

// Synthetic Kubernetes resource name backing automatedrules and matchexpressions.
const rbacAutomatedRulesResource = "cryostatautomatedrules"

// The Cryostat permission names the automatedrules ladder test asserts.
const (
	rbacAutomatedRulesReadPermission     = "automatedrules:read"
	rbacAutomatedRulesWritePermission    = "automatedrules:write"
	rbacAutomatedRulesDeletePermission   = "automatedrules:delete"
	rbacMatchExpressionsWritePermission  = "matchexpressions:write"
	rbacMatchExpressionsDeletePermission = "matchexpressions:delete"
)

// rbacAutomatedRuleName is the name of the single Automated Rule the fixture creates.
const rbacAutomatedRuleName = "scorecard-rbac-rule"

// rbacAutomatedRulesLevels returns the automatedrules privilege ladder.
func rbacAutomatedRulesLevels() []*rbacPrivilegeLevel {
	return newRBACPrivilegeLadder(rbacLadderVerbs{Read: "get", Write: "update", Delete: "delete"})
}

// rbacAutomatedRulesRBACPermissions maps automatedrules and matchexpressions permissions onto
// the same synthetic resource and verb pair.
//
// Rules.java's create/update/delete endpoints each require matchexpressions:write or
// :delete alongside the corresponding automatedrules permission (creating or removing a rule
// also touches its match expression). Mapping both Cryostat permissions onto one synthetic
// resource means a single Kubernetes grant satisfies both checks, without tracking a second
// resource through the ladder.
func rbacAutomatedRulesRBACPermissions() map[string]string {
	return map[string]string{
		rbacAutomatedRulesReadPermission:     rbacAutomatedRulesResource + ":get",
		rbacAutomatedRulesWritePermission:    rbacAutomatedRulesResource + ":update",
		rbacAutomatedRulesDeletePermission:   rbacAutomatedRulesResource + ":delete",
		rbacMatchExpressionsWritePermission:  rbacAutomatedRulesResource + ":update",
		rbacMatchExpressionsDeletePermission: rbacAutomatedRulesResource + ":delete",
	}
}

// setupRBACAutomatedRuleFixture creates a single, disabled Automated Rule as the scorecard
// pod's own ServiceAccount, for the ladder's write and delete assertions to act on.
//
// The rule is created disabled, with a match expression that never matches, so it never
// actually fires against any target; it exists only as something for the write and delete
// assertions to PATCH and DELETE. One rule serves both: PATCH is non-destructive and every
// ladder level runs its write assertion before its delete assertion, so by the time the last
// ("full") level's delete assertion succeeds and consumes the rule, every other level has
// already finished using it.
func (r *TestResources) setupRBACAutomatedRuleFixture(ctx context.Context, base *url.URL) error {
	body := fmt.Sprintf(`{
		"name": %q,
		"description": "",
		"matchExpression": {"script": "false"},
		"eventSpecifier": "template=ALL",
		"archivalPeriodSeconds": 60,
		"initialDelaySeconds": 0,
		"preservedArchives": 1,
		"maxAgeSeconds": 0,
		"maxSizeBytes": 0,
		"enabled": false
	}`, rbacAutomatedRuleName)
	if err := r.sendPrivilegedJSON(ctx, http.MethodPost, base.JoinPath("/api/v4/rules"), &body, nil); err != nil {
		return fmt.Errorf("failed to create Automated Rule fixture: %w", err)
	}
	r.Log += fmt.Sprintf("created Automated Rule %s\n", rbacAutomatedRuleName)
	return nil
}

// rbacAutomatedRulesAssertions returns the automatedrules privilege matrix. There is no
// GraphQL surface for Automated Rules, so this is REST-only.
func rbacAutomatedRulesAssertions() []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacAutomatedRulesReadPermission, http.MethodGet,
				base.JoinPath("/api/v4/rules"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			header.Set("Accept", "*/*")
			body := `{"description": "scorecard-rbac"}`
			r.checkREST(ctx, level, level.CanWrite, rbacAutomatedRulesWritePermission, http.MethodPatch,
				base.JoinPath("/api/v4/rules", rbacAutomatedRuleName), &body, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanDelete, rbacAutomatedRulesDeletePermission, http.MethodDelete,
				base.JoinPath("/api/v4/rules", rbacAutomatedRuleName), nil, header)
		},
	}
}

// CryostatRBACAutomatedRulesTest checks that the fine-grained RBAC permission mappings for
// Cryostat's Automated Rules (automatedrules:read/write/delete) are enforced by the deployed
// Cryostat application - the same property CryostatRBACArchivedRecordingsTest checks for archivedrecordings,
// applied to a different resource, to prove Automated Rules' endpoints declare and enforce
// the permissions they are supposed to.
//
// Unlike the archivedrecordings and diagnostics ladders, this one needs no target: Automated
// Rules are defined independently of any connected JVM.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACAutomatedRulesTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACAutomatedRulesTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACAutomatedRulesTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACAutomatedRulesTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacAutomatedRulesLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacAutomatedRulesResource, nil, levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacAutomatedRulesRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACAutomatedRulesTestName, err.Error()))
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

	if err := r.setupRBACAutomatedRuleFixture(ctx, base); err != nil {
		return r.fail(fmt.Sprintf("failed to set up the Automated Rule fixture for %s test: %s", CryostatRBACAutomatedRulesTestName, err.Error()))
	}

	r.assertRBACMatrix(ctx, base, nil, rbacAutomatedRulesResource, levels, rbacAutomatedRulesAssertions())

	return r.TestResult
}
