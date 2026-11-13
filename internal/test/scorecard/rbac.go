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

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controller/constants"
	"github.com/cryostatio/cryostat-operator/internal/cryostatapi"
	appsv1 "k8s.io/api/apps/v1"
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
	// test ServiceAccounts, so every Cryostat permission a test does not map explicitly is
	// denied to every test ServiceAccount.
	rbacDenyResource = "cryostatnothing"
)

const (
	// rbacSubjectPrefix prefixes the ServiceAccount, Role and RoleBinding names a test
	// creates. It is distinct from the Cryostat CR's name so that none of these collide with
	// resources the operator creates for the CR.
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

// rbacPrivilegeLevel is one rung of a resource's privilege ladder.
//
// Every level is granted the same supporting permissions - at minimum the auth proxy's entry
// gate - so that the verbs held on the resource under test are the only variable between
// levels. Each assertion failure is therefore attributable to exactly one permission rather
// than to an incidental gap in a supporting one.
type rbacPrivilegeLevel struct {
	// Name identifies the level and is the suffix of its ServiceAccount, Role and
	// RoleBinding names.
	Name string
	// Verbs are the Kubernetes verbs granted on the synthetic resource backing the
	// permission under test.
	Verbs []string
	// CanRead, CanWrite and CanDelete are the expected outcomes of the corresponding
	// read/write/delete permission checks on both the REST and GraphQL surfaces.
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

// The Cryostat permission names whose enforcement the archivedrecordings ladder test
// asserts.
const (
	rbacArchivedRecordingsReadPermission   = "archivedrecordings:read"
	rbacArchivedRecordingsWritePermission  = "archivedrecordings:write"
	rbacArchivedRecordingsDeletePermission = "archivedrecordings:delete"
)

// rbacLadderVerbs names the Kubernetes verb that unlocks each rung of a standard
// none/read/write/delete privilege ladder, for a resource using Cryostat's conventional
// get/update/delete verb mapping.
type rbacLadderVerbs struct {
	Read, Write, Delete string
}

// newRBACPrivilegeLadder returns the standard four-rung privilege ladder, in ascending order
// of privilege, for a resource whose Kubernetes verbs are given by verbs.
//
// The ladder is monotone: each level holds everything the level below it holds, so a
// write-without-read level is never constructed - which matters for resources (such as
// archivedrecordings) whose write/delete operations are only reachable through a read-gated
// GraphQL query. The bottom rung holds no verbs at all, so that a regression granting read
// unconditionally does not go unnoticed.
func newRBACPrivilegeLadder(verbs rbacLadderVerbs) []*rbacPrivilegeLevel {
	return []*rbacPrivilegeLevel{
		{
			Name: "none",
		},
		{
			Name:    "read",
			Verbs:   []string{verbs.Read},
			CanRead: true,
		},
		{
			Name:     "write",
			Verbs:    []string{verbs.Read, verbs.Write},
			CanRead:  true,
			CanWrite: true,
		},
		{
			Name:      "full",
			Verbs:     []string{verbs.Read, verbs.Write, verbs.Delete},
			CanRead:   true,
			CanWrite:  true,
			CanDelete: true,
		},
	}
}

// rbacPrivilegeLevels returns the archivedrecordings privilege ladder.
//
// Cryostat's GraphQL mutations for archived recordings (doPutMetadata, doDelete) are
// resolvers nested under the archivedRecordings query, which itself requires
// archivedrecordings:read, so a write-without-read level would be unable to reach them and
// the GraphQL half of the matrix could not be exercised. The standard ladder already keeps
// read cumulative, so this holds without any special-casing here.
func rbacPrivilegeLevels() []*rbacPrivilegeLevel {
	return newRBACPrivilegeLadder(rbacLadderVerbs{Read: "get", Write: "update", Delete: "delete"})
}

// archivedRecordingsRBACPermissions is the explicit permission mapping for the
// archivedrecordings ladder test: every permission the test depends on, mapped onto a
// synthetic resource.
func archivedRecordingsRBACPermissions() map[string]string {
	return map[string]string{
		rbacArchivedRecordingsReadPermission:   rbacArchivesResource + ":get",
		rbacArchivedRecordingsWritePermission:  rbacArchivesResource + ":update",
		rbacArchivedRecordingsDeletePermission: rbacArchivesResource + ":delete",
		"activerecordings:read":                rbacRecordingsResource + ":get",
		"activerecordings:write":               rbacRecordingsResource + ":update",
		"targets:read":                         rbacTargetsResource + ":get",
	}
}

// archivedRecordingsSupportingRules are the Kubernetes rules every privilege level holds
// regardless of ladder position, so that the verbs on rbacArchivesResource remain the only
// variable between levels.
func archivedRecordingsSupportingRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
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
}

// newRBACCryostatCR builds a Cryostat CR configured for an RBAC ladder test: every entry in
// permissions is mapped explicitly, and everything else is denied by default.
//
// All four default fields must be set: Cryostat resolves a permission through the explicit
// mapping, then the verb-level default, then the global catch-all, so setting only the
// catch-all would leave the application's own verb-level defaults (pods/exec:create) in
// effect and the catch-all unreachable.
func (r *TestResources) newRBACCryostatCR(permissions map[string]string) *operatorv1beta2.Cryostat {
	cr := r.newCryostatCR()

	denyAll := rbacDenyResource + ":get"
	noCache := "0s"
	namespaced := true

	cr.Spec.AuthorizationOptions = &operatorv1beta2.AuthorizationOptions{
		// Scope Cryostat's access reviews to the install namespace, so a Role and
		// RoleBinding there are sufficient and no cluster-scoped grants are needed.
		NamespacedRBACPermissions: &namespaced,
		RBACPermissions:           permissions,
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
// supportingRules are granted to every level's Role regardless of ladder position, so that
// the verbs on resource remain the only variable between levels. The Roles grant only
// permissions the scorecard ServiceAccount itself holds, so Kubernetes' privilege-escalation
// prevention is satisfied without granting the scorecard ServiceAccount the escalate or bind
// verbs on roles.
func (r *TestResources) setupRBACSubjects(ctx context.Context, resource string, supportingRules []rbacv1.PolicyRule, levels []*rbacPrivilegeLevel) error {
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

		rules := make([]rbacv1.PolicyRule, len(supportingRules))
		copy(rules, supportingRules)
		// A PolicyRule must name at least one verb, so the bottom rung of the ladder gets no
		// rule for resource rather than one with an empty verb list.
		if len(level.Verbs) > 0 {
			rules = append(rules, rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{resource},
				Verbs:     level.Verbs,
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

		r.Log += fmt.Sprintf("created ServiceAccount %s with %s verbs %v\n", name, resource, level.Verbs)
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

// rbacFixture is the state the privileged setup produces for the archivedrecordings
// assertions to act on.
type rbacFixture struct {
	// Target is the Cryostat target the live recording belongs to.
	Target *cryostatapi.Target
	// Recording is the live continuous recording that the write assertions archive.
	Recording *cryostatapi.Recording
	// Archives are the archived recordings available to the delete assertions, in the
	// order they will be consumed.
	Archives []cryostatapi.ArchivedRecordingNode
}

// Take removes and returns the next archived recording, for an assertion that is expected to
// delete it. Returns nil when the fixture has been exhausted.
func (f *rbacFixture) Take() *cryostatapi.ArchivedRecordingNode {
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
func (f *rbacFixture) Peek() *cryostatapi.ArchivedRecordingNode {
	if len(f.Archives) == 0 {
		return nil
	}
	return &f.Archives[0]
}

// setupRBACAgentTarget deploys a sample application with an auto-configured Agent and waits
// for it to register as a target, as the scorecard pod's own ServiceAccount. It is the shared
// first step of every RBAC fixture that needs a live target: archivedrecordings and the
// diagnostics resources (thread dumps, heap dumps, unified logs) alike.
//
// An Agent target is used rather than a synthetic JMX one so the fixture is discovered
// through real registration, the same path the privilege levels exercise, instead of being
// synthesized through POST /targets. The caller is responsible for deferring
// r.logApplication(deployment.Name) itself, so that it fires when the caller's own setup (not
// just this step) returns.
func (r *TestResources) setupRBACAgentTarget(ctx context.Context, cr *operatorv1beta2.Cryostat, base *url.URL) (*appsv1.Deployment, *cryostatapi.Target, error) {
	apiClient := r.NewCryostatRESTClientset(base)

	deployment := r.newAgentApplication(cr, true)
	deployed, err := r.Client.AppsV1().Deployments(r.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deploy the RBAC fixture's sample application: %w", err)
	}
	for _, key := range []string{constants.AgentLabelCryostatName, constants.AgentLabelCryostatNamespace} {
		if deployed.Spec.Template.Labels[key] != deployment.Labels[key] {
			return nil, nil, fmt.Errorf("Deployment webhook did not propagate %s to the Pod template", key)
		}
	}
	if err := r.waitForDeploymentAvailability(ctx, deployment.Name, r.Namespace); err != nil {
		return nil, nil, fmt.Errorf("RBAC fixture's sample application did not become available: %w", err)
	}

	pods, err := r.Client.CoreV1().Pods(r.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deployment.Name})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list RBAC fixture's sample application Pods: %w", err)
	}
	if len(pods.Items) != 1 {
		return nil, nil, fmt.Errorf("expected one RBAC fixture sample application Pod, got %d", len(pods.Items))
	}
	pod := &pods.Items[0]

	target, err := waitForAgentTarget(ctx, apiClient.Targets(), pod.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("Agent did not register with Cryostat: %w", err)
	}
	r.Log += fmt.Sprintf("Agent registered target: %+v\n", target)

	return deployment, target, nil
}

// setupRBACFixture performs the privileged part of the archivedrecordings test as the
// scorecard pod's own ServiceAccount: on top of setupRBACAgentTarget, it starts a continuous
// recording and archives it repeatedly, so that each privilege level has live and archived
// recordings to act on.
func (r *TestResources) setupRBACFixture(ctx context.Context, cr *operatorv1beta2.Cryostat, base *url.URL) (*rbacFixture, error) {
	apiClient := r.NewCryostatRESTClientset(base)

	deployment, target, err := r.setupRBACAgentTarget(ctx, cr, base)
	if err != nil {
		return nil, err
	}
	defer r.logApplication(deployment.Name)

	recording, err := apiClient.Recordings().Create(ctx, target, &cryostatapi.RecordingCreateOptions{
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
	var archives []cryostatapi.ArchivedRecordingNode
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
func (r *TestResources) listArchivedRecordings(ctx context.Context, base *url.URL, name string) ([]cryostatapi.ArchivedRecordingNode, error) {
	query := &cryostatapi.GraphQLQuery{
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

	response := &cryostatapi.ArchivedRecordingsGraphQLResponse{}
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
func (r *TestResources) sendGraphQLRequest(ctx context.Context, base *url.URL, query *cryostatapi.GraphQLQuery, token string, result any) (int, string, error) {
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

// sendPrivilegedJSON issues a REST request as the scorecard pod's own ServiceAccount and, on
// a 2xx response, decodes the JSON body into result (if result is non-nil). It is used by the
// privileged setup step of the diagnostics and automated rules ladder tests, which have no
// dedicated client type the way archived recordings does.
func (r *TestResources) sendPrivilegedJSON(ctx context.Context, method string, requestURL *url.URL, body *string, result any) error {
	contentType := ""
	if body != nil {
		contentType = "application/json"
	}
	return r.sendPrivilegedRequest(ctx, method, requestURL, contentType, body, result)
}

// sendPrivilegedRequest is sendPrivilegedJSON generalized over the request body's content
// type, for privileged setup steps whose request body isn't JSON (e.g. the form-encoded and
// multipart POSTs credentials and event templates expect).
func (r *TestResources) sendPrivilegedRequest(ctx context.Context, method string, requestURL *url.URL, contentType string, body *string, result any) error {
	header := make(http.Header)
	header.Set("Accept", "*/*")
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}

	resp, err := SendRequestWithToken(ctx, NewHttpClient(), method, requestURL.String(), body, header, "")
	if err != nil {
		return fmt.Errorf("%s %s failed: %w", method, requestURL.Path, err)
	}
	defer r.closeStream(resp.Body)

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read %s %s response body: %w", method, requestURL.Path, err)
	}
	if !StatusOK(resp.StatusCode) {
		return fmt.Errorf("%s %s failed with status %d: %s", method, requestURL.Path, resp.StatusCode, raw)
	}
	if result != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, result); err != nil {
			return fmt.Errorf("failed to unmarshal %s %s response %q: %w", method, requestURL.Path, raw, err)
		}
	}
	return nil
}

// rbacGomegaT adapts TestResources to Gomega's minimal testing interface
// (gomega/types.GomegaTestingT), so a failed Gomega matcher records its failure message via
// r.logError instead of stopping the process the way a real *testing.T's Fatalf would. A
// scorecard test has no *testing.T to begin with, and the RBAC matrix needs every assertion
// to run regardless of earlier ones failing, so the adapter's Fatalf deliberately returns
// rather than halting.
type rbacGomegaT struct {
	r *TestResources
}

func (t *rbacGomegaT) Helper() {}

func (t *rbacGomegaT) Fatalf(format string, args ...any) {
	t.r.logError(fmt.Sprintf(format, args...))
}

var _ types.GomegaTestingT = (*rbacGomegaT)(nil)

// checkREST issues a Cryostat REST request as the given privilege level and asserts, via
// Gomega matchers, that the response matched allowed, the expected outcome for the
// permission under test.
//
// A denied request is 403. An allowed but malformed one is 400. A 401 is called out
// separately because it means the auth proxy rejected the token before Cryostat evaluated
// any permission, which is a different failure from a permission denial.
func (r *TestResources) checkREST(ctx context.Context, level *rbacPrivilegeLevel, allowed bool, permission string,
	method string, requestURL *url.URL, body *string, header http.Header) {
	g := gomega.NewWithT(&rbacGomegaT{r: r})
	label := fmt.Sprintf("REST %s %s as %s, expecting %s", method, requestURL.Path, level.Name, rbacExpectation(allowed))

	resp, err := SendRequestWithToken(ctx, NewHttpClient(), method, requestURL.String(), body, header, level.Token)
	if !g.Expect(err).To(gomega.Succeed(), func() string {
		return fmt.Sprintf("%s: request failed", label)
	}) {
		return
	}
	defer r.closeStream(resp.Body)

	if !g.Expect(resp.StatusCode).ToNot(gomega.Equal(http.StatusUnauthorized), func() string {
		return fmt.Sprintf("%s: got 401, so the auth proxy rejected the token before Cryostat"+
			" evaluated %s; check that the ServiceAccount passes the proxy's access review", label, permission)
	}) {
		return
	}

	if allowed {
		if !g.Expect(StatusOK(resp.StatusCode)).To(gomega.BeTrue(), func() string {
			return fmt.Sprintf("%s: got %d, want 2xx; body: %s", label, resp.StatusCode, ReadError(resp))
		}) {
			return
		}
	} else {
		if !g.Expect(resp.StatusCode).To(gomega.Equal(http.StatusForbidden), func() string {
			return fmt.Sprintf("%s: got %d, want 403; body: %s", label, resp.StatusCode, ReadError(resp))
		}) {
			return
		}
	}

	r.Log += fmt.Sprintf("%s: got %d, as expected\n", label, resp.StatusCode)
}

// checkGraphQL issues a Cryostat GraphQL request as the given privilege level and asserts,
// via Gomega matchers, that the response matched allowed, the expected outcome for the
// permission under test.
//
// Unlike the REST surface, a GraphQL permission denial is HTTP 200 with a populated "errors"
// array: Cryostat's GraphQL security interceptor aborts the operation by throwing a
// GraphQLException, which SmallRye renders as an ordinary GraphQL error response. The status
// code therefore carries no authorization signal here, and the assertion is on the presence
// or absence of a "Forbidden" error instead.
func (r *TestResources) checkGraphQL(ctx context.Context, level *rbacPrivilegeLevel, allowed bool, permission string,
	base *url.URL, query *cryostatapi.GraphQLQuery, response *cryostatapi.ArchivedRecordingsGraphQLResponse) {
	g := gomega.NewWithT(&rbacGomegaT{r: r})
	label := fmt.Sprintf("GraphQL %s as %s, expecting %s", permission, level.Name, rbacExpectation(allowed))

	status, body, err := r.sendGraphQLRequest(ctx, base, query, level.Token, response)
	if !g.Expect(err).To(gomega.Succeed(), func() string {
		return fmt.Sprintf("%s: %s", label, err)
	}) {
		return
	}
	if !g.Expect(status).ToNot(gomega.Equal(http.StatusUnauthorized), func() string {
		return fmt.Sprintf("%s: got 401, so the auth proxy rejected the token before Cryostat"+
			" evaluated %s; check that the ServiceAccount passes the proxy's access review", label, permission)
	}) {
		return
	}
	if !g.Expect(status).To(gomega.Equal(http.StatusOK), func() string {
		return fmt.Sprintf("%s: got HTTP %d, want 200; GraphQL denials are reported in the"+
			" response body, not the status code; body: %s", label, status, body)
	}) {
		return
	}

	denied := response.Denied()
	if allowed {
		if !g.Expect(denied).To(gomega.BeFalse(), func() string {
			return fmt.Sprintf("%s: denied; errors: %v", label, response.Messages())
		}) {
			return
		}
		if !g.Expect(response.Errors).To(gomega.BeEmpty(), func() string {
			return fmt.Sprintf("%s: allowed but the response carries errors: %v", label, response.Messages())
		}) {
			return
		}
	} else {
		if !g.Expect(denied).To(gomega.BeTrue(), func() string {
			return fmt.Sprintf("%s: not denied; errors: %v, body: %s", label, response.Messages(), body)
		}) {
			return
		}
	}

	r.Log += fmt.Sprintf("%s: %s, as expected\n", label, rbacExpectation(allowed))
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
func rbacPutMetadataQuery(name string) *cryostatapi.GraphQLQuery {
	return &cryostatapi.GraphQLQuery{
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
func rbacDeleteQuery(name string) *cryostatapi.GraphQLQuery {
	return &cryostatapi.GraphQLQuery{
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
func rbacListQuery() *cryostatapi.GraphQLQuery {
	return &cryostatapi.GraphQLQuery{
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

// rbacAssertion is one permission check run against a privilege level, used to compose a
// resource's REST and GraphQL privilege matrix.
type rbacAssertion func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel)

// archivedRecordingsAssertions returns the archivedrecordings privilege matrix: every
// privilege level is checked against read, write and delete, on both the REST and GraphQL
// surfaces.
func archivedRecordingsAssertions() []rbacAssertion {
	return []rbacAssertion{
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.assertRESTRead(ctx, base, level)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.assertRESTWrite(ctx, base, fixture, level)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.assertRESTDelete(ctx, base, fixture, level)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.assertGraphQLRead(ctx, base, level)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.assertGraphQLWrite(ctx, base, fixture, level)
		},
		func(r *TestResources, ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
			r.assertGraphQLDelete(ctx, base, fixture, level)
		},
	}
}

// assertRBACMatrix runs every assertion in a resource's privilege matrix against every
// privilege level, in order. Failures are accumulated rather than returned, so that one
// unexpected outcome does not hide the rest of the matrix.
func (r *TestResources) assertRBACMatrix(ctx context.Context, base *url.URL, fixture *rbacFixture, resource string, levels []*rbacPrivilegeLevel, assertions []rbacAssertion) {
	for _, level := range levels {
		r.Log += fmt.Sprintf("\n--- privilege level %q (%s verbs %v) ---\n", level.Name, resource, level.Verbs)
		for _, assertion := range assertions {
			assertion(r, ctx, base, fixture, level)
		}
	}
}

// assertRESTRead checks archivedrecordings:read via GET /api/v4/recordings, which requires
// no other permission.
func (r *TestResources) assertRESTRead(ctx context.Context, base *url.URL, level *rbacPrivilegeLevel) {
	header := make(http.Header)
	header.Set("Accept", "*/*")
	r.checkREST(ctx, level, level.CanRead, rbacArchivedRecordingsReadPermission, http.MethodGet, base.JoinPath("/api/v4/recordings"), nil, header)
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
	r.checkREST(ctx, level, level.CanWrite, rbacArchivedRecordingsWritePermission, http.MethodPatch, requestURL, &body, header)
}

// assertRESTDelete checks archivedrecordings:delete via DELETE of an archived recording. The
// endpoint's other declared permission, targets:read, is held by every level, so the outcome
// turns only on archivedrecordings:delete. A level expected to succeed consumes an archived
// recording from the fixture; a level expected to be denied leaves it in place.
func (r *TestResources) assertRESTDelete(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	var archive *cryostatapi.ArchivedRecordingNode
	if level.CanDelete {
		archive = fixture.Take()
	} else {
		archive = fixture.Peek()
	}
	if archive == nil {
		r.logError(fmt.Sprintf("REST %s as %s: no archived recording left to act on", rbacArchivedRecordingsDeletePermission, level.Name))
		return
	}

	header := make(http.Header)
	header.Set("Accept", "*/*")
	requestURL := base.JoinPath("/api/beta/fs/recordings", archive.JvmId, archive.Name)
	r.checkREST(ctx, level, level.CanDelete, rbacArchivedRecordingsDeletePermission, http.MethodDelete, requestURL, nil, header)
}

// assertGraphQLRead checks archivedrecordings:read via the top-level archivedRecordings
// query.
func (r *TestResources) assertGraphQLRead(ctx context.Context, base *url.URL, level *rbacPrivilegeLevel) {
	r.checkGraphQL(ctx, level, level.CanRead, rbacArchivedRecordingsReadPermission, base, rbacListQuery(), &cryostatapi.ArchivedRecordingsGraphQLResponse{})
}

// assertGraphQLWrite checks archivedrecordings:write via the doPutMetadata resolver. Updating
// metadata is idempotent and non-destructive, so every level acts on the same archived
// recording.
func (r *TestResources) assertGraphQLWrite(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	archive := fixture.Peek()
	if archive == nil {
		r.logError(fmt.Sprintf("GraphQL %s as %s: no archived recording left to act on", rbacArchivedRecordingsWritePermission, level.Name))
		return
	}
	r.checkGraphQL(ctx, level, level.CanWrite, rbacArchivedRecordingsWritePermission, base, rbacPutMetadataQuery(archive.Name), &cryostatapi.ArchivedRecordingsGraphQLResponse{})
}

// assertGraphQLDelete checks archivedrecordings:delete via the doDelete resolver. A level
// expected to succeed consumes an archived recording from the fixture; a level expected to
// be denied leaves it in place.
func (r *TestResources) assertGraphQLDelete(ctx context.Context, base *url.URL, fixture *rbacFixture, level *rbacPrivilegeLevel) {
	var archive *cryostatapi.ArchivedRecordingNode
	if level.CanDelete {
		archive = fixture.Take()
	} else {
		archive = fixture.Peek()
	}
	if archive == nil {
		r.logError(fmt.Sprintf("GraphQL %s as %s: no archived recording left to act on", rbacArchivedRecordingsDeletePermission, level.Name))
		return
	}
	r.checkGraphQL(ctx, level, level.CanDelete, rbacArchivedRecordingsDeletePermission, base, rbacDeleteQuery(archive.Name), &cryostatapi.ArchivedRecordingsGraphQLResponse{})
}
