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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Synthetic Kubernetes resource names in the core ("") API group, used as the targets of
// Cryostat's fine-grained RBAC permission mappings.
//
// Synthetic names are used rather than real resources so that the fine-grained permission
// checks stay independent of the auth proxy's default entry-gate access review, which is
// "get pods" in the install namespace. Mapping to pods:get would make the entry grant
// silently imply archivedrecordings:read.
const (
	rbacArchivesResource   = "cryostatarchives"
	rbacRecordingsResource = "cryostatrecordings"
	rbacTargetsResource    = "cryostattargets"
	// rbacDenyResource backs the deny-by-default catch-all. It is granted to the scorecard
	// pod's own ServiceAccount, which performs the privileged setup, and to none of the
	// test ServiceAccounts, so every Cryostat permission the test does not map explicitly
	// is denied to every test ServiceAccount.
	rbacDenyResource = "cryostatnothing"
)

const (
	// rbacSubjectPrefix prefixes the ServiceAccount, Role and RoleBinding names the test
	// creates. It is distinct from the Cryostat CR's name so that none of these collide
	// with resources the operator creates for the CR.
	rbacSubjectPrefix = "scorecard-rbac"
	// rbacEntryRoleName is the shared Role granting the auth proxy's entry-gate
	// permission. Every test ServiceAccount is bound to it; without it the proxy rejects
	// the request with 401 and Cryostat never evaluates a fine-grained permission.
	rbacEntryRoleName = rbacSubjectPrefix + "-entry"
	// rbacTokenExpirationSeconds is the lifetime requested for each test ServiceAccount's
	// token. Long enough to outlive the test, short enough not to leave a usable
	// credential behind if teardown is interrupted.
	rbacTokenExpirationSeconds int64 = 3600
	// rbacArchiveCount is the number of archived recordings created during the privileged
	// setup. Each ServiceAccount allowed to delete consumes one per delete-capable
	// surface (REST and GraphQL); the remainder back the read and write assertions.
	rbacArchiveCount = 4
	// rbacRecordingWarmup is how long the continuous recording is left running before the
	// first archive, so that the archived files have content.
	rbacRecordingWarmup = 10 * time.Second
	// rbacArchiveInterval spaces out the privileged archive operations.
	rbacArchiveInterval = 2 * time.Second
)

// rbacPrivilegeLevel is one rung of the privilege ladder the test exercises.
//
// Every level is granted the same supporting permissions - the auth proxy entry gate,
// targets:read and activerecordings:read/write - so that the verbs held on the synthetic
// cryostatarchives resource are the only variable between levels. Each assertion failure is
// therefore attributable to exactly one archivedrecordings permission rather than to an
// incidental gap in a supporting one.
type rbacPrivilegeLevel struct {
	// Name identifies the level and is the suffix of its ServiceAccount, Role and
	// RoleBinding names.
	Name string
	// ArchiveVerbs are the Kubernetes verbs granted on rbacArchivesResource.
	ArchiveVerbs []string
	// CanRead, CanWrite and CanDelete are the expected outcomes of the corresponding
	// archivedrecordings permission checks on both the REST and GraphQL surfaces.
	CanRead   bool
	CanWrite  bool
	CanDelete bool
	// Token is the ServiceAccount's bearer token, minted during setup.
	Token string
}

// ServiceAccountName returns the name of the level's ServiceAccount.
func (l *rbacPrivilegeLevel) ServiceAccountName() string {
	return fmt.Sprintf("%s-%s", rbacSubjectPrefix, l.Name)
}

// Allows reports whether the level is expected to be granted the given Cryostat permission.
func (l *rbacPrivilegeLevel) Allows(permission string) bool {
	switch permission {
	case rbacReadPermission:
		return l.CanRead
	case rbacWritePermission:
		return l.CanWrite
	case rbacDeletePermission:
		return l.CanDelete
	}
	return false
}

