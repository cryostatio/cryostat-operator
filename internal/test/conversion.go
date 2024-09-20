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
	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// v1beta1 versions of Cryostat CRs used for testing the conversion webhook

func (r *TestResources) NewCryostatV1Beta1() *operatorv1beta1.Cryostat {
	return &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: r.newCryostatSpecV1Beta1(),
	}
}

func (r *TestResources) newCryostatSpecV1Beta1() operatorv1beta1.CryostatSpec {
	certManager := true
	var reportOptions *operatorv1beta1.ReportConfiguration
	if r.ReportReplicas > 0 {
		reportOptions = &operatorv1beta1.ReportConfiguration{
			Replicas: r.ReportReplicas,
		}
	}
	return operatorv1beta1.CryostatSpec{
		EnableCertManager: &certManager,
		ReportOptions:     reportOptions,
	}
}

func (r *TestResources) NewCryostatWithMinimalModeV1Beta1() *operatorv1beta1.Cryostat {
	spec := r.newCryostatSpecV1Beta1()
	spec.Minimal = true
	return &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: spec,
	}
}

func (r *TestResources) NewCryostatWithSecretsV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	key := "test.crt"
	cr.Spec.TrustedCertSecrets = []operatorv1beta1.CertificateSecret{
		{
			SecretName:     "testCert1",
			CertificateKey: &key,
		},
		{
			SecretName: "testCert2",
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithTemplatesV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.EventTemplates = []operatorv1beta1.TemplateConfigMap{
		{
			ConfigMapName: "templateCM1",
			Filename:      "template.jfc",
		},
		{
			ConfigMapName: "templateCM2",
			Filename:      "other-template.jfc",
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithIngressV1Beta1() *operatorv1beta1.Cryostat {
	return r.addIngressToCryostatV1Beta1(r.NewCryostatV1Beta1())
}

func (r *TestResources) NewCryostatWithIngressCertManagerDisabledV1Beta1() *operatorv1beta1.Cryostat {
	return r.addIngressToCryostatV1Beta1(r.NewCryostatCertManagerDisabledV1Beta1())
}

func (r *TestResources) addIngressToCryostatV1Beta1(cr *operatorv1beta1.Cryostat) *operatorv1beta1.Cryostat {
	networkConfig := r.newNetworkConfigurationListV1Beta1()
	cr.Spec.NetworkOptions = &networkConfig
	return cr
}

func (r *TestResources) NewCryostatWithPVCSpecV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Annotations: map[string]string{
				"my/custom": "annotation",
			},
			Labels: map[string]string{
				"my":  "label",
				"app": "somethingelse",
			},
			Spec: newPVCSpec("cool-storage", "10Gi", corev1.ReadWriteMany),
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithPVCSpecSomeDefaultV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Spec: newPVCSpec("", "1Gi"),
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithPVCLabelsOnlyV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Labels: map[string]string{
				"my": "label",
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithDefaultEmptyDirV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		EmptyDir: &operatorv1beta1.EmptyDirConfig{
			Enabled: true,
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithEmptyDirSpecV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		EmptyDir: &operatorv1beta1.EmptyDirConfig{
			Enabled:   true,
			Medium:    "Memory",
			SizeLimit: "200Mi",
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithCoreSvcV1Beta1() *operatorv1beta1.Cryostat {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	cr := r.NewCryostatV1Beta1()
	cr.Spec.ServiceOptions = &operatorv1beta1.ServiceConfigList{
		CoreConfig: &operatorv1beta1.CoreServiceConfig{
			HTTPPort: &httpPort,
			ServiceConfig: operatorv1beta1.ServiceConfig{
				ServiceType: &svcType,
				Annotations: map[string]string{
					"my/custom": "annotation",
				},
				Labels: map[string]string{
					"my":  "label",
					"app": "somethingelse",
				},
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithCoreSvcJMXPortV1Beta1() *operatorv1beta1.Cryostat {
	jmxPort := int32(9095)
	cr := r.NewCryostatWithCoreSvcV1Beta1()
	cr.Spec.ServiceOptions.CoreConfig.JMXPort = &jmxPort
	return cr
}

func (r *TestResources) NewCryostatWithGrafanaSvcV1Beta1() *operatorv1beta1.Cryostat {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	cr := r.NewCryostatV1Beta1()
	cr.Spec.ServiceOptions = &operatorv1beta1.ServiceConfigList{
		GrafanaConfig: &operatorv1beta1.GrafanaServiceConfig{
			HTTPPort: &httpPort,
			ServiceConfig: operatorv1beta1.ServiceConfig{
				ServiceType: &svcType,
				Annotations: map[string]string{
					"my/custom": "annotation",
				},
				Labels: map[string]string{
					"my":  "label",
					"app": "somethingelse",
				},
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportsSvcV1Beta1() *operatorv1beta1.Cryostat {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(13161)
	cr := r.NewCryostatV1Beta1()
	cr.Spec.ServiceOptions = &operatorv1beta1.ServiceConfigList{
		ReportsConfig: &operatorv1beta1.ReportsServiceConfig{
			HTTPPort: &httpPort,
			ServiceConfig: operatorv1beta1.ServiceConfig{
				ServiceType: &svcType,
				Annotations: map[string]string{
					"my/custom": "annotation",
				},
				Labels: map[string]string{
					"my":  "label",
					"app": "somethingelse",
				},
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithCoreNetworkOptionsV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		CoreConfig: &operatorv1beta1.NetworkConfiguration{
			Annotations: map[string]string{"custom": "annotation"},
			Labels: map[string]string{
				"custom":    "label",
				"app":       "test-app",
				"component": "test-comp",
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithGrafanaNetworkOptionsV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		GrafanaConfig: &operatorv1beta1.NetworkConfiguration{
			Annotations: map[string]string{"grafana": "annotation"},
			Labels: map[string]string{
				"grafana":   "label",
				"component": "test-comp",
				"app":       "test-app",
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportsResourcesV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1600m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("800m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportLowResourceLimitV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
	}
	return cr
}

func populateCryostatWithSchedulingV1Beta1() *operatorv1beta1.SchedulingConfiguration {
	return &operatorv1beta1.SchedulingConfiguration{
		NodeSelector: map[string]string{"node": "good"},
		Affinity: &operatorv1beta1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node",
									Operator: corev1.NodeSelectorOpIn,
									Values: []string{
										"good",
										"better",
									},
								},
							},
						},
					},
				},
			},
			PodAffinity: &corev1.PodAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						LabelSelector: metav1.AddLabelToSelector(&metav1.LabelSelector{},
							"pod", "good"),
						TopologyKey: "topology.kubernetes.io/zone",
					},
				},
			},
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{LabelSelector: metav1.AddLabelToSelector(&metav1.LabelSelector{},
						"pod", "bad"),
						TopologyKey: "topology.kubernetes.io/zone",
					},
				},
			},
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "node",
				Operator: corev1.TolerationOpEqual,
				Value:    "ok",
				Effect:   corev1.TaintEffectNoExecute,
			},
		},
	}

}

func (r *TestResources) NewCryostatWithSchedulingV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.SchedulingOptions = populateCryostatWithSchedulingV1Beta1()
	return cr
}

func (r *TestResources) NewCryostatWithReportsSchedulingV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
		Replicas:          1,
		SchedulingOptions: populateCryostatWithSchedulingV1Beta1(),
	}

	return cr
}

func (r *TestResources) NewCryostatCertManagerDisabledV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	certManager := false
	cr.Spec.EnableCertManager = &certManager
	return cr
}

func (r *TestResources) NewCryostatCertManagerUndefinedV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.EnableCertManager = nil
	return cr
}

func (r *TestResources) NewCryostatWithResourcesV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.Resources = &operatorv1beta1.ResourceConfigList{
		CoreResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		GrafanaResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("550m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("128m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		DataSourceResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("600m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("300m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
	return cr
}

// Converted from v1beta1 to v1beta2
func (r *TestResources) NewCryostatWithResourcesToV1Beta2() *model.CryostatInstance {
	cr := r.NewCryostatWithResources()
	cr.Spec.Resources.DatabaseResources = corev1.ResourceRequirements{}
	cr.Spec.Resources.ObjectStorageResources = corev1.ResourceRequirements{}
	cr.Spec.Resources.AuthProxyResources = corev1.ResourceRequirements{}
	cr.Spec.Resources.AgentProxyResources = corev1.ResourceRequirements{}
	return cr
}

func (r *TestResources) NewCryostatWithLowResourceLimitV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.Resources = &operatorv1beta1.ResourceConfigList{
		CoreResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
		GrafanaResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
		DataSourceResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
	}
	return cr
}

// Converted from v1beta1 to v1beta2
func (r *TestResources) NewCryostatWithLowResourceLimitToV1Beta2() *model.CryostatInstance {
	cr := r.NewCryostatWithLowResourceLimit()
	cr.Spec.Resources.DatabaseResources = corev1.ResourceRequirements{}
	cr.Spec.Resources.ObjectStorageResources = corev1.ResourceRequirements{}
	cr.Spec.Resources.AuthProxyResources = corev1.ResourceRequirements{}
	cr.Spec.Resources.AgentProxyResources = corev1.ResourceRequirements{}
	return cr
}

func (r *TestResources) NewCryostatWithAuthPropertiesV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.AuthProperties = &operatorv1beta1.AuthorizationProperties{
		ConfigMapName:   "authConfigMapName",
		Filename:        "auth.properties",
		ClusterRoleName: "custom-auth-cluster-role",
	}
	return cr
}

func (r *TestResources) NewCryostatWithBuiltInDiscoveryDisabledV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta1.TargetDiscoveryOptions{
		BuiltInDiscoveryDisabled: true,
	}
	return cr
}

func (r *TestResources) NewCryostatWithDiscoveryPortConfigV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta1.TargetDiscoveryOptions{
		DiscoveryPortNames:   []string{"custom-port-name", "another-custom-port-name"},
		DiscoveryPortNumbers: []int32{9092, 9090},
	}
	return cr
}

func (r *TestResources) NewCryostatWithBuiltInPortConfigDisabledV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta1.TargetDiscoveryOptions{
		DisableBuiltInPortNames:   true,
		DisableBuiltInPortNumbers: true,
	}
	return cr
}

func (r *TestResources) NewCryostatWithJmxCacheOptionsSpecV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.JmxCacheOptions = &operatorv1beta1.JmxCacheOptions{
		TargetCacheSize: 10,
		TargetCacheTTL:  20,
	}
	return cr
}

