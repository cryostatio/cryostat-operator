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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	securityv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	authzv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type TestResources struct {
	Name             string
	Namespace        string
	TLS              bool
	ExternalTLS      bool
	OpenShift        bool
	ReportReplicas   int32
	TargetNamespaces []string
	InsightsURL      string
}

func NewTestScheme() *runtime.Scheme {
	s := scheme.Scheme

	// Add all APIs used by the operator to the scheme
	sb := runtime.NewSchemeBuilder(
		operatorv1beta2.AddToScheme,
		certv1.AddToScheme,
		routev1.AddToScheme,
		consolev1.AddToScheme,
	)
	err := sb.AddToScheme(s)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return s
}

func NewTESTRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		certv1.SchemeGroupVersion,
	})
	// Add cert-manager Issuer GVK
	mapper.Add(schema.GroupVersionKind{
		Group:   certv1.SchemeGroupVersion.Group,
		Version: certv1.SchemeGroupVersion.Version,
		Kind:    certv1.IssuerKind,
	}, meta.RESTScopeNamespace)
	return mapper
}

func (r *TestResources) NewCryostat() *model.CryostatInstance {
	return r.ConvertNamespacedToModel(r.newCryostat())
}

func (r *TestResources) newCryostat() *operatorv1beta2.Cryostat {
	return &operatorv1beta2.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: r.newCryostatSpec(),
	}
}

func (r *TestResources) newCryostatSpec() operatorv1beta2.CryostatSpec {
	certManager := true
	var reportOptions *operatorv1beta2.ReportConfiguration
	if r.ReportReplicas > 0 {
		reportOptions = &operatorv1beta2.ReportConfiguration{
			Replicas: r.ReportReplicas,
		}
	}
	return operatorv1beta2.CryostatSpec{
		TargetNamespaces:  r.TargetNamespaces,
		EnableCertManager: &certManager,
		ReportOptions:     reportOptions,
	}
}

func (r *TestResources) ConvertNamespacedToModel(cr *operatorv1beta2.Cryostat) *model.CryostatInstance {
	return &model.CryostatInstance{
		Name:                  cr.Name,
		InstallNamespace:      cr.Namespace,
		TargetNamespaces:      cr.Spec.TargetNamespaces,
		TargetNamespaceStatus: &cr.Status.TargetNamespaces,
		Spec:                  &cr.Spec,
		Status:                &cr.Status,
		Object:                cr,
	}
}

