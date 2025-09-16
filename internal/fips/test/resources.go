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

package test

import (
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FIPSTestResources struct {
	*test.TestResources
}

const installConfig = `additionalTrustBundlePolicy: Proxyonly
apiVersion: v1
baseDomain: cluster.example.com
compute:
- architecture: amd64
  hyperthreading: Enabled
  name: worker
  platform: {}
  replicas: 3
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform: {}
  replicas: 3
%smetadata:
  creationTimestamp: null
  name: cluster
networking:
  clusterNetwork:
  - cidr: 0.0.0.0/32
    hostPrefix: 32
  machineNetwork:
  - cidr: 0.0.0.0/32
  networkType: OVNKubernetes
  serviceNetwork:
  - 0.0.0.0/32
platform: {}
publish: External
pullSecret: ""
sshKey: |
  ssh-rsa foo`

func (r *FIPSTestResources) NewClusterConfigFIPS() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-config-v1",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"install-config": fmt.Sprintf(installConfig, "fips: true\n"),
		},
	}
}

func (r *FIPSTestResources) NewClusterConfigNoFIPS() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-config-v1",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"install-config": fmt.Sprintf(installConfig, ""),
		},
	}
}

func (r *FIPSTestResources) NewClusterConfigBad() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-config-v1",
			Namespace: "kube-system",
		},
	}
}
