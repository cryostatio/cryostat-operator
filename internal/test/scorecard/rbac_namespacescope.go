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

// Synthetic Kubernetes resource name backing the NamespacedRBACPermissions test.
const rbacNamespaceScopeResource = "cryostatnamespacescope"

// rbacNamespaceScopeLevels returns the two ServiceAccounts this test compares: one granted
// the resource under test through an ordinary namespaced Role, one through a ClusterRole.
// CanRead doubles here as "is cluster-bound, and therefore expected to pass."
func rbacNamespaceScopeLevels() []*rbacPrivilegeLevel {
	return []*rbacPrivilegeLevel{
		{Name: "namespaced", CanRead: false},
		{Name: "clusterbound", CanRead: true},
	}
}

// rbacNamespaceScopeRBACPermissions maps archivedrecordings:read onto the synthetic resource
// under test.
func rbacNamespaceScopeRBACPermissions() map[string]string {
	return map[string]string{
		rbacArchivedRecordingsReadPermission: rbacNamespaceScopeResource + ":get",
	}
}

// newRBACNamespaceScopeCryostatCR builds the CR for this test: identical to the standard RBAC
// CR, except NamespacedRBACPermissions is explicitly false, so Cryostat's SSARs omit the
// namespace field and only cluster-scoped Kubernetes bindings are honored.
func (r *TestResources) newRBACNamespaceScopeCryostatCR() *operatorv1beta2.Cryostat {
	cr := r.newRBACCryostatCR(rbacNamespaceScopeRBACPermissions())
	clusterScoped := false
	cr.Spec.AuthorizationOptions.NamespacedRBACPermissions = &clusterScoped
	return cr
}

// setupRBACNamespaceScopeSubjects creates the shared entry-gate Role plus a ServiceAccount per
// level, granted the resource under test through a namespaced Role ("namespaced") or a
// ClusterRole ("clusterbound") - unlike setupRBACSubjects, which only ever creates namespaced
// Roles, since this test's whole point is to compare the two binding shapes against a
// cluster-scoped SSAR.
func (r *TestResources) setupRBACNamespaceScopeSubjects(ctx context.Context, levels []*rbacPrivilegeLevel) error {
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

		rule := rbacv1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{rbacNamespaceScopeResource},
			Verbs:     []string{"get"},
		}

		if level.CanRead {
			clusterRole := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Rules:      []rbacv1.PolicyRule{rule},
			}
			if _, err := r.Client.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create ClusterRole %s: %w", name, err)
			}
			clusterBinding := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Subjects: []rbacv1.Subject{
					{Kind: rbacv1.ServiceAccountKind, Name: name, Namespace: r.Namespace},
				},
				RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
			}
			if _, err := r.Client.RbacV1().ClusterRoleBindings().Create(ctx, clusterBinding, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("failed to create ClusterRoleBinding %s: %w", name, err)
			}
		} else {
			role := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace},
				Rules:      []rbacv1.PolicyRule{rule},
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
		}

		entryBindingName := name + "-entry"
		entryBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      entryBindingName,
				Namespace: r.Namespace,
			},
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
		level.Token = token

		r.Log += fmt.Sprintf("created ServiceAccount %s, cluster-bound=%t\n", name, level.CanRead)
	}

	return nil
}

// cleanupRBACNamespaceScopeSubjects deletes the ServiceAccounts, Roles/ClusterRoles and
// RoleBindings/ClusterRoleBindings created by setupRBACNamespaceScopeSubjects. Failures are
// logged rather than returned: cleanup runs from a deferred call and must not mask the test's
// own result.
func (r *TestResources) cleanupRBACNamespaceScopeSubjects(levels []*rbacPrivilegeLevel) {
	ctx := context.Background()

	for _, level := range levels {
		name := level.ServiceAccountName()
		if err := r.Client.RbacV1().RoleBindings(r.Namespace).Delete(ctx, name+"-entry", metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete RoleBinding %s: %s\n", name+"-entry", err.Error())
		}
		if level.CanRead {
			if err := r.Client.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete ClusterRoleBinding %s: %s\n", name, err.Error())
			}
			if err := r.Client.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete ClusterRole %s: %s\n", name, err.Error())
			}
		} else {
			if err := r.Client.RbacV1().RoleBindings(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete RoleBinding %s: %s\n", name, err.Error())
			}
			if err := r.Client.RbacV1().Roles(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete Role %s: %s\n", name, err.Error())
			}
		}
		if err := r.Client.CoreV1().ServiceAccounts(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete ServiceAccount %s: %s\n", name, err.Error())
		}
	}

	if err := r.Client.RbacV1().Roles(r.Namespace).Delete(ctx, rbacEntryRoleName, metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
		r.Log += fmt.Sprintf("failed to delete Role %s: %s\n", rbacEntryRoleName, err.Error())
	}
}

// rbacNamespaceScopeAssertions checks archivedrecordings:read against the resource under test.
func rbacNamespaceScopeAssertions() []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			header := make(http.Header)
			header.Set("Accept", "*/*")
			r.checkREST(ctx, level, level.CanRead, rbacArchivedRecordingsReadPermission, http.MethodGet,
				base.JoinPath("/api/v4/recordings"), nil, header)
		},
	}
}

// CryostatRBACNamespaceScopeTest checks that NamespacedRBACPermissions: false makes Cryostat's
// SelfSubjectAccessReviews cluster-scoped: a ServiceAccount granted the mapped permission only
// through an ordinary namespaced RoleBinding - the shape every other RBAC test in this suite
// relies on - is denied, while the identical grant made through a ClusterRoleBinding is
// allowed.
//
// The test is a no-op on Kubernetes: see rbacSkipReason.
func CryostatRBACNamespaceScopeTest(bundle *apimanifests.Bundle, namespace string, openShiftCertManager bool) *scapiv1alpha3.TestResult {
	r := newTestResources(CryostatRBACNamespaceScopeTestName, namespace)

	err := r.setupCRTestResources(openShiftCertManager)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up %s test: %s", CryostatRBACNamespaceScopeTestName, err.Error()))
	}
	if !r.OpenShift {
		r.Log += rbacSkipReason(CryostatRBACNamespaceScopeTestName) + "\n"
		return r.TestResult
	}
	defer r.cleanupAndLogs()

	setupCtx, cancelSetup := context.WithTimeout(context.Background(), testTimeout)
	defer cancelSetup()

	levels := rbacNamespaceScopeLevels()
	defer r.cleanupRBACNamespaceScopeSubjects(levels)
	err = r.setupRBACNamespaceScopeSubjects(setupCtx, levels)
	if err != nil {
		return r.fail(fmt.Sprintf("failed to set up RBAC subjects: %s", err.Error()))
	}

	cr, err := r.createAndWaitTillCryostatAvailable(r.newRBACNamespaceScopeCryostatCR())
	if err != nil {
		return r.fail(fmt.Sprintf("%s test failed: %s", CryostatRBACNamespaceScopeTestName, err.Error()))
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

	r.assertRBACMatrix(ctx, base, nil, rbacNamespaceScopeResource, levels, rbacNamespaceScopeAssertions())

	return r.TestResult
}