func (r *TestResources) NewCryostatWithSecrets() *model.CryostatInstance {
	cr := r.NewCryostat()
	key := "test.crt"
	cr.Spec.TrustedCertSecrets = []operatorv1beta2.CertificateSecret{
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

func (r *TestResources) NewCryostatWithTemplates() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.EventTemplates = []operatorv1beta2.TemplateConfigMap{
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

func (r *TestResources) NewCryostatWithIngress() *model.CryostatInstance {
	return r.addIngressToCryostat(r.NewCryostat())
}

func (r *TestResources) NewCryostatWithIngressCertManagerDisabled() *model.CryostatInstance {
	return r.addIngressToCryostat(r.NewCryostatCertManagerDisabled())
}

func (r *TestResources) addIngressToCryostat(cr *model.CryostatInstance) *model.CryostatInstance {
	networkConfig := r.newNetworkConfigurationList()
	cr.Spec.NetworkOptions = &networkConfig
	return cr
}

func (r *TestResources) NewCryostatWithPVCSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfiguration{
		PVC: &operatorv1beta2.PersistentVolumeClaimConfig{
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

func (r *TestResources) NewCryostatWithPVCSpecSomeDefault() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfiguration{
		PVC: &operatorv1beta2.PersistentVolumeClaimConfig{
			Spec: newPVCSpec("", "1Gi"),
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithPVCLabelsOnly() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfiguration{
		PVC: &operatorv1beta2.PersistentVolumeClaimConfig{
			Labels: map[string]string{
				"my": "label",
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithDefaultEmptyDir() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfiguration{
		EmptyDir: &operatorv1beta2.EmptyDirConfig{
			Enabled: true,
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithEmptyDirSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta2.StorageConfiguration{
		EmptyDir: &operatorv1beta2.EmptyDirConfig{
			Enabled:   true,
			Medium:    "Memory",
			SizeLimit: "200Mi",
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithCoreSvc() *model.CryostatInstance {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	cr := r.NewCryostat()
	cr.Spec.ServiceOptions = &operatorv1beta2.ServiceConfigList{
		CoreConfig: &operatorv1beta2.CoreServiceConfig{
			HTTPPort: &httpPort,
			ServiceConfig: operatorv1beta2.ServiceConfig{
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

func (r *TestResources) NewCryostatWithReportsSvc() *model.CryostatInstance {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(13161)
	cr := r.NewCryostat()
	cr.Spec.ServiceOptions = &operatorv1beta2.ServiceConfigList{
		ReportsConfig: &operatorv1beta2.ReportsServiceConfig{
			HTTPPort: &httpPort,
			ServiceConfig: operatorv1beta2.ServiceConfig{
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

func (r *TestResources) NewCryostatWithAgentSvc() *model.CryostatInstance {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	cr := r.NewCryostat()
	cr.Spec.ServiceOptions = &operatorv1beta2.ServiceConfigList{
		AgentConfig: &operatorv1beta2.AgentServiceConfig{
			HTTPPort: &httpPort,
			ServiceConfig: operatorv1beta2.ServiceConfig{
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

func (r *TestResources) NewCryostatWithCoreNetworkOptions() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.NetworkOptions = &operatorv1beta2.NetworkConfigurationList{
		CoreConfig: &operatorv1beta2.NetworkConfiguration{
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

func (r *TestResources) NewCryostatWithCoreRouteHost() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.NetworkOptions = &operatorv1beta2.NetworkConfigurationList{
		CoreConfig: &operatorv1beta2.NetworkConfiguration{
			ExternalHost: &[]string{"cryostat.example.com"}[0],
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportsResources() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{
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

func (r *TestResources) NewCryostatWithReportLowResourceLimit() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{
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

func populateCryostatWithScheduling() *operatorv1beta2.SchedulingConfiguration {
	return &operatorv1beta2.SchedulingConfiguration{
		NodeSelector: map[string]string{"node": "good"},
		Affinity: &operatorv1beta2.Affinity{
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

func (r *TestResources) NewCryostatWithScheduling() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.SchedulingOptions = populateCryostatWithScheduling()
	return cr
}

func (r *TestResources) NewCryostatWithReportsScheduling() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{
		Replicas:          1,
		SchedulingOptions: populateCryostatWithScheduling(),
	}

	return cr
}

func (r *TestResources) NewCryostatCertManagerDisabled() *model.CryostatInstance {
	cr := r.NewCryostat()
	certManager := false
	cr.Spec.EnableCertManager = &certManager
	return cr
}

func (r *TestResources) NewCryostatCertManagerUndefined() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.EnableCertManager = nil
	return cr
}

func (r *TestResources) NewCryostatWithResources() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.Resources = &operatorv1beta2.ResourceConfigList{
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
		ObjectStorageResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("600m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("300m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		DatabaseResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		AuthProxyResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("80m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("40m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		},
		AgentProxyResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("60m"),
				corev1.ResourceMemory: resource.MustParse("160Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("30m"),
				corev1.ResourceMemory: resource.MustParse("80Mi"),
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithLowResourceLimit() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.Resources = &operatorv1beta2.ResourceConfigList{
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
		ObjectStorageResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("40m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		DatabaseResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
		},
		AuthProxyResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20m"),
				corev1.ResourceMemory: resource.MustParse("40Mi"),
			},
		},
		AgentProxyResources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("15m"),
				corev1.ResourceMemory: resource.MustParse("45Mi"),
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithBuiltInDiscoveryDisabled() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta2.TargetDiscoveryOptions{
		DisableBuiltInDiscovery: true,
	}
	return cr
}

func (r *TestResources) NewCryostatWithDiscoveryPortConfig() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta2.TargetDiscoveryOptions{
		DiscoveryPortNames:   []string{"custom-port-name", "another-custom-port-name"},
		DiscoveryPortNumbers: []int32{9092, 9090},
	}
	return cr
}

func (r *TestResources) NewCryostatWithBuiltInPortConfigDisabled() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta2.TargetDiscoveryOptions{
		DisableBuiltInPortNames:   true,
		DisableBuiltInPortNumbers: true,
	}
	return cr
}

func newPVCSpec(storageClass string, storageRequest string,
	accessModes ...corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaimSpec {
	return &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
		AccessModes:      accessModes,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(storageRequest),
			},
		},
	}
}

func (r *TestResources) NewCryostatWithJmxCacheOptionsSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.TargetConnectionCacheOptions = &operatorv1beta2.TargetConnectionCacheOptions{
		TargetCacheSize: 10,
		TargetCacheTTL:  20,
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportSubprocessHeapSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	if cr.Spec.ReportOptions == nil {
		cr.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{}
	}
	cr.Spec.ReportOptions.SubProcessMaxHeapSize = 500
	return cr
}

func (r *TestResources) NewCryostatWithSecurityOptions() *model.CryostatInstance {
	cr := r.NewCryostat()
	privEscalation := true
	nonRoot := false
	runAsUser := int64(0)
	fsGroup := int64(20000)
	cr.Spec.SecurityOptions = &operatorv1beta2.SecurityOptions{
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
		StorageSecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
		DatabaseSecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
		AuthProxySecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
		AgentProxySecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &privEscalation,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{},
			},
			RunAsUser: &runAsUser,
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportSecurityOptions() *model.CryostatInstance {
	cr := r.NewCryostat()
	nonRoot := true
	privEscalation := false
	runAsUser := int64(1002)
	if cr.Spec.ReportOptions == nil {
		cr.Spec.ReportOptions = &operatorv1beta2.ReportConfiguration{}
	}
	cr.Spec.ReportOptions.SecurityOptions = &operatorv1beta2.ReportsSecurityOptions{
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

var providedDatabaseSecretName string = "credentials-database-secret"

func (r *TestResources) NewCryostatWithDatabaseSecretProvided() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.DatabaseOptions = &operatorv1beta2.DatabaseOptions{
		SecretName: &providedDatabaseSecretName,
	}
	return cr
}

func (r *TestResources) NewCryostatWithAdditionalMetadata() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.OperandMetadata = &operatorv1beta2.OperandMetadata{
		DeploymentMetadata: &operatorv1beta2.ResourceMetadata{
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
		PodMetadata: &operatorv1beta2.ResourceMetadata{
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

func (r *TestResources) NewCryostatService() *corev1.Service {
	appProtocol := "http"
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":                         r.Name,
				"component":                   "cryostat",
				"app.kubernetes.io/name":      "cryostat",
				"app.kubernetes.io/instance":  r.Name,
				"app.kubernetes.io/component": "cryostat",
				"app.kubernetes.io/part-of":   "cryostat",
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
					Name:        "http",
					Port:        4180,
					TargetPort:  intstr.FromInt(4180),
					AppProtocol: &appProtocol,
				},
			},
		},
	}
}

func (r *TestResources) NewReportsService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-reports",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":                         r.Name,
				"component":                   "reports",
				"app.kubernetes.io/name":      "cryostat",
				"app.kubernetes.io/instance":  r.Name,
				"app.kubernetes.io/component": "reports",
				"app.kubernetes.io/part-of":   "cryostat",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":       r.Name,
				"component": "reports",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       10000,
					TargetPort: intstr.FromInt(10000),
				},
			},
		},
	}
}

func (r *TestResources) NewAgentProxyService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-agent",
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":                         r.Name,
				"component":                   "cryostat-agent-gateway",
				"app.kubernetes.io/name":      "cryostat",
				"app.kubernetes.io/instance":  r.Name,
				"app.kubernetes.io/component": "cryostat-agent-gateway",
				"app.kubernetes.io/part-of":   "cryostat",
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
					Port:       8282,
					TargetPort: intstr.FromInt(8282),
				},
			},
		},
	}
}

func (r *TestResources) NewCustomizedCoreService() *corev1.Service {
	svc := r.NewCryostatService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 8080
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":                         r.Name,
		"component":                   "cryostat",
		"my":                          "label",
		"app.kubernetes.io/name":      "cryostat",
		"app.kubernetes.io/instance":  r.Name,
		"app.kubernetes.io/component": "cryostat",
		"app.kubernetes.io/part-of":   "cryostat",
	}
	return svc
}

func (r *TestResources) NewCustomizedReportsService() *corev1.Service {
	svc := r.NewReportsService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 13161
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":                         r.Name,
		"component":                   "reports",
		"my":                          "label",
		"app.kubernetes.io/name":      "cryostat",
		"app.kubernetes.io/instance":  r.Name,
		"app.kubernetes.io/component": "reports",
		"app.kubernetes.io/part-of":   "cryostat",
	}
	return svc
}

func (r *TestResources) NewCustomizedAgentService() *corev1.Service {
	svc := r.NewAgentProxyService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 8080
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":                         r.Name,
		"component":                   "cryostat-agent-gateway",
		"my":                          "label",
		"app.kubernetes.io/name":      "cryostat",
		"app.kubernetes.io/instance":  r.Name,
		"app.kubernetes.io/component": "cryostat-agent-gateway",
		"app.kubernetes.io/part-of":   "cryostat",
	}
	return svc
}

func (r *TestResources) NewTestService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: r.Namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "1.2.3.4",
			Ports: []corev1.ServicePort{
				{
					Name: "test",
					Port: 4180,
				},
			},
		},
	}
}

func (r *TestResources) NewCACertSecret(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getClusterUniqueNameForCA(),
			Namespace: ns,
			Labels: map[string]string{
				"operator.cryostat.io/name":      r.Name,
				"operator.cryostat.io/namespace": r.Namespace,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			corev1.TLSCertKey: []byte(r.Name + "-ca-bytes"),
		},
	}
}

func (r *TestResources) NewAgentCertSecret(ns string) *corev1.Secret {
	name := r.GetClusterUniqueNameForAgent(ns)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
		},
		Data: map[string][]byte{
			corev1.TLSPrivateKeyKey: []byte(name + "-key"),
			corev1.TLSCertKey:       []byte(name + "-bytes"),
		},
	}
}

func (r *TestResources) NewAgentCertSecretCopy(ns string) *corev1.Secret {
	secret := r.NewAgentCertSecret(ns)
	secret.Labels = map[string]string{
		"operator.cryostat.io/name":      r.Name,
		"operator.cryostat.io/namespace": r.Namespace,
	}
	secret.Namespace = ns
	return secret
}

func (r *TestResources) NewDatabaseSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-db",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CONNECTION_KEY": "connection_key",
			"ENCRYPTION_KEY": "encryption_key",
		},
		Immutable: &[]bool{true}[0],
	}
}

func (r *TestResources) NewCustomDatabaseSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providedDatabaseSecretName,
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CONNECTION_KEY": "custom-connection_database",
			"ENCRYPTION_KEY": "custom-encryption_key",
		},
	}
}

func (r *TestResources) NewStorageSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-storage",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"SECRET_KEY": "object_storage",
		},
	}
}

func (r *TestResources) OtherDatabaseSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-db",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CONNECTION_KEY": "other-pass",
			"ENCRYPTION_KEY": "other-key",
		},
	}
}

func (r *TestResources) NewKeystoreSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-keystore",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"KEYSTORE_PASS": "keystore",
		},
	}
}

func (r *TestResources) NewTestCertSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
		},
		Data: map[string][]byte{
			corev1.TLSCertKey: []byte(name + "-bytes"),
		},
	}
}

func (r *TestResources) NewCryostatCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat",
			DNSNames: []string{
				r.Name,
				fmt.Sprintf(r.Name+".%s.svc", r.Namespace),
				fmt.Sprintf(r.Name+".%s.svc.cluster.local", r.Namespace),
			},
			SecretName: r.Name + "-tls",
			Keystores: &certv1.CertificateKeystores{
				PKCS12: &certv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: certMeta.SecretKeySelector{
						LocalObjectReference: certMeta.LocalObjectReference{
							Name: r.Name + "-keystore",
						},
						Key: "KEYSTORE_PASS",
					},
				},
			},
			IssuerRef: certMeta.ObjectReference{
				Name: r.Name + "-ca",
			},
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
				certv1.UsageClientAuth,
			},
		},
	}
}

func (r *TestResources) OtherCryostatCert() *certv1.Certificate {
	cert := r.NewCryostatCert()
	cert.Spec.CommonName = fmt.Sprintf("%s.%s.svc", r.Name, r.Namespace)
	return cert
}

