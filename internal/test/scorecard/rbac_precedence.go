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

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	apimanifests "github.com/operator-framework/api/pkg/manifests"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Synthetic Kubernetes resources backing the three-tier permission resolution precedence test:
// an explicit mapping target (C), a verb-level default target (A), and the global catch-all
// target (B). Distinct resources let each tier's grant be held independently, so which tier
// actually resolved a permission is unambiguous from which resource the test maps it to.
const (
	rbacPrecedenceExplicitResource    = "cryostatprecedenceexplicit"
	rbacPrecedenceVerbDefaultResource = "cryostatprecedenceverbdefault"
	rbacPrecedenceCatchAllResource    = "cryostatprecedencecatchall"
)

// rbacPrecedenceSubject is one ServiceAccount in the precedence test: granted "get" on exactly
// one of the three synthetic resources, with the expected outcome of each of the two
// permissions under test recorded alongside.
type rbacPrecedenceSubject struct {
	// Name identifies the subject and is the suffix of its ServiceAccount, Role and
	// RoleBinding names.
	Name string
	// Resource is the single synthetic resource this subject's Role grants "get" on.
	Resource string
	// ExpectArchivedRead is the expected outcome of archivedrecordings:read, which this CR
	// maps explicitly onto rbacPrecedenceExplicitResource.
	ExpectArchivedRead bool
	// ExpectThreadDumpRead is the expected outcome of threaddumps:read, which this CR
	// deliberately leaves unmapped, so it falls to the verb-level default and then the
	// global catch-all.
	ExpectThreadDumpRead bool
	// Token is the ServiceAccount's bearer token, minted during setup.
	Token string
}

// rbacPrecedenceSubjects returns the three subjects this test compares.
//
// "explicit" holds only the explicit-mapping resource: it proves an explicit mapping is used
// regardless of the defaults, and that holding a default's resource has no bearing on a
// permission that has its own explicit mapping.
//
// "verbdefault" holds only the verb-level default resource, not the catch-all: it proves the
// verb-level default alone is sufficient to resolve an unmapped permission.
//
// "catchallonly" holds only the global catch-all resource, not the verb-level default: it
// proves the opposite of what might be a regression - that the verb-level default, once set,
// is authoritative and is NOT skipped in favor of a catch-all the subject happens to hold.
func rbacPrecedenceSubjects() []*rbacPrecedenceSubject {
	return []*rbacPrecedenceSubject{
		{
			Name:                 "explicit",
			Resource:             rbacPrecedenceExplicitResource,
			ExpectArchivedRead:   true,
			ExpectThreadDumpRead: false,
		},
		{
			Name:                 "verbdefault",
			Resource:             rbacPrecedenceVerbDefaultResource,
			ExpectArchivedRead:   false,
			ExpectThreadDumpRead: true,
		},
		{
			Name:                 "catchallonly",
			Resource:             rbacPrecedenceCatchAllResource,
			ExpectArchivedRead:   false,
			ExpectThreadDumpRead: false,
		},
	}
}

// newRBACPrecedenceCryostatCR builds the CR for this test. Unlike newRBACCryostatCR, the four
// default-permission fields are NOT all set to the same value: DefaultReadPermission points at
// a different synthetic resource than DefaultPermission, so the two tiers can be told apart.
// Write and delete defaults are set to the catch-all only because the CRD requires all four
// fields to avoid falling back to Cryostat's own built-in defaults (pods/exec:create) - this
// test asserts nothing about them.
func (r *TestResources) newRBACPrecedenceCryostatCR() *operatorv1beta2.Cryostat {
	cr := r.newCryostatCR()

	verbDefault := rbacPrecedenceVerbDefaultResource + ":get"
	catchAll := rbacPrecedenceCatchAllResource + ":get"
	noCache := "0s"
	namespaced := true

	cr.Spec.AuthorizationOptions = &operatorv1beta2.AuthorizationOptions{
		NamespacedRBACPermissions: &namespaced,
		RBACPermissions: map[string]string{
			// archivedrecordings:read is explicitly mapped; threaddumps:read is
			// deliberately left unmapped, so it falls through to the defaults below.
			rbacArchivedRecordingsReadPermission: rbacPrecedenceExplicitResource + ":get",
		},
		RBACDefaultPermissions: &operatorv1beta2.RBACDefaultPermissions{
			DefaultReadPermission:   &verbDefault,
			DefaultWritePermission:  &catchAll,
			DefaultDeletePermission: &catchAll,
			DefaultPermission:       &catchAll,
		},
		RBACCacheOptions: &operatorv1beta2.RBACCacheOptions{
			ClientCacheExpireAfterAccess: &noCache,
			DecisionCacheTTL:             &noCache,
		},
	}

	return cr
}