// The Cryostat permission names whose enforcement the test asserts.
const (
	rbacReadPermission   = "archivedrecordings:read"
	rbacWritePermission  = "archivedrecordings:write"
	rbacDeletePermission = "archivedrecordings:delete"
)

// rbacPrivilegeLevels returns the privilege ladder, in ascending order of privilege.
//
// The ladder is monotone: each level holds everything the level below it holds. Cryostat's
// GraphQL mutations for archived recordings (doPutMetadata, doDelete) are resolvers nested
// under the archivedRecordings query, which itself requires archivedrecordings:read, so a
// write-without-read level would be unable to reach them and the GraphQL half of the matrix
// could not be exercised.
//
// The bottom rung holds no archive verbs at all. Without it every level would expect
// archivedrecordings:read to be allowed, and a regression that granted read unconditionally
// would go unnoticed.
func rbacPrivilegeLevels() []*rbacPrivilegeLevel {
	return []*rbacPrivilegeLevel{
		{
			Name:         "none",
			ArchiveVerbs: []string{},
		},
		{
			Name:         "read",
			ArchiveVerbs: []string{"get"},
			CanRead:      true,
		},
		{
			Name:         "write",
			ArchiveVerbs: []string{"get", "update"},
			CanRead:      true,
			CanWrite:     true,
		},
		{
			Name:         "full",
			ArchiveVerbs: []string{"get", "update", "delete"},
			CanRead:      true,
			CanWrite:     true,
			CanDelete:    true,
		},
	}
}

// newRBACCryostatCR builds the Cryostat CR for the RBAC test.
//
// The mappings point every permission the test depends on at a synthetic resource, and the
// defaults deny everything else. All four default fields must be set: Cryostat resolves a
// permission through the explicit mapping, then the verb-level default, then the global
// catch-all, so setting only the catch-all would leave the application's own verb-level
// defaults (pods/exec:create) in effect and the catch-all unreachable.
func (r *TestResources) newRBACCryostatCR() *operatorv1beta2.Cryostat {
	cr := r.newCryostatCR()

	denyAll := rbacDenyResource + ":get"
	noCache := "0s"
	namespaced := true

	cr.Spec.AuthorizationOptions = &operatorv1beta2.AuthorizationOptions{
		// Scope Cryostat's access reviews to the install namespace, so a Role and
		// RoleBinding there are sufficient and no cluster-scoped grants are needed.
		NamespacedRBACPermissions: &namespaced,
		RBACPermissions: map[string]string{
			rbacReadPermission:       rbacArchivesResource + ":get",
			rbacWritePermission:      rbacArchivesResource + ":update",
			rbacDeletePermission:     rbacArchivesResource + ":delete",
			"activerecordings:read":  rbacRecordingsResource + ":get",
			"activerecordings:write": rbacRecordingsResource + ":update",
			"targets:read":           rbacTargetsResource + ":get",
		},
		RBACDefaultPermissions: &operatorv1beta2.RBACDefaultPermissions{
			DefaultReadPermission:   &denyAll,
			DefaultWritePermission:  &denyAll,
			DefaultDeletePermission: &denyAll,
			DefaultPermission:       &denyAll,
		},
		// Disable both caches. Otherwise a decision can outlive the RoleBinding that
		// produced it by up to the decision cache TTL, which would let a stale allow mask
		// a regression.
		RBACCacheOptions: &operatorv1beta2.RBACCacheOptions{
			ClientCacheExpireAfterAccess: &noCache,
			DecisionCacheTTL:             &noCache,
		},
	}

	return cr
}