func (r *TestResources) NewCryostatWithWsConnectionsSpecV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.MaxWsConnections = 10
	return cr
}

func (r *TestResources) newCommandService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-command",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":       r.Name,
				"component": "cryostat",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":       r.Name,
				"component": "cryostat",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       10001,
					TargetPort: intstr.FromInt(10001),
				},
			},
		},
	}
}

func (r *TestResources) NewCryostatWithCommandConfigV1Beta1() *operatorv1beta1.Cryostat {
	commandSVC := r.newCommandService()
	commandIng := r.newNetworkConfigurationV1Beta1(commandSVC.Name, commandSVC.Spec.Ports[0].Port)
	commandIng.Annotations["command"] = "annotation"
	commandIng.Labels["command"] = "label"

	cr := r.NewCryostatWithIngressV1Beta1()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		CoreConfig:    cr.Spec.NetworkOptions.CoreConfig,
		GrafanaConfig: cr.Spec.NetworkOptions.GrafanaConfig,
		CommandConfig: &commandIng,
	}
	return cr
}

func (r *TestResources) newGrafanaService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-grafana",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":       r.Name,
				"component": "cryostat",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":       r.Name,
				"component": "cryostat",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       3000,
					TargetPort: intstr.FromInt(3000),
				},
			},
		},
	}
}

