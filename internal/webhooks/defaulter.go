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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type cryostatDefaulter struct {
	log *logr.Logger
}

var _ admission.CustomDefaulter = &cryostatDefaulter{}

// Default applies default values to a Cryostat
func (r *cryostatDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cr, ok := obj.(*operatorv1beta2.Cryostat)
	if !ok {
		return fmt.Errorf("expected a Cryostat, but received a %T", obj)
	}
	r.log.Info("defaulting Cryostat", "name", cr.Name, "namespace", cr.Namespace)

	if cr.Spec.TargetNamespaces == nil {
		r.log.Info("defaulting target namespaces", "name", cr.Name, "namespace", cr.Namespace)
		cr.Spec.TargetNamespaces = []string{cr.Namespace}
	}
	return nil
}