func (r *TestResources) NewReportsCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-reports",
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat-reports",
			DNSNames: []string{
				r.Name + "-reports",
				fmt.Sprintf(r.Name+"-reports.%s.svc", r.Namespace),
				fmt.Sprintf(r.Name+"-reports.%s.svc.cluster.local", r.Namespace),
			},
			SecretName: r.Name + "-reports-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: r.Name + "-ca",
			},
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
			},
		},
	}
}

func (r *TestResources) OtherReportsCert() *certv1.Certificate {
	cert := r.NewReportsCert()
	cert.Spec.CommonName = fmt.Sprintf("%s-reports.%s.svc", r.Name, r.Namespace)
	return cert
}

func (r *TestResources) NewAgentProxyCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-agent-proxy",
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat-agent-proxy",
			DNSNames: []string{
				r.Name + "-agent",
				fmt.Sprintf(r.Name+"-agent.%s.svc", r.Namespace),
				fmt.Sprintf(r.Name+"-agent.%s.svc.cluster.local", r.Namespace),
			},
			SecretName: r.Name + "-agent-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: r.Name + "-ca",
			},
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
			},
		},
	}
}

func (r *TestResources) OtherAgentProxyCert() *certv1.Certificate {
	cert := r.NewAgentProxyCert()
	cert.Spec.CommonName = fmt.Sprintf("%s-agent.%s.svc", r.Name, r.Namespace)
	return cert
}

func (r *TestResources) NewCACert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-ca",
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat-ca-cert-manager",
			SecretName: r.getClusterUniqueNameForCA(),
			IssuerRef: certMeta.ObjectReference{
				Name: r.Name + "-self-signed",
			},
			IsCA: true,
		},
	}
}

func (r *TestResources) OtherCACert() *certv1.Certificate {
	cert := r.NewCACert()
	cert.Spec.CommonName = fmt.Sprintf("ca.%s.cert-manager", r.Name)
	cert.Spec.SecretName = r.Name + "-ca"
	return cert
}

func (r *TestResources) NewAgentCert(namespace string) *certv1.Certificate {
	name := r.GetClusterUniqueNameForAgent(namespace)
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat-agent",
			DNSNames: []string{
				fmt.Sprintf("*.%s.%s.svc", r.GetAgentServiceName(), namespace),
			},
			SecretName: name,
			IssuerRef: certMeta.ObjectReference{
				Name: r.Name + "-ca",
			},
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
				certv1.UsageClientAuth,
			},
		},
	}
}

func (r *TestResources) NewSelfSignedIssuer() *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-self-signed",
			Namespace: r.Namespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				SelfSigned: &certv1.SelfSignedIssuer{},
			},
		},
	}
}

func (r *TestResources) NewCryostatCAIssuer() *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-ca",
			Namespace: r.Namespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				CA: &certv1.CAIssuer{
					SecretName: r.getClusterUniqueNameForCA(),
				},
			},
		},
	}
}

func (r *TestResources) OtherCAIssuer() *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-ca",
			Namespace: r.Namespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				CA: &certv1.CAIssuer{
					SecretName: r.Name + "-ca",
				},
			},
		},
	}
}

func (r *TestResources) newPVC(spec *corev1.PersistentVolumeClaimSpec, labels map[string]string,
	annotations map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.Name,
			Namespace:   r.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: *spec,
	}
}

func (r *TestResources) NewDefaultPVC() *corev1.PersistentVolumeClaim {
	return r.newPVC(&corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("500Mi"),
			},
		},
	}, map[string]string{
		"app": r.Name,
	}, nil)
}

func (r *TestResources) NewCustomPVC() *corev1.PersistentVolumeClaim {
	storageClass := "cool-storage"
	return r.newPVC(&corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
		},
	}, map[string]string{
		"my":  "label",
		"app": r.Name,
	}, map[string]string{
		"my/custom": "annotation",
	})
}

func (r *TestResources) NewCustomPVCSomeDefault() *corev1.PersistentVolumeClaim {
	storageClass := ""
	return r.newPVC(&corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			},
		},
	}, map[string]string{
		"app": r.Name,
	}, nil)
}

func (r *TestResources) NewDefaultPVCWithLabel() *corev1.PersistentVolumeClaim {
	return r.newPVC(&corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("500Mi"),
			},
		},
	}, map[string]string{
		"app": r.Name,
		"my":  "label",
	}, nil)
}

func (r *TestResources) NewDefaultEmptyDir() *corev1.EmptyDirVolumeSource {
	sizeLimit := resource.MustParse("0")
	return &corev1.EmptyDirVolumeSource{
		SizeLimit: &sizeLimit,
	}
}

func (r *TestResources) NewEmptyDirWithSpec() *corev1.EmptyDirVolumeSource {
	sizeLimit := resource.MustParse("200Mi")
	return &corev1.EmptyDirVolumeSource{
		Medium:    "Memory",
		SizeLimit: &sizeLimit,
	}
}

func (r *TestResources) NewCorePorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 8181,
		},
	}
}

func (r *TestResources) NewGrafanaPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 3000,
		},
	}
}

func (r *TestResources) NewDatasourcePorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 8989,
		},
	}
}

func (r *TestResources) NewReportsPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 10000,
		},
	}
}

func (r *TestResources) NewStoragePorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 8333,
		},
	}
}

func (r *TestResources) NewDatabasePorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 5432,
		},
	}
}

func (r *TestResources) NewAuthProxyPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 4180,
		},
	}
}

func (r *TestResources) NewAgentProxyPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 8281,
		},
		{
			ContainerPort: 8282,
		},
	}
}

func (r *TestResources) NewCoreEnvironmentVariables(reportsUrl string, ingress bool,
	emptyDir bool, hasPortConfig bool, builtInDiscoveryDisabled bool, builtInPortConfigDisabled bool, dbSecretProvided bool) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: "localhost",
		},
		{
			Name:  "QUARKUS_HTTP_PORT",
			Value: "8181",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_PROXY_ADDRESS_FORWARDING",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_ALLOW_X_FORWARDED",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_ENABLE_FORWARDED_HOST",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HTTP_PROXY_ENABLE_FORWARDED_PREFIX",
			Value: "true",
		},
		{
			Name:  "QUARKUS_HIBERNATE_ORM_DATABASE_GENERATION",
			Value: "none",
		},
		{
			Name:  "QUARKUS_HIBERNATE_ORM_SQL_LOAD_SCRIPT",
			Value: "no-file",
		},
		{
			Name:  "QUARKUS_DATASOURCE_USERNAME",
			Value: "cryostat",
		},
		{
			Name:  "QUARKUS_DATASOURCE_JDBC_URL",
			Value: "jdbc:postgresql://localhost:5432/cryostat",
		},
		{
			Name:  "STORAGE_BUCKETS_ARCHIVE_NAME",
			Value: "archivedrecordings",
		},
		{
			Name:  "QUARKUS_S3_ENDPOINT_OVERRIDE",
			Value: "http://localhost:8333",
		},
		{
			Name:  "QUARKUS_S3_PATH_STYLE_ACCESS",
			Value: "true",
		},
		{
			Name:  "QUARKUS_S3_AWS_REGION",
			Value: "us-east-1",
		},
		{
			Name:  "QUARKUS_S3_AWS_CREDENTIALS_TYPE",
			Value: "static",
		},
		{
			Name:  "QUARKUS_S3_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID",
			Value: "cryostat",
		},
		{
			Name:  "AWS_ACCESS_KEY_ID",
			Value: "$(QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID)",
		},
		{
			Name:  "CRYOSTAT_CONFIG_PATH",
			Value: "/opt/cryostat.d/conf.d",
		},
		{
			Name:  "CRYOSTAT_TEMPLATE_PATH",
			Value: "/opt/cryostat.d/templates.d",
		},
		{
			Name:  "CRYOSTAT_CONNECTIONS_MAX_OPEN",
			Value: "-1",
		},
		{
			Name:  "CRYOSTAT_CONNECTIONS_TTL",
			Value: "10",
		},
		{
			Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_NAMESPACES",
			Value: strings.Join(r.TargetNamespaces, ","),
		},
		{
			Name:  "GRAFANA_DATASOURCE_URL",
			Value: "http://127.0.0.1:8989",
		},
		{
			Name:  "QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_ACCESS_KEY_ID",
			Value: "cryostat",
		},
		{
			Name:  "AWS_SECRET_ACCESS_KEY",
			Value: "$(QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_SECRET_ACCESS_KEY)",
		},
	}

	envs = append(envs, r.NewTargetDiscoveryEnvVars(hasPortConfig, builtInDiscoveryDisabled, builtInPortConfigDisabled)...)

	optional := false
	secretName := r.NewDatabaseSecret().Name
	if dbSecretProvided {
		secretName = providedDatabaseSecretName
	}
	envs = append(envs, corev1.EnvVar{
		Name: "QUARKUS_DATASOURCE_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      "CONNECTION_KEY",
				Optional: &optional,
			},
		},
	},
	)

	secretName = r.NewStorageSecret().Name
	envs = append(envs, corev1.EnvVar{
		Name: "QUARKUS_S3_AWS_CREDENTIALS_STATIC_PROVIDER_SECRET_ACCESS_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      "SECRET_KEY",
				Optional: &optional,
			},
		},
	},
	)

	if r.OpenShift || ingress {
		envs = append(envs, r.newNetworkEnvironmentVariables()...)
	}

	if reportsUrl != "" {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_SERVICES_REPORTS_URL",
				Value: reportsUrl,
			})
	}

	if len(r.InsightsURL) > 0 {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "INSIGHTS_PROXY",
				Value: r.InsightsURL,
			})
	}
	return envs
}

