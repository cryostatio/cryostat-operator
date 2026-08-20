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

package resource_definitions

import (
	"net/url"
	"strings"
	"testing"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controller/model"
)

const rbacNamespaceEnvVar = "CRYOSTAT_SECURITY_RBAC_NAMESPACE"

// minimalCR builds the smallest CryostatInstance that lets newEnvForCoreContainer
// reach (and return from) the AuthorizationOptions block without panicking.
// specs must carry a non-nil StorageURL because DeployManagedStorage is true
// whenever ObjectStorageOptions is nil.
func minimalCR(permissions map[string]string) (*model.CryostatInstance, *ServiceSpecs) {
	storageURL, _ := url.Parse("http://storage.svc:8333")
	databaseURL, _ := url.Parse("http://database.svc:5432")
	specs := &ServiceSpecs{
		StorageURL:  storageURL,
		DatabaseURL: databaseURL,
	}
	cr := &model.CryostatInstance{
		Name:             "cryostat",
		InstallNamespace: "default",
		Spec: &operatorv1beta2.CryostatSpec{
			AuthorizationOptions: &operatorv1beta2.AuthorizationOptions{
				RBACPermissions: permissions,
			},
		},
		Status: &operatorv1beta2.CryostatStatus{},
	}
	return cr, specs
}

func TestNewEnvForCoreContainer_RBACPermissions(t *testing.T) {
	t.Run("distinct keys produce correct env var names in sorted order", func(t *testing.T) {
		// Keys use the Cryostat internal resource:verb form; values are k8s resource:verb pairs.
		perms := map[string]string{
			"activerecordings:read": "deployments:get",
			"targets:read":          "deployments:get",
			"credentials:write":     "pods:delete",
		}
		cr, specs := minimalCR(perms)

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Colon is normalized to underscore and the key is uppercased.
		expected := []struct{ name, value string }{
			{"CRYOSTAT_SECURITY_RBAC_PERMISSIONS__ACTIVERECORDINGS_READ_", "deployments:get"},
			{"CRYOSTAT_SECURITY_RBAC_PERMISSIONS__CREDENTIALS_WRITE_", "pods:delete"},
			{"CRYOSTAT_SECURITY_RBAC_PERMISSIONS__TARGETS_READ_", "deployments:get"},
		}

		var found []struct{ name, value string }
		for _, e := range envs {
			if strings.HasPrefix(e.Name, "CRYOSTAT_SECURITY_RBAC_PERMISSIONS__") {
				found = append(found, struct{ name, value string }{e.Name, e.Value})
			}
		}

		if len(found) != len(expected) {
			t.Fatalf("expected %d RBAC env vars, got %d: %v", len(expected), len(found), found)
		}
		for i, ex := range expected {
			if found[i].name != ex.name {
				t.Errorf("env var[%d]: expected name %q, got %q", i, ex.name, found[i].name)
			} else if found[i].value != ex.value {
				t.Errorf("env var[%d] %q: expected value %q, got %q", i, ex.name, ex.value, found[i].value)
			}
		}
	})

	t.Run("hyphen in key is normalized to underscore", func(t *testing.T) {
		perms := map[string]string{
			"my-resource:read": "deployments:get",
		}
		cr, specs := minimalCR(perms)

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "CRYOSTAT_SECURITY_RBAC_PERMISSIONS__MY_RESOURCE_READ_"
		for _, e := range envs {
			if e.Name == want {
				return
			}
		}
		t.Errorf("expected env var %q not found in output", want)
	})

	t.Run("colon in key is normalized to underscore", func(t *testing.T) {
		perms := map[string]string{
			"targets:read": "deployments:get",
		}
		cr, specs := minimalCR(perms)

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "CRYOSTAT_SECURITY_RBAC_PERMISSIONS__TARGETS_READ_"
		for _, e := range envs {
			if e.Name == want {
				return
			}
		}
		t.Errorf("expected env var %q not found in output", want)
	})

	t.Run("colon in multi-word key is normalized to underscore (regression: archivedrecordings:read)", func(t *testing.T) {
		// Regression: the key "archivedrecordings:read" was reported to produce an
		// env var name containing a literal colon, e.g.
		// CRYOSTAT_SECURITY_RBAC_PERMISSIONS__ARCHIVEDRECORDINGS:READ_
		// The colon must be replaced with an underscore.
		perms := map[string]string{
			"archivedrecordings:read": "pods:get",
		}
		cr, specs := minimalCR(perms)

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "CRYOSTAT_SECURITY_RBAC_PERMISSIONS__ARCHIVEDRECORDINGS_READ_"
		for _, e := range envs {
			if strings.Contains(e.Name, ":") {
				t.Errorf("env var name %q contains a literal colon; colons must be normalized to underscores", e.Name)
			}
			if e.Name == want {
				return
			}
		}
		t.Errorf("expected env var %q not found in output", want)
	})

	t.Run("keys that collide after normalization return an error", func(t *testing.T) {
		// "targets:read" and "targets-read" both normalize to "TARGETS_READ", so they collide.
		perms := map[string]string{
			"targets:read": "deployments:get",
			"targets-read": "pods:get",
		}
		cr, specs := minimalCR(perms)

		_, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err == nil {
			t.Fatal("expected an error for colliding RBAC permission keys, got nil")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "TARGETS_READ") {
			t.Errorf("error message should mention the colliding env var name %q, got: %s", "TARGETS_READ", errMsg)
		}
		if !strings.Contains(errMsg, "rbacPermissions") {
			t.Errorf("error message should mention rbacPermissions, got: %s", errMsg)
		}
	})

	t.Run("nil AuthorizationOptions produces no RBAC permission env vars", func(t *testing.T) {
		storageURL, _ := url.Parse("http://storage.svc:8333")
		specs := &ServiceSpecs{StorageURL: storageURL}
		cr := &model.CryostatInstance{
			Name:             "cryostat",
			InstallNamespace: "default",
			Spec:             &operatorv1beta2.CryostatSpec{},
			Status:           &operatorv1beta2.CryostatStatus{},
		}

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, e := range envs {
			if strings.HasPrefix(e.Name, "CRYOSTAT_SECURITY_RBAC_PERMISSIONS__") {
				t.Errorf("unexpected RBAC env var %q when AuthorizationOptions is nil", e.Name)
			}
		}
	})
}

