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

package webhooks

import (
	"context"
	"fmt"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/go-logr/logr"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type CryostatValidator struct {
	Client client.Client
	Log    *logr.Logger
}

var _ admission.CustomValidator = &CryostatValidator{}

// ValidateCreate validates a Create operation on a Cryostat
func (r *CryostatValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	return r.validate(ctx, obj, "create")
}

// ValidateCreate validates an Update operation on a Cryostat
func (r *CryostatValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	return r.validate(ctx, newObj, "update")
}

// ValidateCreate validates a Delete operation on a Cryostat
func (r *CryostatValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	// Nothing to validate on deletion
	return nil
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

func (r *CryostatValidator) validate(ctx context.Context, obj runtime.Object, op string) error {
	cr, ok := obj.(*operatorv1beta2.Cryostat)
	if !ok {
		return fmt.Errorf("expected a Cryostat, but received a %T", obj)
	}
	r.Log.Info(fmt.Sprintf("validate %s", op), "name", cr.Name, "namespace", cr.Namespace)

	// Look up the user who made this request
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("no admission request found in context: %w", err)
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

		err := r.Client.Create(ctx, sar)
		if err != nil {
			return fmt.Errorf("failed to check permissions: %w", err)
		}

		if !sar.Status.Allowed {
			return NewErrNotPermitted(op, namespace)
		}
	}

	return nil
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
