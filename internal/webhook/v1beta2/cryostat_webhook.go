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

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta2

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
)

// nolint:unused
// log is for logging in this package.
var cryostatlog = logf.Log.WithName("cryostat-resource")

// SetupCryostatWebhookWithManager registers the webhook for Cryostat in the manager.
func SetupCryostatWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&operatorv1beta2.Cryostat{}).
		WithValidator(&CryostatCustomValidator{}).
		WithDefaulter(&CryostatCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-operator-cryostat-io-v1beta2-cryostat,mutating=true,failurePolicy=fail,sideEffects=None,groups=operator.cryostat.io,resources=cryostats,verbs=create;update,versions=v1beta2,name=mcryostat-v1beta2.kb.io,admissionReviewVersions=v1

// CryostatCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Cryostat when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CryostatCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &CryostatCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Cryostat.
func (d *CryostatCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	cryostat, ok := obj.(*operatorv1beta2.Cryostat)

	if !ok {
		return fmt.Errorf("expected an Cryostat object but got %T", obj)
	}
	cryostatlog.Info("Defaulting for Cryostat", "name", cryostat.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-operator-cryostat-io-v1beta2-cryostat,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.cryostat.io,resources=cryostats,verbs=create;update,versions=v1beta2,name=vcryostat-v1beta2.kb.io,admissionReviewVersions=v1

// CryostatCustomValidator struct is responsible for validating the Cryostat resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CryostatCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &CryostatCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Cryostat.
func (v *CryostatCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	cryostat, ok := obj.(*operatorv1beta2.Cryostat)
	if !ok {
		return nil, fmt.Errorf("expected a Cryostat object but got %T", obj)
	}
	cryostatlog.Info("Validation for Cryostat upon creation", "name", cryostat.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Cryostat.
func (v *CryostatCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	cryostat, ok := newObj.(*operatorv1beta2.Cryostat)
	if !ok {
		return nil, fmt.Errorf("expected a Cryostat object for the newObj but got %T", newObj)
	}
	cryostatlog.Info("Validation for Cryostat upon update", "name", cryostat.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Cryostat.
func (v *CryostatCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cryostat, ok := obj.(*operatorv1beta2.Cryostat)
	if !ok {
		return nil, fmt.Errorf("expected a Cryostat object but got %T", obj)
	}
	cryostatlog.Info("Validation for Cryostat upon deletion", "name", cryostat.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
