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

package webhook

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/go-logr/logr"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	rbacPermKeyRE = regexp.MustCompile(`^[a-z][a-z0-9]*:[a-z]+$`)
	rbacPermValRE = regexp.MustCompile(`^[a-z][a-z0-9]*(/[a-z][a-z0-9]*)?:[a-z]+$`)
)

type cryostatValidator struct {
	client client.Client
	log    *logr.Logger
}

var _ admission.CustomValidator = &cryostatValidator{}

// ValidateCreate validates a Create operation on a Cryostat
func (r *cryostatValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return r.validate(ctx, obj, "create")
}

// ValidateCreate validates an Update operation on a Cryostat
func (r *cryostatValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return r.validate(ctx, newObj, "update")
}

// ValidateCreate validates a Delete operation on a Cryostat
func (r *cryostatValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// Nothing to validate on deletion
	return nil, nil
}

type ErrNotPermitted struct {
	operation string
	namespace string
}

func NewErrNotPermitted(operation string, namespace string) *ErrNotPermitted {
	return &ErrNotPermitted{
		operation: operation,
		namespace: namespace,
	}
}

func (e *ErrNotPermitted) Error() string {
	return fmt.Sprintf("unable to %s Cryostat: user is not permitted to create a Cryostat in namespace %s", e.operation, e.namespace)
}

var _ error = &ErrNotPermitted{}

func (r *cryostatValidator) validate(ctx context.Context, obj runtime.Object, op string) (admission.Warnings, error) {
	cr, ok := obj.(*operatorv1beta2.Cryostat)
	if !ok {
		return nil, fmt.Errorf("expected a Cryostat, but received a %T", obj)
	}
	r.log.Info(fmt.Sprintf("validate %s", op), "name", cr.Name, "namespace", cr.Namespace)

	// Validate RBAC fields before checking namespace permissions.
	if errs := validateAuthorizationOptions(cr.Spec.AuthorizationOptions); len(errs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: operatorv1beta2.GroupVersion.Group, Kind: "Cryostat"},
			cr.Name, errs)
	}

	// Look up the user who made this request
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("no admission request found in context: %w", err)
	}
	userInfo := req.UserInfo

	// Check that for each target namespace, the user has permissions
	// to create a Cryostat CR in that namespace
	for _, namespace := range cr.Spec.TargetNamespaces {
		sar := &authzv1.SubjectAccessReview{
			Spec: authzv1.SubjectAccessReviewSpec{
				User:   userInfo.Username,
				Groups: userInfo.Groups,
				UID:    userInfo.UID,
				Extra:  translateExtra(userInfo.Extra),
				ResourceAttributes: &authzv1.ResourceAttributes{
					Namespace: namespace,
					Verb:      "create",
					Group:     operatorv1beta2.GroupVersion.Group,
					Version:   operatorv1beta2.GroupVersion.Version,
					Resource:  "cryostats",
				},
			},
		}

		err := r.client.Create(ctx, sar)
		if err != nil {
			return nil, fmt.Errorf("failed to check permissions: %w", err)
		}

		if !sar.Status.Allowed {
			return nil, NewErrNotPermitted(op, namespace)
		}
	}

	return nil, nil
}

func validateAuthorizationOptions(authz *operatorv1beta2.AuthorizationOptions) field.ErrorList {
	if authz == nil {
		return nil
	}
	authzPath := field.NewPath("spec", "authorizationOptions")
	var errs field.ErrorList

	errs = append(errs, validateRBACPermissions(authz.RBACPermissions, authzPath.Child("rbacPermissions"))...)
	errs = append(errs, validateRBACDefaultPermissions(authz.RBACDefaultPermissions, authzPath.Child("rbacDefaultPermissions"))...)
	errs = append(errs, validateRBACCacheOptions(authz.RBACCacheOptions, authzPath.Child("rbacCacheOptions"))...)
	return errs
}