func (r *TestResources) NewCryostatWithGrafanaConfigV1Beta1() *operatorv1beta1.Cryostat {
	grafanaSVC := r.newGrafanaService()
	grafanaIng := r.newNetworkConfigurationV1Beta1(grafanaSVC.Name, grafanaSVC.Spec.Ports[0].Port)
	grafanaIng.Annotations["command"] = "annotation"
	grafanaIng.Labels["command"] = "label"

	cr := r.NewCryostatWithIngressV1Beta1()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		CoreConfig:    cr.Spec.NetworkOptions.CoreConfig,
		GrafanaConfig: &grafanaIng,
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportSubprocessHeapSpecV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	if cr.Spec.ReportOptions == nil {
		cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{}
	}
	cr.Spec.ReportOptions.SubProcessMaxHeapSize = 500
	return cr
}

func (r *TestResources) NewCryostatWithSecurityOptionsV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	privEscalation := true
	nonRoot := false
	runAsUser := int64(0)
	fsGroup := int64(20000)
	cr.Spec.SecurityOptions = &operatorv1beta1.SecurityOptions{
		PodSecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &nonRoot,
			FSGroup:      &fsGroup,
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		CoreSecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
		GrafanaSecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
		DataSourceSecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
	}
	return cr
}

// Converted from v1beta1 to v1beta2
func (r *TestResources) NewCryostatWithSecurityOptionsToV1Beta2() *model.CryostatInstance {
	cr := r.NewCryostatWithSecurityOptions()
	cr.Spec.SecurityOptions.DatabaseSecurityContext = nil
	cr.Spec.SecurityOptions.StorageSecurityContext = nil
	cr.Spec.SecurityOptions.AuthProxySecurityContext = nil
	cr.Spec.SecurityOptions.AgentProxySecurityContext = nil
	return cr
}