func TestNewEnvForCoreContainer_NamespacedRBAC(t *testing.T) {
	t.Run("NamespacedRBACPermissions nil (default) injects install namespace", func(t *testing.T) {
		cr, specs := minimalCR(nil)
		// NamespacedRBACPermissions is nil — treated as true (namespaced by default)

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, e := range envs {
			if e.Name == rbacNamespaceEnvVar {
				if e.Value != cr.InstallNamespace {
					t.Errorf("expected %s=%q, got %q", rbacNamespaceEnvVar, cr.InstallNamespace, e.Value)
				}
				return
			}
		}
		t.Errorf("%s not found in env vars", rbacNamespaceEnvVar)
	})

	t.Run("NamespacedRBACPermissions true injects install namespace", func(t *testing.T) {
		cr, specs := minimalCR(nil)
		cr.Spec.AuthorizationOptions.NamespacedRBACPermissions = &[]bool{true}[0]

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, e := range envs {
			if e.Name == rbacNamespaceEnvVar {
				if e.Value != cr.InstallNamespace {
					t.Errorf("expected %s=%q, got %q", rbacNamespaceEnvVar, cr.InstallNamespace, e.Value)
				}
				return
			}
		}
		t.Errorf("%s not found in env vars", rbacNamespaceEnvVar)
	})

	t.Run("NamespacedRBACPermissions false omits namespace env var", func(t *testing.T) {
		cr, specs := minimalCR(nil)
		cr.Spec.AuthorizationOptions.NamespacedRBACPermissions = &[]bool{false}[0]

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, e := range envs {
			if e.Name == rbacNamespaceEnvVar {
				t.Errorf("unexpected %s when NamespacedRBACPermissions=false", rbacNamespaceEnvVar)
			}
		}
	})

	t.Run("nil AuthorizationOptions injects install namespace (default namespaced)", func(t *testing.T) {
		storageURL, _ := url.Parse("http://storage.svc:8333")
		databaseURL, _ := url.Parse("http://database.svc:5432")
		specs := &ServiceSpecs{StorageURL: storageURL, DatabaseURL: databaseURL}
		cr := &model.CryostatInstance{
			Name:             "cryostat",
			InstallNamespace: "default",
			Spec:             &operatorv1beta2.CryostatSpec{},
			Status:           &operatorv1beta2.CryostatStatus{},
		}

		envs, err := newEnvForCoreContainer(cr, specs, nil, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, e := range envs {
			if e.Name == rbacNamespaceEnvVar {
				if e.Value != cr.InstallNamespace {
					t.Errorf("expected %s=%q, got %q", rbacNamespaceEnvVar, cr.InstallNamespace, e.Value)
				}
				return
			}
		}
		t.Errorf("%s not found in env vars", rbacNamespaceEnvVar)
	})
}

func TestNewRBACCacheEnvForCoreContainer(t *testing.T) {
	t.Run("nil AuthorizationOptions produces no cache env vars", func(t *testing.T) {
		cr := &model.CryostatInstance{
			Name:             "cryostat",
			InstallNamespace: "default",
			Spec:             &operatorv1beta2.CryostatSpec{},
			Status:           &operatorv1beta2.CryostatStatus{},
		}
		envs := newRBACCacheEnvForCoreContainer(cr)
		if len(envs) != 0 {
			t.Errorf("expected no cache env vars, got %v", envs)
		}
	})

	t.Run("nil RBACCacheOptions produces no cache env vars", func(t *testing.T) {
		cr, _ := minimalCR(nil)
		envs := newRBACCacheEnvForCoreContainer(cr)
		if len(envs) != 0 {
			t.Errorf("expected no cache env vars, got %v", envs)
		}
	})

	t.Run("all fields set produces all four env vars", func(t *testing.T) {
		cr, _ := minimalCR(nil)
		size1 := int64(500)
		size2 := int64(5000)
		cr.Spec.AuthorizationOptions.RBACCacheOptions = &operatorv1beta2.RBACCacheOptions{
			ClientCacheExpireAfterAccess: strPtr("10m"),
			ClientCacheMaximumSize:       &size1,
			DecisionCacheTTL:             strPtr("2m"),
			DecisionCacheMaximumSize:     &size2,
		}

		envs := newRBACCacheEnvForCoreContainer(cr)

		expected := map[string]string{
			"CRYOSTAT_SECURITY_RBAC_CACHE_EXPIRE_AFTER_ACCESS":   "10m",
			"CRYOSTAT_SECURITY_RBAC_CACHE_MAXIMUM_SIZE":          "500",
			"CRYOSTAT_SECURITY_RBAC_DECISION_CACHE_TTL":          "2m",
			"CRYOSTAT_SECURITY_RBAC_DECISION_CACHE_MAXIMUM_SIZE": "5000",
		}
		if len(envs) != len(expected) {
			t.Fatalf("expected %d env vars, got %d: %v", len(expected), len(envs), envs)
		}
		for _, e := range envs {
			want, ok := expected[e.Name]
			if !ok {
				t.Errorf("unexpected env var %q", e.Name)
			} else if e.Value != want {
				t.Errorf("%s: expected %q, got %q", e.Name, want, e.Value)
			}
		}
	})

	t.Run("only DecisionCacheTTL set produces only that env var", func(t *testing.T) {
		cr, _ := minimalCR(nil)
		cr.Spec.AuthorizationOptions.RBACCacheOptions = &operatorv1beta2.RBACCacheOptions{
			DecisionCacheTTL: strPtr("30s"),
		}

		envs := newRBACCacheEnvForCoreContainer(cr)

		if len(envs) != 1 {
			t.Fatalf("expected 1 env var, got %d: %v", len(envs), envs)
		}
		if envs[0].Name != "CRYOSTAT_SECURITY_RBAC_DECISION_CACHE_TTL" {
			t.Errorf("unexpected env var name %q", envs[0].Name)
		}
		if envs[0].Value != "30s" {
			t.Errorf("expected value %q, got %q", "30s", envs[0].Value)
		}
	})

	t.Run("zero-value sizes are emitted when explicitly set to 0", func(t *testing.T) {
		cr, _ := minimalCR(nil)
		zero := int64(0)
		cr.Spec.AuthorizationOptions.RBACCacheOptions = &operatorv1beta2.RBACCacheOptions{
			ClientCacheMaximumSize:   &zero,
			DecisionCacheMaximumSize: &zero,
		}

		envs := newRBACCacheEnvForCoreContainer(cr)

		if len(envs) != 2 {
			t.Fatalf("expected 2 env vars, got %d: %v", len(envs), envs)
		}
		for _, e := range envs {
			if e.Value != "0" {
				t.Errorf("%s: expected value \"0\", got %q", e.Name, e.Value)
			}
		}
	})
}

func strPtr(s string) *string { return &s }