func (r *TestResources) newNetworkEnvironmentVariables() []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "GRAFANA_DASHBOARD_URL",
			Value: "http://localhost:3000",
		},
		{
			Name:  "GRAFANA_DASHBOARD_EXT_URL",
			Value: "/grafana/",
		},
	}
	return envs
}

func (r *TestResources) NewGrafanaEnvironmentVariables() []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "JFR_DATASOURCE_URL",
			Value: "http://127.0.0.1:8989",
		},
		{
			Name:      "GF_AUTH_ANONYMOUS_ENABLED",
			Value:     "true",
			ValueFrom: nil,
		},
		{
			Name:      "GF_SERVER_DOMAIN",
			Value:     "localhost",
			ValueFrom: nil,
		},
		{
			Name:      "GF_SERVER_SERVE_FROM_SUB_PATH",
			Value:     "true",
			ValueFrom: nil,
		},
		{
			Name:  "GF_SERVER_ROOT_URL",
			Value: "http://localhost:4180/grafana/",
		},
	}
	return envs
}

func (r *TestResources) NewDatasourceEnvironmentVariables() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: "127.0.0.1",
		},
		{
			Name:  "QUARKUS_HTTP_PORT",
			Value: "8989",
		},
	}
}

func (r *TestResources) NewReportsEnvironmentVariables(resources *corev1.ResourceRequirements) []corev1.EnvVar {
	cpus := resources.Requests.Cpu().Value()
	if limit := resources.Limits; limit != nil {
		if cpu := limit.Cpu(); limit != nil {
			cpus = cpu.Value()
		}
	}
	opts := fmt.Sprintf("-XX:+PrintCommandLineFlags -XX:ActiveProcessorCount=%d -Dorg.openjdk.jmc.flightrecorder.parser.singlethreaded=%t", cpus, cpus < 2)
	envs := []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "JAVA_OPTS",
			Value: opts,
		},
	}
	if r.TLS {
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_PORT",
			Value: "10000",
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_KEY_FILES",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-reports-tls/tls.key", r.Name),
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_FILES",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-reports-tls/tls.crt", r.Name),
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_INSECURE_REQUESTS",
			Value: "disabled",
		})
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_PORT",
			Value: "10000",
		})
	}
	return envs
}

func (r *TestResources) NewStorageEnvironmentVariables() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_BUCKETS",
			Value: "archivedrecordings,archivedreports,eventtemplates,probes",
		},
		{
			Name:  "CRYOSTAT_ACCESS_KEY",
			Value: "cryostat",
		},
		{
			Name:  "DATA_DIR",
			Value: "/data",
		},
		{
			Name:  "IP_BIND",
			Value: "0.0.0.0",
		},
		{
			Name:  "REST_ENCRYPTION_ENABLE",
			Value: "1",
		},
		{
			Name: "CRYOSTAT_SECRET_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.Name + "-storage",
					},
					Key:      "SECRET_KEY",
					Optional: &[]bool{false}[0],
				},
			},
		},
	}
}

func (r *TestResources) NewDatabaseEnvironmentVariables(dbSecretProvided bool) []corev1.EnvVar {
	optional := false
	secretName := r.Name + "-db"
	if dbSecretProvided {
		secretName = providedDatabaseSecretName
	}
	return []corev1.EnvVar{
		{
			Name:  "POSTGRESQL_USER",
			Value: "cryostat",
		},
		{
			Name:  "POSTGRESQL_DATABASE",
			Value: "cryostat",
		},
		{
			Name: "POSTGRESQL_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key:      "CONNECTION_KEY",
					Optional: &optional,
				},
			},
		},
		{
			Name: "PG_ENCRYPT_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key:      "ENCRYPTION_KEY",
					Optional: &optional,
				},
			},
		},
	}
}

func (r *TestResources) NewAuthProxyEnvironmentVariables(authOptions *operatorv1beta2.AuthorizationOptions) []corev1.EnvVar {
	envs := []corev1.EnvVar{}

	if !r.OpenShift {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "OAUTH2_PROXY_REDIRECT_URL",
				Value: "http://localhost:4180/oauth2/callback",
			},
			corev1.EnvVar{
				Name:  "OAUTH2_PROXY_EMAIL_DOMAINS",
				Value: "*",
			},
		)

		basicAuthConfigured := authOptions != nil && authOptions.BasicAuth != nil &&
			authOptions.BasicAuth.Filename != nil && authOptions.BasicAuth.SecretName != nil
		if basicAuthConfigured {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "OAUTH2_PROXY_HTPASSWD_FILE",
					Value: "/var/run/secrets/operator.cryostat.io/" + *authOptions.BasicAuth.Filename,
				},
				corev1.EnvVar{
					Name:  "OAUTH2_PROXY_HTPASSWD_USER_GROUP",
					Value: "write",
				},
				corev1.EnvVar{
					Name:  "OAUTH2_PROXY_SKIP_AUTH_ROUTES",
					Value: "^/health(/liveness)?$",
				},
			)
		} else {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "OAUTH2_PROXY_SKIP_AUTH_ROUTES",
					Value: ".*",
				})
		}
	}

	return envs
}

func (r *TestResources) NewAgentProxyEnvironmentVariables() []corev1.EnvVar {
	return []corev1.EnvVar{}
}

func (r *TestResources) NewAuthProxyEnvFromSource() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.Name + "-oauth2-cookie",
				},
				Optional: &[]bool{false}[0],
			},
		},
	}
}

func (r *TestResources) NewAgentProxyEnvFromSource() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{}
}

func (r *TestResources) NewCoreEnvFromSource() []corev1.EnvFromSource {
	envsFrom := []corev1.EnvFromSource{}
	return envsFrom
}

func (r *TestResources) NewJmxCacheOptionsEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_CONNECTIONS_MAX_OPEN",
			Value: "10",
		},
		{
			Name:  "CRYOSTAT_CONNECTIONS_TTL",
			Value: "20",
		},
	}
}

func (r *TestResources) NewTargetDiscoveryEnvVars(hasPortConfig bool, builtInDiscoveryDisabled bool, builtInPortConfigDisabled bool) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_ENABLED",
			Value: fmt.Sprintf("%t", !builtInDiscoveryDisabled),
		},
	}

	if hasPortConfig {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NAMES",
				Value: "custom-port-name,another-custom-port-name",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NUMBERS",
				Value: "9092,9090",
			},
		)
	} else if builtInPortConfigDisabled {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NAMES",
				Value: "",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NUMBERS",
				Value: "",
			},
		)
	} else {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NAMES",
				Value: "jfr-jmx",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISCOVERY_KUBERNETES_PORT_NUMBERS",
				Value: "9091",
			},
		)
	}
	return envs
}