// setupRBACSubjects creates a ServiceAccount, Role and RoleBinding per privilege level plus
// the shared entry-gate Role, then mints a bearer token for each ServiceAccount.
//
// The Roles grant only permissions the scorecard ServiceAccount itself holds, so Kubernetes'
// privilege-escalation prevention is satisfied without granting the scorecard
// ServiceAccount the escalate or bind verbs on roles.
func (r *TestResources) setupRBACSubjects(ctx context.Context, levels []*rbacPrivilegeLevel) error {
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

		// The supporting permissions every level holds, so that the verbs on
		// rbacArchivesResource remain the only variable between levels.
		rules := []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{rbacTargetsResource},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{rbacRecordingsResource},
				Verbs:     []string{"get", "update"},
			},
		}
		// A PolicyRule must name at least one verb, so the bottom rung of the ladder gets
		// no rule for the archives resource rather than one with an empty verb list.
		if len(level.ArchiveVerbs) > 0 {
			rules = append(rules, rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{rbacArchivesResource},
				Verbs:     level.ArchiveVerbs,
			})
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

		r.Log += fmt.Sprintf("created ServiceAccount %s with %s verbs %v\n", name, rbacArchivesResource, level.ArchiveVerbs)
	}

	return nil
}

// requestServiceAccountToken mints a short-lived bearer token for a ServiceAccount using the
// TokenRequest API. No custom audience is requested, so the token is accepted by the auth
// proxy's TokenReview. Legacy ServiceAccount token Secrets are deprecated and are not used.
func (r *TestResources) requestServiceAccountToken(ctx context.Context, saName string) (string, error) {
	expiration := rbacTokenExpirationSeconds
	request := &authnv1.TokenRequest{
		Spec: authnv1.TokenRequestSpec{
			ExpirationSeconds: &expiration,
		},
	}
	response, err := r.Client.CoreV1().ServiceAccounts(r.Namespace).CreateToken(ctx, saName, request, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to request a token for ServiceAccount %s: %w", saName, err)
	}
	if len(response.Status.Token) == 0 {
		return "", fmt.Errorf("TokenRequest for ServiceAccount %s returned an empty token", saName)
	}
	return response.Status.Token, nil
}

// cleanupRBACSubjects deletes the ServiceAccounts, Roles and RoleBindings created by
// setupRBACSubjects. Failures are logged rather than returned: cleanup runs from a deferred
// call and must not mask the test's own result.
func (r *TestResources) cleanupRBACSubjects(levels []*rbacPrivilegeLevel) {
	ctx := context.Background()

	for _, level := range levels {
		name := level.ServiceAccountName()
		for _, bindingName := range []string{name, name + "-entry"} {
			err := r.Client.RbacV1().RoleBindings(r.Namespace).Delete(ctx, bindingName, metav1.DeleteOptions{})
			if err != nil && !kerrors.IsNotFound(err) {
				r.Log += fmt.Sprintf("failed to delete RoleBinding %s: %s\n", bindingName, err.Error())
			}
		}
		err := r.Client.RbacV1().Roles(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete Role %s: %s\n", name, err.Error())
		}
		err = r.Client.CoreV1().ServiceAccounts(r.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			r.Log += fmt.Sprintf("failed to delete ServiceAccount %s: %s\n", name, err.Error())
		}
	}

	err := r.Client.RbacV1().Roles(r.Namespace).Delete(ctx, rbacEntryRoleName, metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		r.Log += fmt.Sprintf("failed to delete Role %s: %s\n", rbacEntryRoleName, err.Error())
	}
}

// rbacFixture is the state the privileged setup produces for the assertions to act on.
type rbacFixture struct {
	// Target is the Cryostat target the live recording belongs to.
	Target *Target
	// Recording is the live continuous recording that the write assertions archive.
	Recording *Recording
	// Archives are the archived recordings available to the delete assertions, in the
	// order they will be consumed.
	Archives []ArchivedRecordingNode
}

// Take removes and returns the next archived recording, for an assertion that is expected to
// delete it. Returns nil when the fixture has been exhausted.
func (f *rbacFixture) Take() *ArchivedRecordingNode {
	if len(f.Archives) == 0 {
		return nil
	}
	archive := f.Archives[0]
	f.Archives = f.Archives[1:]
	return &archive
}

