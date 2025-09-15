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

package fips

import (
	"context"
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var fipsInstallConfigRegex = regexp.MustCompile(`(?m)^\s*fips:\s*true\s*$`)

func IsFIPS(reader client.Reader) (bool, error) {
	cm := &corev1.ConfigMap{}

	// Query cluster-config-v1 config map for FIPS mode
	// https://access.redhat.com/solutions/6525331
	err := reader.Get(context.Background(), types.NamespacedName{Name: "cluster-config-v1", Namespace: "kube-system"}, cm)
	if err != nil {
		return false, err
	}

	config, pres := cm.Data["install-config"]
	if !pres {
		return false, fmt.Errorf("%s config map has no \"install-config\" key", cm.Name)
	}

	return fipsInstallConfigRegex.MatchString(config), nil
}
