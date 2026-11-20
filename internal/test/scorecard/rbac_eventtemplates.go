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
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"

	"github.com/cryostatio/cryostat-operator/internal/cryostatapi"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
)

// Synthetic Kubernetes resource name backing eventtemplates.
const rbacEventTemplatesResource = "cryostateventtemplates"

// The Cryostat permission names the eventtemplates ladder test asserts.
const (
	rbacEventTemplatesReadPermission   = "eventtemplates:read"
	rbacEventTemplatesWritePermission  = "eventtemplates:write"
	rbacEventTemplatesDeletePermission = "eventtemplates:delete"
)

// rbacEventTemplatesLevels returns the eventtemplates privilege ladder.
func rbacEventTemplatesLevels() []*rbacPrivilegeLevel {
	return newRBACPrivilegeLadder(rbacLadderVerbs{Read: "get", Write: "update", Delete: "delete"})
}

// rbacEventTemplatesRBACPermissions maps eventtemplates onto its synthetic resource.
//
// EventTemplates.java's endpoints require only the eventtemplates permission itself, with no
// paired targets:read, so no supporting permission is needed here.
func rbacEventTemplatesRBACPermissions() map[string]string {
	return map[string]string{
		rbacEventTemplatesReadPermission:   rbacEventTemplatesResource + ":get",
		rbacEventTemplatesWritePermission:  rbacEventTemplatesResource + ":update",
		rbacEventTemplatesDeletePermission: rbacEventTemplatesResource + ":delete",
	}
}

// rbacEventTemplateXML is a minimal, valid custom event template document - the same fixture
// Cryostat's own EventTemplatesTest uses (testCreateEmptyAndDelete) to prove an empty template
// round-trips, rather than needing a full realistic .jfc file. label is embedded as both the
// template's name and description, since the create response's "name" field is read back from
// the label.
func rbacEventTemplateXML(label string) string {
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>
<configuration version="2.0" label=%q description="scorecard-rbac fixture" provider="cryostat-operator-scorecard">
</configuration>
`, label)
}

// newRBACEventTemplateUpload encodes label's event template XML as the multipart/form-data
// body EventTemplates.java expects: a single "template" file part.
func newRBACEventTemplateUpload(label string) (string, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("template", label+".xml")
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write([]byte(rbacEventTemplateXML(label))); err != nil {
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	return buf.String(), writer.FormDataContentType(), nil
}

// setupRBACEventTemplateFixture uploads one custom event template as the scorecard pod's own
// ServiceAccount, for the ladder's delete assertion to act on.
//
// The write assertion does not need this pre-existing template: it exercises
// eventtemplates:write by uploading a new, uniquely labelled template each time (event
// template names must be unique), the same way archivedrecordings:write is exercised through
// a PATCH that performs a write rather than a read-back.
func (r *TestResources) setupRBACEventTemplateFixture(ctx context.Context, base *url.URL) (string, error) {
	body, contentType, err := newRBACEventTemplateUpload("ScorecardRBACDeletable")
	if err != nil {
		return "", fmt.Errorf("failed to encode the Event Template fixture upload: %w", err)
	}
	var created cryostatapi.EventTemplate
	if err := r.sendPrivilegedRequest(ctx, http.MethodPost, base.JoinPath("/api/v4/event_templates"),
		contentType, &body, &created); err != nil {
		return "", fmt.Errorf("failed to create an Event Template fixture: %w", err)
	}
	r.Log += fmt.Sprintf("created Event Template %s\n", created.Name)
	return created.Name, nil
}

// rbacEventTemplatesAssertions returns the eventtemplates privilege matrix. There is no
// GraphQL surface for event templates, so this is REST-only.
func rbacEventTemplatesAssertions(deletableName string) []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacEventTemplatesReadPermission, http.MethodGet,
				base.JoinPath("/api/v4/event_templates"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			// A distinct label per level, so a level's own repeated writes across the
			// matrix never collide with another level's or with the deletable fixture.
			body, contentType, err := newRBACEventTemplateUpload("ScorecardRBACWrite" + level.Name)
			if err != nil {
				r.logError(fmt.Sprintf("REST POST api/v4/event_templates as %s: failed to encode upload: %s", level.Name, err.Error()))
				return
			}
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanWrite, rbacEventTemplatesWritePermission, http.MethodPost,
				base.JoinPath("/api/v4/event_templates"), &body, withContentType(header, contentType))
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanDelete, rbacEventTemplatesDeletePermission, http.MethodDelete,
				base.JoinPath("/api/v4/event_templates", deletableName), nil, header)
		},
	}
}

// withContentType sets the Content-Type header on an existing http.Header and returns it, for
// inline use within a single call expression.
func withContentType(header http.Header, contentType string) http.Header {
	header.Set("Content-Type", contentType)
	return header
}

// CryostatRBACEventTemplatesTest checks that the fine-grained RBAC permission mappings for
// Cryostat's custom event templates (eventtemplates:read/write/delete) are enforced by the
// deployed Cryostat application - the same property CryostatRBACArchivedRecordingsTest checks
// for archivedrecordings, applied to a different resource.
//
// Unlike the archivedrecordings and diagnostics ladders, this one needs no target: event
// templates are defined independently of any connected JVM.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACEventTemplatesTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACEventTemplatesTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACEventTemplatesTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACEventTemplatesTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacEventTemplatesLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACSubjects(setupCtx, rbacEventTemplatesResource, nil, levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacEventTemplatesRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACEventTemplatesTestName, err.Error()))
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

	deletableName, err := r.setupRBACEventTemplateFixture(ctx, base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up an Event Template fixture for %s test: %s", CryostatRBACEventTemplatesTestName, err.Error()))
	}

	r.assertRBACMatrix(ctx, base, nil, rbacEventTemplatesResource, levels, rbacEventTemplatesAssertions(deletableName))

	return r.TestResult
}