func (r *TestResources) NewAuthProxyArguments(authOptions *operatorv1beta2.AuthorizationOptions) ([]string, error) {
	if !r.OpenShift {
		return []string{
			"--alpha-config=/etc/oauth2_proxy/alpha_config/alpha_config.json",
		}, nil
	}

	basicAuthConfigured := authOptions != nil && authOptions.BasicAuth != nil &&
		authOptions.BasicAuth.Filename != nil && authOptions.BasicAuth.SecretName != nil

	openShiftSSOConfigured := authOptions != nil && authOptions.OpenShiftSSO != nil
	openShiftSSODisabled := openShiftSSOConfigured && authOptions.OpenShiftSSO.Disable != nil && *authOptions.OpenShiftSSO.Disable

	accessReview := authzv1.ResourceAttributes{
		Namespace:   r.Namespace,
		Verb:        "create",
		Group:       "",
		Version:     "",
		Resource:    "pods",
		Subresource: "exec",
		Name:        "",
	}
	if openShiftSSOConfigured && authOptions.OpenShiftSSO.AccessReview != nil {
		accessReview = *authOptions.OpenShiftSSO.AccessReview
	}

	subjectAccessReviewJson, err := json.Marshal([]authzv1.ResourceAttributes{accessReview})
	if err != nil {
		return nil, err
	}

	delegateUrls := make(map[string]authzv1.ResourceAttributes)
	delegateUrls["/"] = accessReview
	tokenReviewJson, err := json.Marshal(delegateUrls)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--pass-access-token=false",
		"--pass-user-bearer-token=false",
		"--pass-basic-auth=false",
		"--upstream=http://localhost:8181/",
		"--upstream=http://localhost:3000/grafana/",
		"--upstream=http://localhost:8333/storage/",
		fmt.Sprintf("--openshift-service-account=%s", r.Name),
		"--proxy-websockets=true",
		"--proxy-prefix=/oauth2",
		fmt.Sprintf("--skip-provider-button=%t", !basicAuthConfigured),
		fmt.Sprintf("--openshift-sar=%s", subjectAccessReviewJson),
		fmt.Sprintf("--openshift-delegate-urls=%s", string(tokenReviewJson)),
	}

	if openShiftSSODisabled {
		args = append(args, "--bypass-auth-for=.*")
	} else {
		args = append(args, "--bypass-auth-for=^/health(/liveness)?$")
	}

	if basicAuthConfigured {
		args = append(args, fmt.Sprintf("--htpasswd-file=%s/%s", "/var/run/secrets/operator.cryostat.io", *authOptions.BasicAuth.Filename))
	}

	if r.TLS {
		args = append(args,
			"--http-address=",
			"--https-address=0.0.0.0:4180",
			fmt.Sprintf("--tls-cert=/var/run/secrets/operator.cryostat.io/%s/%s", r.Name+"-tls", corev1.TLSCertKey),
			fmt.Sprintf("--tls-key=/var/run/secrets/operator.cryostat.io/%s/%s", r.Name+"-tls", corev1.TLSPrivateKeyKey),
		)
	} else {
		args = append(args,
			"--http-address=0.0.0.0:4180",
			"--https-address=",
		)
	}
	return args, nil
}

func (r *TestResources) NewAgentProxyCommand() []string {
	return []string{
		"nginx", "-c", "/etc/nginx-cryostat/nginx.conf", "-g", "daemon off;",
	}
}

func (r *TestResources) NewCoreVolumeMounts() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      r.Name,
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/conf.d",
			SubPath:   "config",
		},
		{
			Name:      r.Name,
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/templates.d",
			SubPath:   "templates",
		},
		{
			Name:      r.Name,
			ReadOnly:  false,
			MountPath: "truststore",
			SubPath:   "truststore",
		},
		{
			Name:      "cert-secrets",
			ReadOnly:  true,
			MountPath: "/truststore/operator",
		},
	}
	return mounts
}

func (r *TestResources) NewStorageVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      r.Name,
			MountPath: "/data",
			SubPath:   "seaweed",
		},
	}
}

func (r *TestResources) NewDatabaseVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      r.Name,
			MountPath: "/data",
			SubPath:   "postgres",
		},
	}
}

func (r *TestResources) NewAuthProxyVolumeMounts(authOptions *operatorv1beta2.AuthorizationOptions) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	if r.TLS {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "auth-proxy-tls-secret",
			MountPath: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-tls", r.Name),
			ReadOnly:  true,
		})
	}

	basicAuthConfigured := authOptions != nil && authOptions.BasicAuth != nil &&
		authOptions.BasicAuth.Filename != nil && authOptions.BasicAuth.SecretName != nil
	if basicAuthConfigured {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      r.Name + "-auth-proxy-htpasswd",
			MountPath: "/var/run/secrets/operator.cryostat.io",
			ReadOnly:  true,
		})
	}

	if !r.OpenShift {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      r.Name + "-oauth2-proxy-cfg",
				MountPath: "/etc/oauth2_proxy/alpha_config",
				ReadOnly:  true,
			})

	}

	return mounts
}

func (r *TestResources) NewAgentProxyVolumeMounts() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	if r.TLS {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "agent-proxy-tls-secret",
			MountPath: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-agent-tls", r.Name),
			ReadOnly:  true,
		})
	}

	mounts = append(mounts,
		corev1.VolumeMount{
			Name:      "agent-proxy-config",
			MountPath: "/etc/nginx-cryostat",
			ReadOnly:  true,
		})

	return mounts
}

func (r *TestResources) NewReportsVolumeMounts() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	if r.TLS {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "reports-tls-secret",
				MountPath: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-reports-tls", r.Name),
				ReadOnly:  true,
			})
	}
	return mounts
}

func (r *TestResources) NewVolumeMountsWithTemplates() []corev1.VolumeMount {
	return append(r.NewCoreVolumeMounts(),
		corev1.VolumeMount{
			Name:      "template-templateCM1",
			ReadOnly:  true,
			MountPath: "/opt/cryostat.d/templates.d/templateCM1_template.jfc",
			SubPath:   "template.jfc",
		},
		corev1.VolumeMount{
			Name:      "template-templateCM2",
			ReadOnly:  true,
			MountPath: "/opt/cryostat.d/templates.d/templateCM2_other-template.jfc",
			SubPath:   "other-template.jfc",
		})
}

func (r *TestResources) NewVolumeMountsWithAuthProperties() []corev1.VolumeMount {
	return append(r.NewCoreVolumeMounts(), r.NewAuthPropertiesVolumeMount())
}

func (r *TestResources) NewAuthPropertiesVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "auth-properties-authConfigMapName",
		ReadOnly:  true,
		MountPath: "/app/resources/io/cryostat/net/openshift/OpenShiftAuthManager.properties",
		SubPath:   "OpenShiftAuthManager.properties",
	}
}

func (r *TestResources) NewCoreLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: r.newCoreProbeHandler(),
	}
}

func (r *TestResources) NewCoreStartupProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:     r.newCoreProbeHandler(),
		FailureThreshold: 18,
	}
}

func (r *TestResources) newCoreProbeHandler() corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Port:   intstr.IntOrString{IntVal: 8181},
			Path:   "/health/liveness",
			Scheme: corev1.URISchemeHTTP,
		},
	}
}

func (r *TestResources) NewGrafanaLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.IntOrString{IntVal: 3000},
				Path:   "/api/health",
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}
}

func (r *TestResources) NewDatasourceLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"curl", "--fail", "http://127.0.0.1:8989"},
			},
		},
	}
}

func (r *TestResources) NewStorageLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.IntOrString{IntVal: 8333},
				Path:   "/status",
				Scheme: corev1.URISchemeHTTP,
			},
		},
		FailureThreshold: 2,
	}
}

func (r *TestResources) NewDatabaseReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"pg_isready", "-U", "cryostat", "-d", "cryostat"},
			},
		},
	}
}

func (r *TestResources) NewAuthProxyLivenessProbe() *corev1.Probe {
	protocol := corev1.URISchemeHTTP
	if r.TLS {
		protocol = corev1.URISchemeHTTPS
	}
	path := "/ping"
	if r.OpenShift {
		path = "/oauth2/healthz"
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.IntOrString{IntVal: 4180},
				Path:   path,
				Scheme: protocol,
			},
		},
	}
}

func (r *TestResources) NewAgentProxyLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.IntOrString{IntVal: 8281},
				Path:   "/healthz",
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}
}

func (r *TestResources) NewReportsLivenessProbe() *corev1.Probe {
	protocol := corev1.URISchemeHTTPS
	if !r.TLS {
		protocol = corev1.URISchemeHTTP
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.IntOrString{IntVal: 10000},
				Path:   "/health",
				Scheme: protocol,
			},
		},
	}
}

func (r *TestResources) NewMainDeploymentSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       r.Name,
			"kind":      "cryostat",
			"component": "cryostat",
		},
	}
}