// Peek returns the next archived recording without consuming it, for an assertion that is
// expected to be denied and therefore to leave it in place. Returns nil when the fixture has
// been exhausted.
func (f *rbacFixture) Peek() *ArchivedRecordingNode {
	if len(f.Archives) == 0 {
		return nil
	}
	return &f.Archives[0]
}

// setupRBACFixture performs the privileged part of the test as the scorecard pod's own
// ServiceAccount: it registers a target, starts a continuous recording and archives it
// repeatedly, so that each privilege level has live and archived recordings to act on.
func (r *TestResources) setupRBACFixture(ctx context.Context, base *url.URL) (*rbacFixture, error) {
	apiClient := r.NewCryostatRESTClientset(base)

	target, err := apiClient.Targets().Create(ctx, &Target{
		ConnectUrl: "service:jmx:rmi:///jndi/rmi://localhost:0/jmxrmi",
		Alias:      "rbacTarget",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create a target: %w", err)
	}
	r.Log += fmt.Sprintf("created a custom target: %+v\n", target)

	// Give Cryostat a moment to discover itself
	time.Sleep(2 * time.Second)

	recording, err := apiClient.Recordings().Create(ctx, target, &RecordingCreateOptions{
		RecordingName: "scorecard_rbac_rec",
		Events:        "template=ALL",
		Duration:      0, // Continuous
		ToDisk:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create a recording: %w", err)
	}
	r.Log += fmt.Sprintf("created a recording: %+v\n", recording)

	time.Sleep(rbacRecordingWarmup)

	for i := 0; i < rbacArchiveCount; i++ {
		if _, err := apiClient.Recordings().Archive(ctx, target, recording.RemoteId); err != nil {
			return nil, fmt.Errorf("failed to archive the recording: %w", err)
		}
		time.Sleep(rbacArchiveInterval)
	}

	// Archiving is asynchronous: the PATCH returns a job ID and the file appears later.
	var archives []ArchivedRecordingNode
	err = wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		found, err := r.listArchivedRecordings(ctx, base, "")
		if err != nil {
			return false, err
		}
		archives = found
		if len(archives) < rbacArchiveCount {
			r.Log += fmt.Sprintf("waiting for archived recordings: have %d, want %d\n", len(archives), rbacArchiveCount)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to produce %d archived recordings: %w", rbacArchiveCount, err)
	}
	r.Log += fmt.Sprintf("created %d archived recordings\n", len(archives))

	return &rbacFixture{
		Target:    target,
		Recording: recording,
		Archives:  archives,
	}, nil
}

// listArchivedRecordings runs the top-level archivedRecordings GraphQL query as the
// scorecard pod's own ServiceAccount, optionally filtered to a single recording name.
func (r *TestResources) listArchivedRecordings(ctx context.Context, base *url.URL, name string) ([]ArchivedRecordingNode, error) {
	query := &GraphQLQuery{
		Query: `
			query ArchivedRecordings($name: String) {
				archivedRecordings(filter: { name: $name }) {
					data {
						name
						jvmId
					}
				}
			}
		`,
		Variables: map[string]any{
			"name": nil,
		},
	}
	if len(name) > 0 {
		query.Variables["name"] = name
	}

	response := &ArchivedRecordingsGraphQLResponse{}
	status, body, err := r.sendGraphQLRequest(ctx, base, query, "", response)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("archivedRecordings query failed with status %d: %s", status, body)
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("archivedRecordings query returned errors: %v", response.Messages())
	}
	return response.Data.ArchivedRecordings.Data, nil
}

// sendGraphQLRequest posts a GraphQL query with the given bearer token and decodes the
// response into result. It returns the HTTP status code and the raw response body alongside,
// so that callers can report an unexpected status without a second read of the body.
func (r *TestResources) sendGraphQLRequest(ctx context.Context, base *url.URL, query *GraphQLQuery, token string, result any) (int, string, error) {
	queryJSON, err := query.ToJSON()
	if err != nil {
		return 0, "", fmt.Errorf("failed to construct GraphQL query: %w", err)
	}
	body := string(queryJSON)

	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "*/*")

	resp, err := SendRequestWithToken(ctx, NewHttpClient(), http.MethodPost, base.JoinPath("/api/v4/graphql").String(), &body, header, token)
	if err != nil {
		return 0, "", fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer r.closeStream(resp.Body)

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("failed to read GraphQL response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, string(raw), nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return resp.StatusCode, string(raw), fmt.Errorf("failed to unmarshal GraphQL response %q: %w", raw, err)
	}
	return resp.StatusCode, string(raw), nil
}

// checkREST issues a Cryostat REST request as the given privilege level and records whether
// the response matched the expectation. It returns true when it did.
//
// A denied request is 403. An allowed but malformed one is 400. A 401 is called out
// separately because it means the auth proxy rejected the token before Cryostat evaluated
// any permission, which is a different failure from a permission denial.
func (r *TestResources) checkREST(ctx context.Context, level *rbacPrivilegeLevel, permission string,
	method string, requestURL *url.URL, body *string, header http.Header) bool {
	allowed := level.Allows(permission)
	label := fmt.Sprintf("REST %s %s as %s, expecting %s", method, requestURL.Path, level.Name, rbacExpectation(allowed))

	resp, err := SendRequestWithToken(ctx, NewHttpClient(), method, requestURL.String(), body, header, level.Token)
	if err != nil {
		r.logError(fmt.Sprintf("%s: request failed: %s", label, err.Error()))
		return false
	}
	defer r.closeStream(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		r.logError(fmt.Sprintf("%s: got 401, so the auth proxy rejected the token before Cryostat"+
			" evaluated %s; check that the ServiceAccount passes the proxy's access review", label, permission))
		return false
	}
	if allowed && !StatusOK(resp.StatusCode) {
		r.logError(fmt.Sprintf("%s: got %d, want 2xx; body: %s", label, resp.StatusCode, ReadError(resp)))
		return false
	}
	if !allowed && resp.StatusCode != http.StatusForbidden {
		r.logError(fmt.Sprintf("%s: got %d, want 403; body: %s", label, resp.StatusCode, ReadError(resp)))
		return false
	}

	r.Log += fmt.Sprintf("%s: got %d, as expected\n", label, resp.StatusCode)
	return true
}

// checkGraphQL issues a Cryostat GraphQL request as the given privilege level and records
// whether the response matched the expectation. It returns true when it did.
//
// Unlike the REST surface, a GraphQL permission denial is HTTP 200 with a populated "errors"
// array: Cryostat's GraphQL security interceptor aborts the operation by throwing a
// GraphQLException, which SmallRye renders as an ordinary GraphQL error response. The status
// code therefore carries no authorization signal here, and the assertion is on the presence
// or absence of a "Forbidden" error instead.
func (r *TestResources) checkGraphQL(ctx context.Context, level *rbacPrivilegeLevel, permission string,
	base *url.URL, query *GraphQLQuery, response *ArchivedRecordingsGraphQLResponse) bool {
	allowed := level.Allows(permission)
	label := fmt.Sprintf("GraphQL %s as %s, expecting %s", permission, level.Name, rbacExpectation(allowed))

	status, body, err := r.sendGraphQLRequest(ctx, base, query, level.Token, response)
	if err != nil {
		r.logError(fmt.Sprintf("%s: %s", label, err.Error()))
		return false
	}
	if status == http.StatusUnauthorized {
		r.logError(fmt.Sprintf("%s: got 401, so the auth proxy rejected the token before Cryostat"+
			" evaluated %s; check that the ServiceAccount passes the proxy's access review", label, permission))
		return false
	}
	if status != http.StatusOK {
		r.logError(fmt.Sprintf("%s: got HTTP %d, want 200; GraphQL denials are reported in the"+
			" response body, not the status code; body: %s", label, status, body))
		return false
	}

	denied := response.Denied()
	if allowed && denied {
		r.logError(fmt.Sprintf("%s: denied; errors: %v", label, response.Messages()))
		return false
	}
	if !allowed && !denied {
		r.logError(fmt.Sprintf("%s: not denied; errors: %v, body: %s", label, response.Messages(), body))
		return false
	}
	if allowed && len(response.Errors) > 0 {
		r.logError(fmt.Sprintf("%s: allowed but the response carries errors: %v", label, response.Messages()))
		return false
	}

	r.Log += fmt.Sprintf("%s: %s, as expected\n", label, rbacExpectation(allowed))
	return true
}

// rbacExpectation renders an expected outcome for inclusion in log and error messages.
func rbacExpectation(allowed bool) string {
	if allowed {
		return "allow"
	}
	return "deny"
}

// rbacPutMetadataQuery builds the GraphQL request exercising archivedrecordings:write. The
// doPutMetadata resolver hangs off the ArchivedRecording type, so it is reached through the
// archivedRecordings query; the filter narrows that query to a single recording so that the
// resolver runs exactly once.
func rbacPutMetadataQuery(name string) *GraphQLQuery {
	return &GraphQLQuery{
		Query: `
			query RBACPutMetadata($name: String!, $labels: [Entry_String_StringInput]) {
				archivedRecordings(filter: { name: $name }) {
					data {
						name
						jvmId
						doPutMetadata(metadataInput: { labels: $labels }) {
							name
						}
					}
				}
			}
		`,
		Variables: map[string]any{
			"name": name,
			"labels": []map[string]string{
				{"key": "scorecard-rbac", "value": "true"},
			},
		},
	}
}

// rbacDeleteQuery builds the GraphQL request exercising archivedrecordings:delete. As with
// doPutMetadata, the doDelete resolver is reached through the archivedRecordings query and
// the filter narrows it to the single recording the caller intends to delete.
func rbacDeleteQuery(name string) *GraphQLQuery {
	return &GraphQLQuery{
		Query: `
			query RBACDelete($name: String!) {
				archivedRecordings(filter: { name: $name }) {
					data {
						name
						jvmId
						doDelete {
							name
						}
					}
				}
			}
		`,
		Variables: map[string]any{
			"name": name,
		},
	}
}

// rbacListQuery builds the GraphQL request exercising archivedrecordings:read.
func rbacListQuery() *GraphQLQuery {
	return &GraphQLQuery{
		Query: `
			query RBACList {
				archivedRecordings {
					data {
						name
						jvmId
					}
				}
			}
		`,
	}
}

// assertRBACMatrix runs every assertion in the privilege matrix: each privilege level
// against each of archivedrecordings read, write and delete, on both the REST and GraphQL
// surfaces. Failures are accumulated rather than returned, so that one unexpected outcome
// does not hide the rest of the matrix.
func (r *TestResources) assertRBACMatrix(ctx context.Context, base *url.URL, fixture *rbacFixture, levels []*rbacPrivilegeLevel) {
	for _, level := range levels {
		r.Log += fmt.Sprintf("\n--- privilege level %q (%s verbs %v) ---\n", level.Name, rbacArchivesResource, level.ArchiveVerbs)

		r.assertRESTRead(ctx, base, level)
		r.assertRESTWrite(ctx, base, fixture, level)
		r.assertRESTDelete(ctx, base, fixture, level)

		r.assertGraphQLRead(ctx, base, level)
		r.assertGraphQLWrite(ctx, base, fixture, level)
		r.assertGraphQLDelete(ctx, base, fixture, level)
	}
}

// assertRESTRead checks archivedrecordings:read via GET /api/v4/recordings, which requires
// no other permission.
func (r *TestResources) assertRESTRead(ctx context.Context, base *url.URL, level *rbacPrivilegeLevel) {
	header := make(http.Header)
	header.Set("Accept", "*/*")
	r.checkREST(ctx, level, rbacReadPermission, http.MethodGet, base.JoinPath("/api/v4/recordings"), nil, header)
}

// assertRESTWrite checks archivedrecordings:write via PATCH of a live recording with the
// body "SAVE". The endpoint's declared permissions are targets:read and
// activerecordings:write, both of which every level holds; the save branch of the handler
// then asserts archivedrecordings:write, which is the permission under test.
func (r *TestResources) assertRESTWrite(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	header := make(http.Header)
	header.Set("Content-Type", "text/plain")
	header.Set("Accept", "*/*")
	body := "SAVE"
	requestURL := base.JoinPath(fmt.Sprintf("/api/v4/targets/%d/recordings/%d", fixture.Target.Id, fixture.Recording.RemoteId))
	r.checkREST(ctx, level, rbacWritePermission, http.MethodPatch, requestURL, &body, header)
}

// assertRESTDelete checks archivedrecordings:delete via DELETE of an archived recording. The
// endpoint's other declared permission, targets:read, is held by every level, so the outcome
// turns only on archivedrecordings:delete. A level expected to succeed consumes an archived
// recording from the fixture; a level expected to be denied leaves it in place.
func (r *TestResources) assertRESTDelete(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	var archive *ArchivedRecordingNode
	if level.CanDelete {
		archive = fixture.Take()
	} else {
		archive = fixture.Peek()
	}
	if archive == nil {
		r.logError(fmt.Sprintf("REST %s as %s: no archived recording left to act on", rbacDeletePermission, level.Name))
		return
	}

	header := make(http.Header)
	header.Set("Accept", "*/*")
	requestURL := base.JoinPath("/api/beta/fs/recordings", archive.JvmId, archive.Name)
	r.checkREST(ctx, level, rbacDeletePermission, http.MethodDelete, requestURL, nil, header)
}

// assertGraphQLRead checks archivedrecordings:read via the top-level archivedRecordings
// query.
func (r *TestResources) assertGraphQLRead(ctx context.Context, base *url.URL, level *rbacPrivilegeLevel) {
	r.checkGraphQL(ctx, level, rbacReadPermission, base, rbacListQuery(), &ArchivedRecordingsGraphQLResponse{})
}

// assertGraphQLWrite checks archivedrecordings:write via the doPutMetadata resolver. Updating
// metadata is idempotent and non-destructive, so every level acts on the same archived
// recording.
func (r *TestResources) assertGraphQLWrite(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	archive := fixture.Peek()
	if archive == nil {
		r.logError(fmt.Sprintf("GraphQL %s as %s: no archived recording left to act on", rbacWritePermission, level.Name))
		return
	}
	r.checkGraphQL(ctx, level, rbacWritePermission, base, rbacPutMetadataQuery(archive.Name), &ArchivedRecordingsGraphQLResponse{})
}

// assertGraphQLDelete checks archivedrecordings:delete via the doDelete resolver. A level
// expected to succeed consumes an archived recording from the fixture; a level expected to
// be denied leaves it in place.
func (r *TestResources) assertGraphQLDelete(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	var archive *ArchivedRecordingNode
	if level.CanDelete {
		archive = fixture.Take()
	} else {
		archive = fixture.Peek()
	}
	if archive == nil {
		r.logError(fmt.Sprintf("GraphQL %s as %s: no archived recording left to act on", rbacDeletePermission, level.Name))
		return
	}
	r.checkGraphQL(ctx, level, rbacDeletePermission, base, rbacDeleteQuery(archive.Name), &ArchivedRecordingsGraphQLResponse{})
}
