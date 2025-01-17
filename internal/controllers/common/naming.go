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

package common

import (
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func AgentHeadlessServiceName(gvk *schema.GroupVersionKind, cr *model.CryostatInstance) string {
	return ClusterUniqueShortNameWithPrefix(gvk, "agent", cr.Name, cr.InstallNamespace)
}

func AgentGatewayServiceName(cr *model.CryostatInstance) string {
	return cr.Name + "-agent"
}

func AgentCertificateName(gvk *schema.GroupVersionKind, cr *model.CryostatInstance, targetNamespace string) string {
	return ClusterUniqueNameWithPrefixTargetNS(gvk, "agent", cr.Name, cr.InstallNamespace, targetNamespace)
}
