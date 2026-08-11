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

	t.Run("nil AuthorizationOptions produces no RBAC env vars", func(t *testing.T) {
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