func (r *TestResources) NewCryostatWithReportSecurityOptionsV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	nonRoot := true
	privEscalation := false
	runAsUser := int64(1002)
	if cr.Spec.ReportOptions == nil {
		cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{}
	}
	cr.Spec.ReportOptions.SecurityOptions = &operatorv1beta1.ReportsSecurityOptions{
		PodSecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &nonRoot,
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		ReportsSecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			RunAsUser:                &runAsUser,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithDatabaseSecretProvidedV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.JmxCredentialsDatabaseOptions = &operatorv1beta1.JmxCredentialsDatabaseOptions{
		DatabaseSecretName: &providedDatabaseSecretName,
	}
	return cr
}

func (r *TestResources) NewCryostatWithAdditionalMetadataV1Beta1() *operatorv1beta1.Cryostat {
	cr := r.NewCryostatV1Beta1()
	cr.Spec.OperandMetadata = &operatorv1beta1.OperandMetadata{
		DeploymentMetadata: &operatorv1beta1.ResourceMetadata{
			Labels: map[string]string{
				"myDeploymentExtraLabel":       "myDeploymentLabel",
				"mySecondDeploymentExtraLabel": "mySecondDeploymentLabel",
				// below, labels that should be discarded as overriden by the default
				"app":                    "myApp",
				"component":              "myComponent",
				"kind":                   "myKind",
				"app.kubernetes.io/name": "myName",
			},
			Annotations: map[string]string{
				"myDeploymentExtraAnnotation":       "myDeploymentAnnotation",
				"mySecondDeploymentExtraAnnotation": "mySecondDeploymentAnnotation",
				// below, annotation that should be discarded as overriden by the default
				"app.openshift.io/connects-to": "connectToMe",
			},
		},
		PodMetadata: &operatorv1beta1.ResourceMetadata{
			Labels: map[string]string{
				"myPodExtraLabel":       "myPodLabel",
				"myPodSecondExtraLabel": "myPodSecondLabel",
				// below, labels that should be discarded as overriden by the default
				"app":       "myApp",
				"component": "myComponent",
				"kind":      "myKind",
			},
			Annotations: map[string]string{
				"myPodExtraAnnotation":       "myPodAnnotation",
				"mySecondPodExtraAnnotation": "mySecondPodAnnotation",
			},
		},
	}
	return cr
}

func (r *TestResources) newNetworkConfigurationListV1Beta1() operatorv1beta1.NetworkConfigurationList {
	coreSVC := r.NewCryostatService()
	coreIng := r.newNetworkConfigurationV1Beta1(coreSVC.Name, coreSVC.Spec.Ports[0].Port)
	coreIng.Annotations["custom"] = "annotation"
	coreIng.Labels["custom"] = "label"

	return operatorv1beta1.NetworkConfigurationList{
		CoreConfig: &coreIng,
	}
}

func (r *TestResources) newNetworkConfigurationV1Beta1(svcName string, svcPort int32) operatorv1beta1.NetworkConfiguration {
	pathtype := netv1.PathTypePrefix
	host := svcName + ".example.com"

	var ingressTLS []netv1.IngressTLS
	if r.ExternalTLS {
		ingressTLS = []netv1.IngressTLS{{}}
	}
	return operatorv1beta1.NetworkConfiguration{
		Annotations: map[string]string{"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS"},
		Labels:      map[string]string{"my": "label"},
		IngressSpec: &netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathtype,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: svcName,
											Port: netv1.ServiceBackendPort{
												Number: svcPort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: ingressTLS,
		},
	}
}