func validateRBACPermissions(perms map[string]string, fldPath *field.Path) field.ErrorList {
	if perms == nil {
		return nil
	}
	var errs field.ErrorList
	seen := make(map[string]string, len(perms))
	for k, v := range perms {
		if !rbacPermKeyRE.MatchString(k) {
			errs = append(errs, field.Invalid(fldPath, k,
				"key must use the form '<resourcetype>:<verb>' with lowercase letters (e.g. 'activerecordings:read')"))
		}
		if !rbacPermValRE.MatchString(v) {
			errs = append(errs, field.Invalid(fldPath, v,
				"value must use the form 'resource[/subresource]:verb' with lowercase letters (e.g. 'pods/exec:create', 'deployments:get')"))
		}
		norm := strings.ToLower(k)
		if orig, exists := seen[norm]; exists {
			errs = append(errs, field.Invalid(fldPath, k,
				fmt.Sprintf("key collides with %q after case normalization", orig)))
		} else {
			seen[norm] = k
		}
	}
	return errs
}

func validateRBACDefaultPermissions(defaults *operatorv1beta2.RBACDefaultPermissions, fldPath *field.Path) field.ErrorList {
	if defaults == nil {
		return nil
	}
	var errs field.ErrorList
	if defaults.DefaultReadPermission != nil && !rbacPermValRE.MatchString(*defaults.DefaultReadPermission) {
		errs = append(errs, field.Invalid(fldPath.Child("defaultReadPermission"), *defaults.DefaultReadPermission,
			"must use the form 'resource[/subresource]:verb' with lowercase letters (e.g. 'pods:get')"))
	}
	if defaults.DefaultWritePermission != nil && !rbacPermValRE.MatchString(*defaults.DefaultWritePermission) {
		errs = append(errs, field.Invalid(fldPath.Child("defaultWritePermission"), *defaults.DefaultWritePermission,
			"must use the form 'resource[/subresource]:verb' with lowercase letters (e.g. 'pods/exec:create')"))
	}
	if defaults.DefaultDeletePermission != nil && !rbacPermValRE.MatchString(*defaults.DefaultDeletePermission) {
		errs = append(errs, field.Invalid(fldPath.Child("defaultDeletePermission"), *defaults.DefaultDeletePermission,
			"must use the form 'resource[/subresource]:verb' with lowercase letters (e.g. 'pods:delete')"))
	}
	if defaults.DefaultPermission != nil && !rbacPermValRE.MatchString(*defaults.DefaultPermission) {
		errs = append(errs, field.Invalid(fldPath.Child("defaultPermission"), *defaults.DefaultPermission,
			"must use the form 'resource[/subresource]:verb' with lowercase letters (e.g. 'pods/exec:create')"))
	}
	return errs
}

func validateRBACCacheOptions(opts *operatorv1beta2.RBACCacheOptions, fldPath *field.Path) field.ErrorList {
	if opts == nil {
		return nil
	}
	var errs field.ErrorList
	if opts.ClientCacheExpireAfterAccess != nil {
		if _, err := time.ParseDuration(*opts.ClientCacheExpireAfterAccess); err != nil {
			errs = append(errs, field.Invalid(fldPath.Child("clientCacheExpireAfterAccess"),
				*opts.ClientCacheExpireAfterAccess,
				"must be a valid Go duration string (e.g. '5m', '30s', '1h30m')"))
		}
	}
	if opts.DecisionCacheTTL != nil {
		if _, err := time.ParseDuration(*opts.DecisionCacheTTL); err != nil {
			errs = append(errs, field.Invalid(fldPath.Child("decisionCacheTTL"),
				*opts.DecisionCacheTTL,
				"must be a valid Go duration string (e.g. '1m', '30s', '0s')"))
		}
	}
	return errs
}

func translateExtra(extra map[string]authnv1.ExtraValue) map[string]authzv1.ExtraValue {
	var result map[string]authzv1.ExtraValue
	if extra == nil {
		return result
	}
	result = make(map[string]authzv1.ExtraValue, len(extra))
	for k, v := range extra {
		result[k] = authzv1.ExtraValue(v)
	}

	return result
}
