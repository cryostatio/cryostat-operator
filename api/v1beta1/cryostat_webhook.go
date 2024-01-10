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

package v1beta1

import (
	"context"
	"fmt"

	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var cryostatlog = logf.Log.WithName("cryostat-resource")

type cryostatValidator struct {
	client client.Client
}

// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *Cryostat) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&cryostatValidator{
			client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-operator-cryostat-io-v1beta1-cryostat,mutating=true,failurePolicy=fail,sideEffects=None,groups=operator.cryostat.io,resources=cryostats,verbs=create;update,versions=v1beta1,name=mcryostat.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Cryostat{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Cryostat) Default() {
	cryostatlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-operator-cryostat-io-v1beta1-cryostat,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.cryostat.io,resources=cryostats,verbs=create;update,versions=v1beta1,name=vcryostat.kb.io,admissionReviewVersions=v1

var _ admission.CustomValidator = &cryostatValidator{}

// ValidateCreate implements admission.CustomValidator so a webhook will be registered for the type
func (r *cryostatValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	return r.validate(ctx, obj, "create")
}

// ValidateUpdate implements admission.CustomValidator so a webhook will be registered for the type
func (r *cryostatValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	return r.validate(ctx, newObj, "update")
}

// ValidateDelete implements admission.CustomValidator so a webhook will be registered for the type
func (r *cryostatValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	// Nothing to validate on deletion
	return nil
}

func (r *cryostatValidator) validate(ctx context.Context, obj runtime.Object, op string) error {
	cr, ok := obj.(*Cryostat)
	if !ok {
		return fmt.Errorf("expected a Cryostat, but received a %T", obj)
	}
	cryostatlog.Info(fmt.Sprintf("validate %s", op), "name", cr.Name)

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
					Group:     GroupVersion.Group,
					Version:   GroupVersion.Version,
					Resource:  "cryostats",
				},
			},
		}

		err := r.client.Create(ctx, sar)
		if err != nil {
			return fmt.Errorf("failed to check permissions: %w", err)
		}

		if !sar.Status.Allowed {
			return fmt.Errorf("unable to %s Cryostat: user is not permitted to create a Cryostat in namespace %s", op, namespace)
		}
		cryostatlog.Info(fmt.Sprintf("Access Allowed: %v", userInfo)) // XXX
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