func (r *TestResources) NewReportsDeploymentSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       r.Name,
			"kind":      "cryostat",
			"component": "reports",
		},
	}
}

func (r *TestResources) NewMainDeploymentStrategy() appsv1.DeploymentStrategy {
	return appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	}
}

func (r *TestResources) OtherDeployment() *appsv1.Deployment {
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":   "something-else",
				"other": "label",
			},
			Annotations: map[string]string{
				"app.openshift.io/connects-to": "something-else",
				"other":                        "annotation",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      r.Name,
					Namespace: r.Namespace,
					Labels: map[string]string{
						"app": "something-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "other-container",
							Image: "incorrect/image:latest",
						},
					},
				},
			},
			Selector: r.NewMainDeploymentSelector(),
			Replicas: &replicas,
		},
	}
}

func (r *TestResources) NewVolumes() []corev1.Volume {
	return r.newVolumes(nil)
}

func (r *TestResources) NewVolumesWithSecrets() []corev1.Volume {
	mode := int32(0440)
	return r.newVolumes([]corev1.VolumeProjection{
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "testCert1",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "test.crt",
						Path: "testCert1_test.crt",
						Mode: &mode,
					},
				},
			},
		},
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "testCert2",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "tls.crt",
						Path: "testCert2_tls.crt",
						Mode: &mode,
					},
				},
			},
		},
	})
}

func (r *TestResources) NewVolumesWithTemplates() []corev1.Volume {
	mode := int32(0440)
	return append(r.NewVolumes(),
		corev1.Volume{
			Name: "template-templateCM1",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "templateCM1",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "template.jfc",
							Path: "template.jfc",
							Mode: &mode,
						},
					},
				},
			},
		},
		corev1.Volume{
			Name: "template-templateCM2",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "templateCM2",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "other-template.jfc",
							Path: "other-template.jfc",
							Mode: &mode,
						},
					},
				},
			},
		})
}

func (r *TestResources) NewVolumeWithAuthProperties() []corev1.Volume {
	return append(r.NewVolumes(), r.NewAuthPropertiesVolume())
}

func (r *TestResources) NewAuthPropertiesVolume() corev1.Volume {
	readOnlyMode := int32(0440)
	return corev1.Volume{
		Name: "auth-properties-authConfigMapName",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "authConfigMapName",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "auth.properties",
						Path: "OpenShiftAuthManager.properties",
						Mode: &readOnlyMode,
					},
				},
			},
		},
	}
}

func (r *TestResources) newVolumes(certProjections []corev1.VolumeProjection) []corev1.Volume {
	readOnlymode := int32(0440)
	volumes := []corev1.Volume{
		{
			Name: r.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.Name,
					ReadOnly:  false,
				},
			},
		},
		{
			Name: "agent-proxy-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.Name + "-agent-proxy",
					},
					DefaultMode: &readOnlymode,
				},
			},
		},
	}
	projs := append([]corev1.VolumeProjection{}, certProjections...)
	if r.TLS {
		projs = append(projs, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.Name + "-tls",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "ca.crt",
						Path: r.Name + "-ca.crt",
						Mode: &readOnlymode,
					},
				},
			},
		})

		volumes = append(volumes,
			corev1.Volume{
				Name: "keystore",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: r.Name + "-tls",
						Items: []corev1.KeyToPath{
							{
								Key:  "keystore.p12",
								Path: "keystore.p12",
								Mode: &readOnlymode,
							},
						},
					},
				},
			},
			corev1.Volume{
				Name: "auth-proxy-tls-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  r.Name + "-tls",
						DefaultMode: &readOnlymode,
					},
				},
			},
			corev1.Volume{
				Name: "agent-proxy-tls-secret",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  r.Name + "-agent-tls",
						DefaultMode: &readOnlymode,
					},
				},
			},
		)
	}

	volumes = append(volumes,
		corev1.Volume{
			Name: "cert-secrets",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: projs,
				},
			},
		})

	if !r.OpenShift {
		readOnlyMode := int32(0440)
		volumes = append(volumes, corev1.Volume{
			Name: r.Name + "-oauth2-proxy-cfg",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.Name + "-oauth2-proxy-cfg",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "alpha_config.json",
							Path: "alpha_config.json",
							Mode: &readOnlyMode,
						},
					},
				},
			},
		})
	}

	return volumes
}

func (r *TestResources) NewReportsVolumes() []corev1.Volume {
	if !r.TLS {
		return nil
	}
	return []corev1.Volume{
		{
			Name: "reports-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.Name + "-reports-tls",
				},
			},
		},
	}
}

func (r *TestResources) commonDefaultPodSecurityContext(fsGroup *int64) *corev1.PodSecurityContext {
	nonRoot := true
	var seccompProfile *corev1.SeccompProfile
	if !r.OpenShift {
		seccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
	}
	return &corev1.PodSecurityContext{
		FSGroup:        fsGroup,
		RunAsNonRoot:   &nonRoot,
		SeccompProfile: seccompProfile,
	}
}

func (r *TestResources) commonDefaultSecurityContext() *corev1.SecurityContext {
	privEscalation := false
	return &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"ALL",
			},
		},
		AllowPrivilegeEscalation: &privEscalation,
	}
}

func (r *TestResources) NewPodSecurityContext(cr *model.CryostatInstance) *corev1.PodSecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.PodSecurityContext != nil {
		return cr.Spec.SecurityOptions.PodSecurityContext
	}
	fsGroup := int64(18500)
	return r.commonDefaultPodSecurityContext(&fsGroup)
}

func (r *TestResources) NewReportPodSecurityContext(cr *model.CryostatInstance) *corev1.PodSecurityContext {
	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SecurityOptions != nil && cr.Spec.ReportOptions.SecurityOptions.PodSecurityContext != nil {
		return cr.Spec.ReportOptions.SecurityOptions.PodSecurityContext
	}
	return r.commonDefaultPodSecurityContext(nil)
}

func (r *TestResources) NewCoreSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.CoreSecurityContext != nil {
		return cr.Spec.SecurityOptions.CoreSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewGrafanaSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.GrafanaSecurityContext != nil {
		return cr.Spec.SecurityOptions.GrafanaSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewDatasourceSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.DataSourceSecurityContext != nil {
		return cr.Spec.SecurityOptions.DataSourceSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewStorageSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.StorageSecurityContext != nil {
		return cr.Spec.SecurityOptions.StorageSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewDatabaseSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.DatabaseSecurityContext != nil {
		return cr.Spec.SecurityOptions.DatabaseSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewAuthProxySecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.AuthProxySecurityContext != nil {
		return cr.Spec.SecurityOptions.AuthProxySecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewAgentProxySecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.SecurityOptions != nil && cr.Spec.SecurityOptions.AgentProxySecurityContext != nil {
		return cr.Spec.SecurityOptions.AgentProxySecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewReportSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SecurityOptions != nil && cr.Spec.ReportOptions.SecurityOptions.ReportsSecurityContext != nil {
		return cr.Spec.ReportOptions.SecurityOptions.ReportsSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewCoreRoute() *routev1.Route {
	return r.newRoute(r.Name, 4180)
}

func (r *TestResources) NewCustomCoreRoute() *routev1.Route {
	route := r.NewCoreRoute()
	route.Annotations = map[string]string{"custom": "annotation"}
	route.Labels = map[string]string{
		"custom":    "label",
		"app":       r.Name,
		"component": "cryostat",
	}
	return route
}

func (r *TestResources) NewCustomHostCoreRoute() *routev1.Route {
	route := r.NewCoreRoute()
	route.Spec.Host = "cryostat.example.com"
	return route
}

func (r *TestResources) newRoute(name string, port int) *routev1.Route {
	var routeTLS *routev1.TLSConfig
	if !r.TLS {
		routeTLS = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		}
	} else {
		routeTLS = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationReencrypt,
			DestinationCACertificate:      r.Name + "-ca-bytes",
			InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		}
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":       r.Name,
				"component": "cryostat",
			},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: name,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(port),
			},
			TLS: routeTLS,
		},
	}
}

func (r *TestResources) OtherCoreRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.Name,
			Namespace:   r.Namespace,
			Annotations: map[string]string{"custom": "annotation"},
			Labels:      map[string]string{"custom": "label"},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: "some-other-service",
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(1234),
			},
			TLS: &routev1.TLSConfig{
				Termination:              routev1.TLSTerminationEdge,
				Certificate:              "foo",
				Key:                      "bar",
				DestinationCACertificate: "baz",
			},
		},
	}
}