// setupRBACPrecedenceSubjects creates the shared entry-gate Role plus a ServiceAccount and
// single-resource Role per subject.
func (r *TestResources) setupRBACPrecedenceSubjects(ctx context.Context, subjects []*rbacPrecedenceSubject) error {
	entryRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacEntryRoleName,
			Namespace: r.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get"},
			},
		},
	}
	if _, err := r.Client.RbacV1().Roles(r.Namespace).Create(ctx, entryRole, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create entry Role %s: %w", rbacEntryRoleName, err)
	}
	r.Log += fmt.Sprintf("created entry Role %s granting the auth proxy's \"get pods\" access review\n", rbacEntryRoleName)

	for _, subject := range subjects {
		name := fmt.Sprintf("%s-%s", rbacSubjectPrefix, subject.Name)

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace},
		}
		if _, err := r.Client.CoreV1().ServiceAccounts(r.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create ServiceAccount %s: %w", name, err)
		}

		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{subject.Resource},
					Verbs:     []string{"get"},
				},
			},
		}
		if _, err := r.Client.RbacV1().Roles(r.Namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create Role %s: %w", name, err)
		}

		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace},
			Subjects: []rbacv1.Subject{
				{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: r.Namespace},
			},
			RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
		}
		if _, err := r.Client.RbacV1().RoleBindings(r.Namespace).Create(ctx, binding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create RoleBinding %s: %w", name, err)
		}

		entryBindingName := name + "-entry"
		entryBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: entryBindingName, Namespace: r.Namespace},
			Subjects: []rbacv1.Subject{
				{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: r.Namespace},
			},
			RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: rbacEntryRoleName},
		}
		if _, err := r.Client.RbacV1().RoleBindings(r.Namespace).Create(ctx, entryBinding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create RoleBinding %s: %w", entryBindingName, err)
		}

		token, err := r.requestServiceAccountToken(ctx, name)
		if err != nil {
			return err
		}
		subject.Token = token

		r.Log += fmt.Sprintf("created ServiceAccount %s with %s verbs [get]\n", name, subject.Resource)
	}

	return nil
}

// cleanupRBACPrecedenceSubjects deletes the ServiceAccounts, Roles and RoleBindings created by
// setupRBACPrecedenceSubjects. Failures are logged rather than returned: cleanup runs from a
// deferred call and must not mask the test's own result.
func (r *TestResources) cleanupRBACPrecedenceSubjects(subjects []*rbacPrecedenceSubject) {
	ctx := context.Background()

	for _, subject := range subjects {
		name := fmt.Sprintf("%s-%s", rbacSubjectPrefix, subject.Name)
		for _, bindingName := range []string{name, name + "-entry"} {
			if err := r.Client.RbacV1().RoleBindings(r.Namespace).Delete(ctx, bindingName, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete RoleBinding %s: %s\n", bindingName, err.Error())
			}
		}
		if err := r.Client.RbacV1().Roles(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete Role %s: %s\n", name, err.Error())
		}
		if err := r.Client.CoreV1().ServiceAccounts(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete ServiceAccount %s: %s\n", name, err.Error())
		}
	}

	if err := r.Client.RbacV1().Roles(r.Namespace).Delete(ctx, rbacEntryRoleName, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
		r.Log += fmt.Sprintf("failed to delete Role %s: %s\n", rbacEntryRoleName, err.Error())
	}
}

// assertRBACPrecedence checks both permissions under test for every subject. archivedrecordings
// is checked via GET /api/v4/recordings (its explicit mapping target); threaddumps via
// GET /api/beta/diagnostics/fs/threaddumps (deliberately unmapped, so it exercises the
// defaults). Neither endpoint needs a target.
func (r *TestResources) assertRBACPrecedence(ctx context.Context, base *url.URL, subjects []*rbacPrecedenceSubject) {
	for _, subject := range subjects {
		level := &rbacPrivilegeLevel{Name: subject.Name, Token: subject.Token}
		r.Log += fmt.Sprintf("\n--- subject %q (holds %s) ---\n", subject.Name, subject.Resource)

		header := make(http.Header)
		header.Set("Accept", "*/*")
		r.checkREST(ctx, level, subject.ExpectArchivedRead, rbacArchivedRecordingsReadPermission, http.MethodGet,
			base.JoinPath("/api/v4/recordings"), nil, header)

		header = make(http.Header)
		header.Set("Accept", "*/*")
		r.checkREST(ctx, level, subject.ExpectThreadDumpRead, rbacThreadDumpsReadPermission, http.MethodGet,
			base.JoinPath("/api/beta/diagnostics/fs/threaddumps"), nil, header)
	}
}

// CryostatRBACPrecedenceTest checks that Cryostat resolves a permission through its three
// tiers in order, and that each tier is authoritative once matched: an explicit mapping is
// used regardless of the defaults; an unmapped permission falls to the verb-level default; and
// the verb-level default, once set, is NOT skipped in favor of the global catch-all even when
// a subject holds the catch-all's resource but not the verb-level default's.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACPrecedenceTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACPrecedenceTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACPrecedenceTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACPrecedenceTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	subjects := rbacPrecedenceSubjects()
	defer r.cleanupRBACPrecedenceSubjects(subjects)
	err = r.setupRBACPrecedenceSubjects(setupCtx, subjects)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACPrecedenceCryostatCR())
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACPrecedenceTestName, err.Error()))
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

	r.assertRBACPrecedence(ctx, base, subjects)

	return r.TestResult
}
