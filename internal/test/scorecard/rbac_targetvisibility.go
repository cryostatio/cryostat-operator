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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Synthetic Kubernetes resource name backing discoverynodes.
const rbacDiscoveryNodesResource = "cryostatdiscoverynodes"

// The Cryostat permission name this test asserts, beyond targets:read.
const rbacDiscoveryNodesReadPermission = "discoverynodes:read"

// rbacTargetVisibilityLevels returns the two ServiceAccounts this test compares. Unlike the
// resource ladders, this is not a none/read/write/delete progression: it is a single
// read/no-read comparison spanning two Cryostat permissions (targets:read and
// discoverynodes:read) at once, reusing rbacPrivilegeLevel's CanRead field for both.
func rbacTargetVisibilityLevels() []*rbacPrivilegeLevel {
	return []*rbacPrivilegeLevel{
		{Name: "denied", CanRead: false},
		{Name: "allowed", CanRead: true},
	}
}

// rbacTargetVisibilityRBACPermissions maps targets:read and discoverynodes:read onto their
// synthetic resources. Both permissions must be denied to hide a target from every listing
// surface: GET /api/v4/targets is gated by targets:read alone, and GET /api/v4/discovery by
// discoverynodes:read alone - denying only one leaves the target visible through the other.
func rbacTargetVisibilityRBACPermissions() map[string]string {
	return map[string]string{
		"targets:read":                   rbacTargetsResource + ":get",
		rbacDiscoveryNodesReadPermission: rbacDiscoveryNodesResource + ":get",
	}
}

// setupRBACTargetVisibilitySubjects creates the shared entry-gate Role plus a ServiceAccount
// and Role per level. Unlike setupRBACSubjects, a level's Role here grants get on both
// rbacTargetsResource and rbacDiscoveryNodesResource together (or on neither), since this test
// compares visibility through two permissions at once rather than varying one resource's
// verbs.
func (r *TestResources) setupRBACTargetVisibilitySubjects(ctx context.Context, levels []*rbacPrivilegeLevel) error {
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

	for _, level := range levels {
		name := level.ServiceAccountName()

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.Namespace,
			},
		}
		if _, err := r.Client.CoreV1().ServiceAccounts(r.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create ServiceAccount %s: %w", name, err)
		}

		var rules []rbacv1.PolicyRule
		if level.CanRead {
			rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{rbacTargetsResource, rbacDiscoveryNodesResource},
					Verbs:     []string{"get"},
				},
			}
		}

		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.Namespace,
			},
			Rules: rules,
		}
		if _, err := r.Client.RbacV1().Roles(r.Namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create Role %s: %w", name, err)
		}

		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.Namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      name,
					Namespace: r.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     name,
			},
		}
		if _, err := r.Client.RbacV1().RoleBindings(r.Namespace).Create(ctx, binding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create RoleBinding %s: %w", name, err)
		}

		entryBindingName := name + "-entry"
		entryBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      entryBindingName,
				Namespace: r.Namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      name,
					Namespace: r.Namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     rbacEntryRoleName,
			},
		}
		if _, err := r.Client.RbacV1().RoleBindings(r.Namespace).Create(ctx, entryBinding, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create RoleBinding %s: %w", entryBindingName, err)
		}

		token, err := r.requestServiceAccountToken(ctx, name)
		if err != nil {
			return err
		}
		level.Token = token

		r.Log += fmt.Sprintf("created ServiceAccount %s, can see targets=%t\n", name, level.CanRead)
	}

	return nil
}

// rbacTargetVisibilityAssertions returns the two checks this test makes per level: that the
// registered target is visible via GET /api/v4/targets only when targets:read is held, and via
// GET /api/v4/discovery only when discoverynodes:read is held. Both endpoints gate the entire
// listing, not individual entries, so "cannot see the target" and "the endpoint is denied" are
// the same outcome here - there is no per-item filtering to verify separately.
func rbacTargetVisibilityAssertions() []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, "targets:read", http.MethodGet,
				base.JoinPath("/api/v4/targets"), nil, header)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacDiscoveryNodesReadPermission, http.MethodGet,
				base.JoinPath("/api/v4/discovery"), nil, header)
		},
	}
}

// CryostatRBACTargetVisibilityTest checks that a ServiceAccount denied both targets:read and
// discoverynodes:read cannot see a real, registered target through either listing endpoint
// Cryostat exposes for it - GET /api/v4/targets and GET /api/v4/discovery - which are gated by
// those two permissions independently, not by a single shared one.
//
// This matters because the two listing surfaces look like they should be interchangeable
// (both enumerate the same underlying targets) but are authorized by different permissions;
// a ServiceAccount missing only one of the two would still see the target through the other.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACTargetVisibilityTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACTargetVisibilityTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACTargetVisibilityTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACTargetVisibilityTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacTargetVisibilityLevels()
	defer r.cleanupRBACSubjects(levels)
	err = r.setupRBACTargetVisibilitySubjects(setupCtx, levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACCryostatCR(rbacTargetVisibilityRBACPermissions()))
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACTargetVisibilityTestName, err.Error()))
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

	deployment, _, err := r.setupRBACAgentTarget(ctx, cr, base)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up a target for %s test: %s", CryostatRBACTargetVisibilityTestName, err.Error()))
	}
	defer r.logApplication(deployment.Name)

	r.assertRBACMatrix(ctx, base, nil, rbacTargetsResource, levels, rbacTargetVisibilityAssertions())

	return r.TestResult
}