func (r *TestResources) NewCoreIngress() *netv1.Ingress {
	return r.newIngress(r.Name, 4180, map[string]string{"custom": "annotation"},
		map[string]string{"my": "label", "custom": "label"})
}

func (r *TestResources) newIngress(name string, svcPort int32, annotations, labels map[string]string) *netv1.Ingress {
	pathtype := netv1.PathTypePrefix

	annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"
	labels["app"] = r.Name
	labels["component"] = "cryostat"

	var ingressTLS []netv1.IngressTLS
	if r.ExternalTLS {
		ingressTLS = []netv1.IngressTLS{{}}
	}
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   r.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: name + ".example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathtype,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: name,
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

func (r *TestResources) OtherCoreIngress() *netv1.Ingress {
	pathtype := netv1.PathTypePrefix
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.Name,
			Namespace:   r.Namespace,
			Annotations: map[string]string{"other": "annotation"},
			Labels:      map[string]string{"other": "label", "app": "not-cryostat"},
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "some-other-host.example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathtype,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "some-other-service",
											Port: netv1.ServiceBackendPort{
												Number: 2000,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *TestResources) newNetworkConfigurationList() operatorv1beta2.NetworkConfigurationList {
	coreSVC := r.NewCryostatService()
	coreIng := r.newNetworkConfiguration(coreSVC.Name, coreSVC.Spec.Ports[0].Port)
	coreIng.Annotations["custom"] = "annotation"
	coreIng.Labels["custom"] = "label"

	return operatorv1beta2.NetworkConfigurationList{
		CoreConfig: &coreIng,
	}
}

func (r *TestResources) newNetworkConfiguration(svcName string, svcPort int32) operatorv1beta2.NetworkConfiguration {
	pathtype := netv1.PathTypePrefix
	host := svcName + ".example.com"

	var ingressTLS []netv1.IngressTLS
	if r.ExternalTLS {
		ingressTLS = []netv1.IngressTLS{{}}
	}
	return operatorv1beta2.NetworkConfiguration{
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

func (r *TestResources) NewServiceAccount() *corev1.ServiceAccount {
	var annotations map[string]string
	if r.OpenShift {
		annotations = map[string]string{
			"serviceaccounts.openshift.io/oauth-redirectreference.route": fmt.Sprintf(`{"metadata":{"creationTimestamp":null},"reference":{"group":"","kind":"Route","name":"%s"}}`, r.Name),
		}
	}

	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app": r.Name,
			},
			Annotations: annotations,
		},
	}
}

func (r *TestResources) OtherServiceAccount() *corev1.ServiceAccount {
	disable := false
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app":   "not-cryostat",
				"other": "label",
			},
			Annotations: map[string]string{
				"hello": "world",
			},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: r.Name + "-dockercfg-abcde",
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: r.Name + "-dockercfg-abcde",
			},
			{
				Name: r.Name + "-token-abcde",
			},
		},
		AutomountServiceAccountToken: &disable,
	}
}

func (r *TestResources) NewRole() *rbacv1.Role {
	rules := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{""},
			Resources: []string{"endpoints"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{""},
			Resources: []string{"pods", "replicationcontrollers"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"apps"},
			Resources: []string{"replicasets", "deployments", "daemonsets", "statefulsets"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"apps.openshift.io"},
			Resources: []string{"deploymentconfigs"},
		},
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"route.openshift.io"},
			Resources: []string{"routes"},
		},
	}
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Rules: rules,
	}
}

func (r *TestResources) OtherRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"test": "label",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"*"},
				APIGroups: []string{"*"},
				Resources: []string{"*"},
			},
		},
	}
}

func (r *TestResources) NewRoleBinding(ns string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getClusterUniqueName(),
			Namespace: ns,
			Labels: map[string]string{
				"operator.cryostat.io/name":      r.Name,
				"operator.cryostat.io/namespace": r.Namespace,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      r.Name,
				Namespace: r.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cryostat-operator-cryostat-namespaced",
		},
	}
}

func (r *TestResources) OtherRoleBinding(ns string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getClusterUniqueName(),
			Namespace: ns,
			Labels: map[string]string{
				"test": "label",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "not-cryostat",
				Namespace: r.Namespace,
			},
			{
				Kind: rbacv1.UserKind,
				Name: "also-not-cryostat",
			},
		},
		RoleRef: r.NewRoleBinding(ns).RoleRef,
	}
}

func (r *TestResources) OtherRoleRef() rbacv1.RoleRef {
	return rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "not-cryostat",
	}
}

func (r *TestResources) clusterUniqueSuffix(namespace string) string {
	var toEncode string
	if len(namespace) == 0 {
		toEncode = r.Namespace + "/" + r.Name
	} else {
		toEncode = r.Namespace + "/" + r.Name + "/" + namespace
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(toEncode)))
}

func (r *TestResources) clusterUniqueShortSuffix() string {
	toEncode := r.Namespace + "/" + r.Name
	return fmt.Sprintf("%x", sha256.Sum224([]byte(toEncode)))
}

func (r *TestResources) NewClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.getClusterUniqueName(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      r.Name,
				Namespace: r.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cryostat-operator-cryostat",
		},
	}
}

func (r *TestResources) OtherClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.getClusterUniqueName(),
			Labels: map[string]string{
				"test": "label",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "not-cryostat",
				Namespace: r.Namespace,
			},
			{
				Kind: rbacv1.UserKind,
				Name: "also-not-cryostat",
			},
		},
		RoleRef: r.NewClusterRoleBinding().RoleRef,
	}
}

func (r *TestResources) NewTemplateConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "templateCM1",
			Namespace: r.Namespace,
		},
		Data: map[string]string{
			"template.jfc": "XML template data",
		},
	}
}

func (r *TestResources) NewOtherTemplateConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "templateCM2",
			Namespace: r.Namespace,
		},
		Data: map[string]string{
			"other-template.jfc": "more XML template data",
		},
	}
}

func (r *TestResources) NewNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Namespace,
		},
	}
}

func (r *TestResources) NewOtherNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func (r *TestResources) NewNamespaceWithSCCSupGroups() *corev1.Namespace {
	ns := r.NewNamespace()
	ns.Annotations = map[string]string{
		securityv1.SupplementalGroupsAnnotation: "1000130000/10000",
	}
	return ns
}

func (r *TestResources) NewConsoleLink() *consolev1.ConsoleLink {
	return &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.getClusterUniqueName(),
		},
		Spec: consolev1.ConsoleLinkSpec{
			Link: consolev1.Link{
				Text: "Cryostat",
				Href: fmt.Sprintf("https://%s.example.com", r.Name),
			},
			Location: consolev1.NamespaceDashboard,
			NamespaceDashboard: &consolev1.NamespaceDashboardSpec{
				Namespaces: []string{r.Namespace},
			},
		},
	}
}

func (r *TestResources) OtherConsoleLink() *consolev1.ConsoleLink {
	return &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.getClusterUniqueName(),
			Labels: map[string]string{
				"my": "label",
			},
			Annotations: map[string]string{
				"my": "annotation",
			},
		},
		Spec: consolev1.ConsoleLinkSpec{
			Link: consolev1.Link{
				Text: "Not Cryostat",
				Href: "https://not-cryostat.example.com",
			},
			Location: consolev1.HelpMenu,
			NamespaceDashboard: &consolev1.NamespaceDashboardSpec{
				Namespaces: []string{"other"},
			},
		},
	}
}

func (r *TestResources) NewApiServer() *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			AdditionalCORSAllowedOrigins: []string{"https://an-existing-user-specified\\.allowed\\.origin\\.com"},
		},
	}
}

func (r *TestResources) NewApiServerWithApplicationURL() *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			AdditionalCORSAllowedOrigins: []string{
				"https://an-existing-user-specified\\.allowed\\.origin\\.com",
				fmt.Sprintf("https://%s.example.com", r.Name),
			},
		},
	}
}

