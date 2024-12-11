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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var cryostatlog = logf.Log.WithName("cryostat-resource")

// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

//+kubebuilder:webhook:path=/mutate-operator-cryostat-io-v1beta2-cryostat,mutating=true,failurePolicy=fail,sideEffects=None,groups=operator.cryostat.io,resources=cryostats,verbs=create;update,versions=v1beta2,name=mcryostat.kb.io,admissionReviewVersions=v1

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-operator-cryostat-io-v1beta2-cryostat,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.cryostat.io,resources=cryostats,verbs=create;update,versions=v1beta2,name=vcryostat.kb.io,admissionReviewVersions=v1

func SetupWebhookWithManager(mgr ctrl.Manager, apiType runtime.Object) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(apiType).
		WithValidator(&cryostatValidator{
			client: mgr.GetClient(),
			log:    &cryostatlog,
		}).
		WithDefaulter(&cryostatDefaulter{
			log: &cryostatlog,
		}).
		Complete()
}
