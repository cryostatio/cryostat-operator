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
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// TODO Remove this file with ClusterCryostat CRD

var _ conversion.Convertible = &ClusterCryostat{}

// ConvertTo converts this ClusterCryostat to the Hub version (v1beta2).
func (src *ClusterCryostat) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*operatorv1beta2.ClusterCryostat)

	// Copy ObjectMeta as-is
	dst.ObjectMeta = src.ObjectMeta

	// Convert existing Spec fields
	convertSpecTo(&src.Spec.CryostatSpec, &dst.Spec.CryostatSpec)
	dst.Spec.InstallNamespace = src.Spec.InstallNamespace
	dst.Spec.TargetNamespaces = src.Spec.TargetNamespaces

	// Convert existing Status fields
	convertStatusTo(&src.Status.CryostatStatus, &dst.Status.CryostatStatus)
	dst.Status.TargetNamespaces = src.Spec.TargetNamespaces

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version.
func (dst *ClusterCryostat) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*operatorv1beta2.ClusterCryostat)

	// Copy ObjectMeta as-is
	dst.ObjectMeta = src.ObjectMeta

	// Convert existing Spec fields
	convertSpecFrom(&src.Spec.CryostatSpec, &dst.Spec.CryostatSpec)
	dst.Spec.InstallNamespace = src.Spec.InstallNamespace
	dst.Spec.TargetNamespaces = src.Spec.TargetNamespaces

	// Convert existing Status fields
	convertStatusFrom(&src.Status.CryostatStatus, &dst.Status.CryostatStatus)
	dst.Status.TargetNamespaces = src.Spec.TargetNamespaces

	return nil
}