func (r *TestResources) NewCoreContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("384Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.CoreResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.CoreResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.CoreResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.CoreResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewDatasourceContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("200m"),
			corev1.ResourceMemory: resource.MustParse("200Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.DataSourceResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.DataSourceResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.DataSourceResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.DataSourceResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewGrafanaContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("25m"),
			corev1.ResourceMemory: resource.MustParse("80Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.GrafanaResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.GrafanaResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.GrafanaResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.GrafanaResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewStorageContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.ObjectStorageResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.ObjectStorageResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.ObjectStorageResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.ObjectStorageResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewDatabaseContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("25m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.DatabaseResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.DatabaseResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.DatabaseResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.DatabaseResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewAuthProxyContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("25m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.AuthProxyResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.AuthProxyResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.AuthProxyResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.AuthProxyResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewAgentProxyContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("25m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.AgentProxyResources.Requests != nil {
		resources.Requests = cr.Spec.Resources.AgentProxyResources.Requests
	}

	if cr.Spec.Resources != nil && cr.Spec.Resources.AgentProxyResources.Limits != nil {
		resources.Limits = cr.Spec.Resources.AgentProxyResources.Limits
		checkWithLimit(resources.Requests, resources.Limits)
	}

	return resources
}

func (r *TestResources) NewReportContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	if cr.Spec.ReportOptions != nil {
		reportOptions := cr.Spec.ReportOptions
		if reportOptions.Resources.Requests != nil {
			resources.Requests = reportOptions.Resources.Requests
		}

		if reportOptions.Resources.Limits != nil {
			resources.Limits = reportOptions.Resources.Limits
			checkWithLimit(resources.Requests, resources.Limits)
		}
	}

	return resources
}

func checkWithLimit(requests, limits corev1.ResourceList) {
	if limits != nil {
		if limitCpu, found := limits[corev1.ResourceCPU]; found && limitCpu.Cmp(*requests.Cpu()) < 0 {
			requests[corev1.ResourceCPU] = limitCpu.DeepCopy()
		}
		if limitMemory, found := limits[corev1.ResourceMemory]; found && limitMemory.Cmp(*requests.Memory()) < 0 {
			requests[corev1.ResourceMemory] = limitMemory.DeepCopy()
		}
	}
}

func (r *TestResources) NewLockConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-lock",
			Namespace: r.Namespace,
		},
	}
}

const nginxFormatTLS = `worker_processes auto;
error_log stderr notice;
pid /run/nginx.pid;

# Load dynamic modules. See /usr/share/doc/nginx/README.dynamic.
include /usr/share/nginx/modules/*.conf;

events {
	worker_connections 1024;
}

http {
	log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
	                  '$status $body_bytes_sent "$http_referer" '
	                  '"$http_user_agent" "$http_x_forwarded_for"';

	access_log  /dev/stdout  main;

	sendfile            on;
	tcp_nopush          on;
	keepalive_timeout   65;
	types_hash_max_size 4096;

	include             /etc/nginx/mime.types;
	default_type        application/octet-stream;

	server {
		server_name %s-agent.%s.svc;

		listen 8282 ssl;
		listen [::]:8282 ssl;

		ssl_certificate /var/run/secrets/operator.cryostat.io/%s-agent-tls/tls.crt;
		ssl_certificate_key /var/run/secrets/operator.cryostat.io/%s-agent-tls/tls.key;

		ssl_session_timeout 5m;
		ssl_session_cache shared:SSL:20m;
		ssl_session_tickets off;

		ssl_dhparam /etc/nginx-cryostat/dhparam.pem;

		# intermediate configuration
		ssl_protocols TLSv1.2 TLSv1.3;
		ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384:DHE-RSA-CHACHA20-POLY1305;
		ssl_prefer_server_ciphers off;

		# HSTS (ngx_http_headers_module is required) (63072000 seconds)
		add_header Strict-Transport-Security "max-age=63072000" always;

		# OCSP stapling
		ssl_stapling on;
		ssl_stapling_verify on;

		ssl_trusted_certificate /var/run/secrets/operator.cryostat.io/%s-agent-tls/ca.crt;

		# Client certificate authentication
		ssl_client_certificate /var/run/secrets/operator.cryostat.io/%s-agent-tls/ca.crt;
		ssl_verify_client on;

		location /api/v4/discovery/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /api/v4/discovery {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location /api/v4/credentials/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /api/v4/credentials {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location /api/beta/recordings/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /api/beta/recordings {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location /health/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /health {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location / {
			return 404;
		}
	}

	# Heatlh Check
	server {
		listen 8281;
		listen [::]:8281;

		location = /healthz {
			return 200;
		}

		location / {
			return 404;
		}
	}
}`

const nginxFormatNoTLS = `worker_processes auto;
error_log stderr notice;
pid /run/nginx.pid;

# Load dynamic modules. See /usr/share/doc/nginx/README.dynamic.
include /usr/share/nginx/modules/*.conf;

events {
	worker_connections 1024;
}

http {
	log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
	                  '$status $body_bytes_sent "$http_referer" '
	                  '"$http_user_agent" "$http_x_forwarded_for"';

	access_log  /dev/stdout  main;

	sendfile            on;
	tcp_nopush          on;
	keepalive_timeout   65;
	types_hash_max_size 4096;

	include             /etc/nginx/mime.types;
	default_type        application/octet-stream;

	server {
		server_name %s-agent.%s.svc;

		listen 8282;
		listen [::]:8282;

		location /api/v4/discovery/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /api/v4/discovery {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location /api/v4/credentials/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /api/v4/credentials {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location /api/beta/recordings/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /api/beta/recordings {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location /health/ {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location = /health {
			proxy_pass http://127.0.0.1:8181$request_uri;
		}

		location / {
			return 404;
		}
	}

	# Heatlh Check
	server {
		listen 8281;
		listen [::]:8281;

		location = /healthz {
			return 200;
		}

		location / {
			return 404;
		}
	}
}`

func (r *TestResources) NewAgentProxyConfigMap() *corev1.ConfigMap {
	var data map[string]string
	if r.TLS {
		data = map[string]string{
			"nginx.conf": fmt.Sprintf(nginxFormatTLS, r.Name, r.Namespace, r.Name, r.Name, r.Name, r.Name),
			"dhparam.pem": `-----BEGIN DH PARAMETERS-----
MIIBCAKCAQEA//////////+t+FRYortKmq/cViAnPTzx2LnFg84tNpWp4TZBFGQz
+8yTnc4kmz75fS/jY2MMddj2gbICrsRhetPfHtXV/WVhJDP1H18GbtCFY2VVPe0a
87VXE15/V8k1mE8McODmi3fipona8+/och3xWKE2rec1MKzKT0g6eXq8CrGCsyT7
YdEIqUuyyOP7uWrat2DX9GgdT0Kj3jlN9K5W7edjcrsZCwenyO4KbXCeAvzhzffi
7MA0BM0oNC9hkXL+nOmFg/+OTxIy7vKBg8P+OxtMb61zO7X8vC7CIAXFjvGDfRaD
ssbzSibBsu/6iGtCOGEoXJf//////////wIBAg==
-----END DH PARAMETERS-----`,
		}
	} else {
		data = map[string]string{
			"nginx.conf": fmt.Sprintf(nginxFormatNoTLS, r.Name, r.Namespace),
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-agent-proxy",
			Namespace: r.Namespace,
		},
		Data: data,
	}
}

func (r *TestResources) getClusterUniqueName() string {
	return "cryostat-" + r.clusterUniqueSuffix("")
}

func (r *TestResources) getClusterUniqueNameForCA() string {
	return "cryostat-ca-" + r.clusterUniqueSuffix("")
}

func (r *TestResources) GetClusterUniqueNameForAgent(namespace string) string {
	return r.GetAgentCertPrefix() + r.clusterUniqueSuffix(namespace)
}

func (r *TestResources) GetAgentCertPrefix() string {
	return "cryostat-agent-"
}

func (r *TestResources) GetAgentServiceName() string {
	return "cryo-" + r.clusterUniqueShortSuffix()
}

func (r *TestResources) NewCreateEvent(obj ctrlclient.Object) event.CreateEvent {
	return event.CreateEvent{
		Object: obj,
	}
}

func (r *TestResources) NewUpdateEvent(obj ctrlclient.Object) event.UpdateEvent {
	return event.UpdateEvent{
		ObjectOld: obj,
		ObjectNew: obj,
	}
}

func (r *TestResources) NewDeleteEvent(obj ctrlclient.Object) event.DeleteEvent {
	return event.DeleteEvent{
		Object: obj,
	}
}

func (r *TestResources) NewGenericEvent(obj ctrlclient.Object) event.GenericEvent {
	return event.GenericEvent{
		Object: obj,
	}
}
