// Copyright The Cryostat Authors
//
// The Universal Permissive License (UPL), Version 1.0
//
// Subject to the condition set forth below, permission is hereby granted to any
// person obtaining a copy of this software, associated documentation and/or data
// (collectively the "Software"), free of charge and under any and all copyright
// rights in the Software, and any and all patent rights owned or freely
// licensable by each licensor hereunder covering either (i) the unmodified
// Software as contributed to or provided by such licensor, or (ii) the Larger
// Works (as defined below), to deal in both
//
// (a) the Software, and
// (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
// one is included with the Software (each a "Larger Work" to which the Software
// is contributed by such licensors),
//
// without restriction, including without limitation the rights to copy, create
// derivative works of, display, perform, and distribute the Software and make,
// use, sell, offer for sale, import, export, have made, and have sold the
// Software and the Larger Work(s), and to sublicense the foregoing rights on
// either these or other terms.
//
// This license is subject to the following condition:
// The above copyright notice and either this complete permission notice or at
// a minimum a reference to the UPL must be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package test

import (
	"crypto/sha256"
	"fmt"
	"strings"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	securityv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
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
)

type TestResources struct {
	Name             string
	Namespace        string
	Minimal          bool
	TLS              bool
	ExternalTLS      bool
	OpenShift        bool
	ReportReplicas   int32
	ClusterScoped    bool
	TargetNamespaces []string
}

func NewTestScheme() *runtime.Scheme {
	s := scheme.Scheme

	// Add all APIs used by the operator to the scheme
	sb := runtime.NewSchemeBuilder(
		operatorv1beta1.AddToScheme,
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
	if r.ClusterScoped {
		return r.ConvertClusterToModel(r.newClusterCryostat())
	} else {
		return r.ConvertNamespacedToModel(r.newCryostat())
	}
}

func (r *TestResources) newClusterCryostat() *operatorv1beta1.ClusterCryostat {
	return &operatorv1beta1.ClusterCryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Name,
		},
		Spec: operatorv1beta1.ClusterCryostatSpec{
			InstallNamespace: r.Namespace,
			TargetNamespaces: r.TargetNamespaces,
			CryostatSpec:     r.newCryostatSpec(),
		},
	}
}

func (r *TestResources) newCryostat() *operatorv1beta1.Cryostat {
	return &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
		},
		Spec: r.newCryostatSpec(),
	}
}

func (r *TestResources) newCryostatSpec() operatorv1beta1.CryostatSpec {
	certManager := true
	var reportOptions *operatorv1beta1.ReportConfiguration
	if r.ReportReplicas > 0 {
		reportOptions = &operatorv1beta1.ReportConfiguration{
			Replicas: r.ReportReplicas,
		}
	}
	return operatorv1beta1.CryostatSpec{
		Minimal:           r.Minimal,
		EnableCertManager: &certManager,
		ReportOptions:     reportOptions,
	}
}

func (r *TestResources) ConvertNamespacedToModel(cr *operatorv1beta1.Cryostat) *model.CryostatInstance {
	targetNS := []string{cr.Namespace}
	return &model.CryostatInstance{
		Name:                  cr.Name,
		InstallNamespace:      cr.Namespace,
		TargetNamespaces:      targetNS,
		TargetNamespaceStatus: &targetNS,
		Spec:                  &cr.Spec,
		Status:                &cr.Status,
		Object:                cr,
	}
}

func (r *TestResources) ConvertClusterToModel(cr *operatorv1beta1.ClusterCryostat) *model.CryostatInstance {
	return &model.CryostatInstance{
		Name:                  cr.Name,
		InstallNamespace:      cr.Spec.InstallNamespace,
		TargetNamespaces:      cr.Spec.TargetNamespaces,
		TargetNamespaceStatus: &cr.Status.TargetNamespaces,
		Spec:                  &cr.Spec.CryostatSpec,
		Status:                &cr.Status.CryostatStatus,
		Object:                cr,
	}
}

func (r *TestResources) NewCryostatWithSecrets() *model.CryostatInstance {
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithTemplates() *model.CryostatInstance {
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithIngress() *model.CryostatInstance {
	cr := r.NewCryostat()
	networkConfig := r.newNetworkConfigurationList()
	cr.Spec.NetworkOptions = &networkConfig
	return cr
}

func (r *TestResources) NewCryostatWithPVCSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithPVCSpecSomeDefault() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Spec: newPVCSpec("", "1Gi"),
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithPVCLabelsOnly() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Labels: map[string]string{
				"my": "label",
			},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithDefaultEmptyDir() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		EmptyDir: &operatorv1beta1.EmptyDirConfig{
			Enabled: true,
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithEmptyDirSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		EmptyDir: &operatorv1beta1.EmptyDirConfig{
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
	jmxPort := int32(9095)
	cr := r.NewCryostat()
	cr.Spec.ServiceOptions = &operatorv1beta1.ServiceConfigList{
		CoreConfig: &operatorv1beta1.CoreServiceConfig{
			HTTPPort: &httpPort,
			JMXPort:  &jmxPort,
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

func (r *TestResources) NewCryostatWithGrafanaSvc() *model.CryostatInstance {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithReportsSvc() *model.CryostatInstance {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(13161)
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithCoreNetworkOptions() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		CoreConfig: &operatorv1beta1.NetworkConfiguration{
			Annotations: map[string]string{"custom": "annotation"},
			Labels:      map[string]string{"custom": "label"},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithGrafanaNetworkOptions() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		GrafanaConfig: &operatorv1beta1.NetworkConfiguration{
			Annotations: map[string]string{"grafana": "annotation"},
			Labels:      map[string]string{"grafana": "label"},
		},
	}
	return cr
}

func (r *TestResources) NewCryostatWithReportsResources() *model.CryostatInstance {
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithReportLowResourceLimit() *model.CryostatInstance {
	cr := r.NewCryostat()
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

func populateCryostatWithScheduling() *operatorv1beta1.SchedulingConfiguration {
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

func (r *TestResources) NewCryostatWithScheduling() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.SchedulingOptions = populateCryostatWithScheduling()
	return cr
}

func (r *TestResources) NewCryostatWithReportsScheduling() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
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

func (r *TestResources) NewCryostatWithLowResourceLimit() *model.CryostatInstance {
	cr := r.NewCryostat()
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

func (r *TestResources) NewCryostatWithAuthProperties() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.AuthProperties = &operatorv1beta1.AuthorizationProperties{
		ConfigMapName:   "authConfigMapName",
		Filename:        "auth.properties",
		ClusterRoleName: "custom-auth-cluster-role",
	}
	return cr
}

func (r *TestResources) NewCryostatWithBuiltInDiscoveryDisabled() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.TargetDiscoveryOptions = &operatorv1beta1.TargetDiscoveryOptions{
		BuiltInDiscoveryDisabled: true,
	}
	return cr
}

func newPVCSpec(storageClass string, storageRequest string,
	accessModes ...corev1.PersistentVolumeAccessMode) *corev1.PersistentVolumeClaimSpec {
	return &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
		AccessModes:      accessModes,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(storageRequest),
			},
		},
	}
}

func (r *TestResources) NewCryostatWithJmxCacheOptionsSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.JmxCacheOptions = &operatorv1beta1.JmxCacheOptions{
		TargetCacheSize: 10,
		TargetCacheTTL:  20,
	}
	return cr
}

func (r *TestResources) NewCryostatWithWsConnectionsSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.MaxWsConnections = 10
	return cr
}

func (r *TestResources) NewCryostatWithReportSubprocessHeapSpec() *model.CryostatInstance {
	cr := r.NewCryostat()
	if cr.Spec.ReportOptions == nil {
		cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{}
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

func (r *TestResources) NewCryostatWithReportSecurityOptions() *model.CryostatInstance {
	cr := r.NewCryostat()
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

var providedDatabaseSecretName string = "credentials-database-secret"

func (r *TestResources) NewCryostatWithDatabaseSecretProvided() *model.CryostatInstance {
	cr := r.NewCryostat()
	cr.Spec.JmxCredentialsDatabaseOptions = &operatorv1beta1.JmxCredentialsDatabaseOptions{
		DatabaseSecretName: &providedDatabaseSecretName,
	}
	return cr
}

func (r *TestResources) NewCryostatService() *corev1.Service {
	c := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: operatorv1beta1.GroupVersion.String(),
					Kind:       "Cryostat",
					Name:       r.Name,
					UID:        "",
					Controller: &c,
				},
			},
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
					Port:       8181,
					TargetPort: intstr.FromInt(8181),
				},
				{
					Name:       "jfr-jmx",
					Port:       9091,
					TargetPort: intstr.FromInt(9091),
				},
			},
		},
	}
}

func (r *TestResources) NewGrafanaService() *corev1.Service {
	c := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-grafana",
			Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: operatorv1beta1.GroupVersion.String(),
					Kind:       "Cryostat",
					Name:       r.Name,
					UID:        "",
					Controller: &c,
				},
			},
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

func (r *TestResources) NewReportsService() *corev1.Service {
	c := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-reports",
			Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: operatorv1beta1.GroupVersion.String(),
					Kind:       "Cryostat",
					Name:       r.Name + "-reports",
					UID:        "",
					Controller: &c,
				},
			},
			Labels: map[string]string{
				"app":       r.Name,
				"component": "reports",
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

func (r *TestResources) NewCustomizedCoreService() *corev1.Service {
	svc := r.NewCryostatService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 8080
	svc.Spec.Ports[1].Port = 9095
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":       r.Name,
		"component": "cryostat",
		"my":        "label",
	}
	return svc
}

func (r *TestResources) NewCustomizedGrafanaService() *corev1.Service {
	svc := r.NewGrafanaService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 8080
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":       r.Name,
		"component": "cryostat",
		"my":        "label",
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
		"app":       r.Name,
		"component": "reports",
		"my":        "label",
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
					Port: 8181,
				},
			},
		},
	}
}

func (r *TestResources) NewGrafanaSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-grafana-basic",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"GF_SECURITY_ADMIN_USER":     "admin",
			"GF_SECURITY_ADMIN_PASSWORD": "grafana",
		},
	}
}

func (r *TestResources) OtherGrafanaSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-grafana-basic",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"GF_SECURITY_ADMIN_USER":     "user",
			"GF_SECURITY_ADMIN_PASSWORD": "goodpassword",
		},
	}
}

func (r *TestResources) NewCredentialsDatabaseSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-jmx-credentials-db",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD": "credentials_database",
		},
	}
}

func (r *TestResources) OtherCredentialsDatabaseSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-jmx-credentials-db",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD": "other-pass",
		},
	}
}

func (r *TestResources) NewJMXSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-jmx-auth",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CRYOSTAT_RJMX_USER": "cryostat",
			"CRYOSTAT_RJMX_PASS": "jmx",
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

func (r *TestResources) OtherJMXSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-jmx-auth",
			Namespace: r.Namespace,
		},
		StringData: map[string]string{
			"CRYOSTAT_RJMX_USER": "not-cryostat",
			"CRYOSTAT_RJMX_PASS": "other-pass",
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
			CommonName: fmt.Sprintf(r.Name+".%s.svc", r.Namespace),
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

func (r *TestResources) NewGrafanaCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-grafana",
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf(r.Name+"-grafana.%s.svc", r.Namespace),
			DNSNames: []string{
				r.Name + "-grafana",
				fmt.Sprintf(r.Name+"-grafana.%s.svc", r.Namespace),
				fmt.Sprintf(r.Name+"-grafana.%s.svc.cluster.local", r.Namespace),
				"cryostat-health.local",
			},
			SecretName: r.Name + "-grafana-tls",
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

func (r *TestResources) NewReportsCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-reports",
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf(r.Name+"-reports.%s.svc", r.Namespace),
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

func (r *TestResources) NewCACert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name + "-ca",
			Namespace: r.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("ca.%s.cert-manager", r.Name),
			SecretName: r.Name + "-ca",
			IssuerRef: certMeta.ObjectReference{
				Name: r.Name + "-self-signed",
			},
			IsCA: true,
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
		Resources: corev1.ResourceRequirements{
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
		Resources: corev1.ResourceRequirements{
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
		Resources: corev1.ResourceRequirements{
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
		Resources: corev1.ResourceRequirements{
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
		{
			ContainerPort: 9091,
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
			ContainerPort: 8080,
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

func (r *TestResources) NewCoreEnvironmentVariables(reportsUrl string, authProps bool, ingress bool,
	emptyDir bool, builtInDiscoveryDisabled bool, dbSecretProvided bool) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_WEB_PORT",
			Value: "8181",
		},
		{
			Name:  "CRYOSTAT_CONFIG_PATH",
			Value: "/opt/cryostat.d/conf.d",
		},
		{
			Name:  "CRYOSTAT_ARCHIVE_PATH",
			Value: "/opt/cryostat.d/recordings.d",
		},
		{
			Name:  "CRYOSTAT_TEMPLATE_PATH",
			Value: "/opt/cryostat.d/templates.d",
		},
		{
			Name:  "CRYOSTAT_CLIENTLIB_PATH",
			Value: "/opt/cryostat.d/clientlib.d",
		},
		{
			Name:  "CRYOSTAT_PROBE_TEMPLATE_PATH",
			Value: "/opt/cryostat.d/probes.d",
		},
		{
			Name:  "CRYOSTAT_ENABLE_JDP_BROADCAST",
			Value: "false",
		},
		{
			Name:  "CRYOSTAT_TARGET_CACHE_SIZE",
			Value: "-1",
		},
		{
			Name:  "CRYOSTAT_TARGET_CACHE_TTL",
			Value: "10",
		},
		{
			Name:  "CRYOSTAT_K8S_NAMESPACES",
			Value: strings.Join(r.TargetNamespaces, ","),
		},
	}

	if builtInDiscoveryDisabled {
		envs = append(envs, corev1.EnvVar{
			Name:  "CRYOSTAT_DISABLE_BUILTIN_DISCOVERY",
			Value: "true",
		})
	}

	if !emptyDir {
		envs = append(envs, r.DatabaseConfigEnvironmentVariables()...)
	}

	optional := false
	secretName := r.NewCredentialsDatabaseSecret().Name
	if dbSecretProvided {
		secretName = providedDatabaseSecretName
	}
	envs = append(envs, corev1.EnvVar{
		Name: "CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key:      "CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD",
				Optional: &optional,
			},
		},
	})

	if !r.Minimal {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "GRAFANA_DATASOURCE_URL",
				Value: "http://127.0.0.1:8080",
			})
	}
	if !r.TLS {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISABLE_SSL",
				Value: "true",
			})
		if r.ExternalTLS {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "CRYOSTAT_SSL_PROXIED",
					Value: "true",
				})
		}
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "KEYSTORE_PATH",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-tls/keystore.p12", r.Name),
		})
	}

	if r.OpenShift {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_PLATFORM",
				Value: "io.cryostat.platform.internal.OpenShiftPlatformStrategy",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AUTH_MANAGER",
				Value: "io.cryostat.net.openshift.OpenShiftAuthManager",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_OAUTH_CLIENT_ID",
				Value: r.Name,
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_BASE_OAUTH_ROLE",
				Value: "cryostat-operator-oauth-client",
			})

		if authProps {
			envs = append(envs, corev1.EnvVar{
				Name:  "CRYOSTAT_CUSTOM_OAUTH_ROLE",
				Value: "custom-auth-cluster-role",
			})
		}
		envs = append(envs, r.newNetworkEnvironmentVariables()...)
	} else if ingress { // On Kubernetes
		envs = append(envs, r.newNetworkEnvironmentVariables()...)
	}

	if reportsUrl != "" {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_REPORT_GENERATOR",
				Value: reportsUrl,
			})
	} else {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_REPORT_GENERATION_MAX_HEAP",
				Value: "200",
			})
	}
	return envs
}

func (r *TestResources) DatabaseConfigEnvironmentVariables() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_JDBC_URL",
			Value: "jdbc:h2:file:/opt/cryostat.d/conf.d/h2;INIT=create domain if not exists jsonb as varchar",
		},
		{
			Name:  "CRYOSTAT_HBM2DDL",
			Value: "update",
		},
		{
			Name:  "CRYOSTAT_JDBC_DRIVER",
			Value: "org.h2.Driver",
		},
		{
			Name:  "CRYOSTAT_HIBERNATE_DIALECT",
			Value: "org.hibernate.dialect.H2Dialect",
		},
		{
			Name:  "CRYOSTAT_JDBC_USERNAME",
			Value: r.Name,
		},
		{
			Name:  "CRYOSTAT_JDBC_PASSWORD",
			Value: r.Name,
		},
	}
}

func (r *TestResources) newNetworkEnvironmentVariables() []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_WEB_HOST",
			Value: r.Name + ".example.com",
		},
	}
	if r.ExternalTLS {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_EXT_WEB_PORT",
				Value: "443",
			})
	} else {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_EXT_WEB_PORT",
				Value: "80",
			})
	}
	if !r.Minimal {
		if r.ExternalTLS {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_EXT_URL",
					Value: fmt.Sprintf("https://%s-grafana.example.com", r.Name),
				})
		} else {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_EXT_URL",
					Value: fmt.Sprintf("http://%s-grafana.example.com", r.Name),
				})
		}
		if r.TLS {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_URL",
					Value: "https://cryostat-health.local:3000",
				})
		} else {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_URL",
					Value: "http://cryostat-health.local:3000",
				})
		}
	}
	return envs
}

func (r *TestResources) NewGrafanaEnvironmentVariables() []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "JFR_DATASOURCE_URL",
			Value: "http://127.0.0.1:8080",
		},
	}
	if r.TLS {
		envs = append(envs, corev1.EnvVar{
			Name:  "GF_SERVER_PROTOCOL",
			Value: "https",
		}, corev1.EnvVar{
			Name:  "GF_SERVER_CERT_KEY",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-grafana-tls/tls.key", r.Name),
		}, corev1.EnvVar{
			Name:  "GF_SERVER_CERT_FILE",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-grafana-tls/tls.crt", r.Name),
		})
	}
	return envs
}

func (r *TestResources) NewDatasourceEnvironmentVariables() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "LISTEN_HOST",
			Value: "127.0.0.1",
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
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_KEY_FILE",
			Value: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-reports-tls/tls.key", r.Name),
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_FILE",
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

func (r *TestResources) NewCoreEnvFromSource() []corev1.EnvFromSource {
	envsFrom := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.Name + "-jmx-auth",
				},
			},
		},
	}
	if r.TLS {
		envsFrom = append(envsFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.Name + "-keystore",
				},
			},
		})
	}
	return envsFrom
}

func (r *TestResources) NewGrafanaEnvFromSource() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: r.Name + "-grafana-basic",
				},
			},
		},
	}
}

func (r *TestResources) NewWsConnectionsEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_MAX_WS_CONNECTIONS",
			Value: "10",
		},
	}
}

func (r *TestResources) NewReportSubprocessHeapEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_REPORT_GENERATION_MAX_HEAP",
			Value: "500",
		},
	}
}

func (r *TestResources) NewJmxCacheOptionsEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_TARGET_CACHE_SIZE",
			Value: "10",
		},
		{
			Name:  "CRYOSTAT_TARGET_CACHE_TTL",
			Value: "20",
		},
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
			MountPath: "/opt/cryostat.d/recordings.d",
			SubPath:   "flightrecordings",
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
			MountPath: "/opt/cryostat.d/clientlib.d",
			SubPath:   "clientlib",
		},
		{
			Name:      r.Name,
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/probes.d",
			SubPath:   "probes",
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
	if r.TLS {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "keystore",
				ReadOnly:  true,
				MountPath: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-tls", r.Name),
			})
	}
	return mounts
}

func (r *TestResources) NewGrafanaVolumeMounts() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	if r.TLS {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "grafana-tls-secret",
				MountPath: fmt.Sprintf("/var/run/secrets/operator.cryostat.io/%s-grafana-tls", r.Name),
				ReadOnly:  true,
			})
	}
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
	protocol := corev1.URISchemeHTTPS
	if !r.TLS {
		protocol = corev1.URISchemeHTTP
	}
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Port:   intstr.IntOrString{IntVal: 8181},
			Path:   "/health/liveness",
			Scheme: protocol,
		},
	}
}

func (r *TestResources) NewGrafanaLivenessProbe() *corev1.Probe {
	protocol := corev1.URISchemeHTTPS
	if !r.TLS {
		protocol = corev1.URISchemeHTTP
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port:   intstr.IntOrString{IntVal: 3000},
				Path:   "/api/health",
				Scheme: protocol,
			},
		},
	}
}

func (r *TestResources) NewDatasourceLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"curl", "--fail", "http://127.0.0.1:8080"},
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
			})
		if !r.Minimal {
			volumes = append(volumes,
				corev1.Volume{
					Name: "grafana-tls-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: r.Name + "-grafana-tls",
						},
					},
				})
		}
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

func (r *TestResources) NewReportSecurityContext(cr *model.CryostatInstance) *corev1.SecurityContext {
	if cr.Spec.ReportOptions != nil && cr.Spec.ReportOptions.SecurityOptions != nil && cr.Spec.ReportOptions.SecurityOptions.ReportsSecurityContext != nil {
		return cr.Spec.ReportOptions.SecurityOptions.ReportsSecurityContext
	}
	return r.commonDefaultSecurityContext()
}

func (r *TestResources) NewCoreRoute() *routev1.Route {
	return r.newRoute(r.Name, 8181)
}

func (r *TestResources) NewCustomCoreRoute() *routev1.Route {
	route := r.NewCoreRoute()
	route.Annotations = map[string]string{"custom": "annotation"}
	route.Labels = map[string]string{"custom": "label"}
	return route
}

func (r *TestResources) NewGrafanaRoute() *routev1.Route {
	return r.newRoute(r.Name+"-grafana", 3000)
}

func (r *TestResources) NewCustomGrafanaRoute() *routev1.Route {
	route := r.NewGrafanaRoute()
	route.Annotations = map[string]string{"grafana": "annotation"}
	route.Labels = map[string]string{"grafana": "label"}
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
			Termination:              routev1.TLSTerminationReencrypt,
			DestinationCACertificate: r.Name + "-ca-bytes",
		}
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.Namespace,
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

func (r *TestResources) OtherGrafanaRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.Name + "-grafana",
			Namespace:   r.Namespace,
			Annotations: map[string]string{"grafana": "annotation"},
			Labels:      map[string]string{"grafana": "label"},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: "not-grafana",
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromInt(5678),
			},
		},
	}
}

func (r *TestResources) OtherCoreIngress() *netv1.Ingress {
	pathtype := netv1.PathTypePrefix
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.Name,
			Namespace:   r.Namespace,
			Annotations: map[string]string{"custom": "annotation"},
			Labels:      map[string]string{"custom": "label"},
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

func (r *TestResources) OtherGrafanaIngress() *netv1.Ingress {
	pathtype := netv1.PathTypePrefix
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.Name + "-grafana",
			Namespace:   r.Namespace,
			Annotations: map[string]string{"grafana": "annotation"},
			Labels:      map[string]string{"grafana": "label"},
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "some-other-grafana.example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathtype,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "some-other-grafana",
											Port: netv1.ServiceBackendPort{
												Number: 5000,
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

func (r *TestResources) newNetworkConfigurationList() operatorv1beta1.NetworkConfigurationList {
	coreSVC := r.NewCryostatService()
	coreIng := r.newNetworkConfiguration(coreSVC.Name, coreSVC.Spec.Ports[0].Port)

	grafanaSVC := r.NewGrafanaService()
	grafanaIng := r.newNetworkConfiguration(grafanaSVC.Name, grafanaSVC.Spec.Ports[0].Port)

	return operatorv1beta1.NetworkConfigurationList{
		CoreConfig:    &coreIng,
		GrafanaConfig: &grafanaIng,
	}
}

func (r *TestResources) newNetworkConfiguration(svcName string, svcPort int32) operatorv1beta1.NetworkConfiguration {
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

func (r *TestResources) NewAuthClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-auth-cluster-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "update", "patch", "delete"},
				APIGroups: []string{"group"},
				Resources: []string{"resources"},
			},
			{
				Verbs:     []string{"get", "update", "patch", "delete"},
				APIGroups: []string{"another_group"},
				Resources: []string{"another_resources"},
			},
		},
	}
}

func (r *TestResources) NewRoleBinding(ns string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Name,
			Namespace: ns,
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
			Name:      r.Name,
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

func (r *TestResources) clusterUniqueSuffix() string {
	toEncode := r.Namespace + "/" + r.Name
	return fmt.Sprintf("%x", sha256.Sum256([]byte(toEncode)))
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

func (r *TestResources) NewAuthPropertiesConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "authConfigMapName",
			Namespace: r.Namespace,
		},
		Data: map[string]string{
			"auth.properties": "CRYOSTAT_RESOURCE=resources.group\nANOTHER_CRYOSTAT_RESOURCE=another_resources.another_group",
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

func newCoreContainerDefaultResource() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("384Mi"),
		},
	}
}

func (r *TestResources) NewCoreContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	requests := newCoreContainerDefaultResource().Requests
	var limits corev1.ResourceList
	if cr.Spec.Resources != nil && cr.Spec.Resources.CoreResources.Requests != nil {
		requests = cr.Spec.Resources.CoreResources.Requests
	} else if cr.Spec.Resources != nil && cr.Spec.Resources.CoreResources.Limits != nil {
		checkWithLimit(requests, cr.Spec.Resources.CoreResources.Limits)
	}

	if cr.Spec.Resources != nil {
		limits = cr.Spec.Resources.CoreResources.Limits
	}

	return &corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

func newDatasourceContainerDefaultResource() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

func (r *TestResources) NewDatasourceContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	requests := newDatasourceContainerDefaultResource().Requests
	var limits corev1.ResourceList
	if cr.Spec.Resources != nil && cr.Spec.Resources.DataSourceResources.Requests != nil {
		requests = cr.Spec.Resources.DataSourceResources.Requests
	} else if cr.Spec.Resources != nil && cr.Spec.Resources.DataSourceResources.Limits != nil {
		checkWithLimit(requests, cr.Spec.Resources.DataSourceResources.Limits)
	}

	if cr.Spec.Resources != nil {
		limits = cr.Spec.Resources.DataSourceResources.Limits
	}

	return &corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

func newGrafanaContainerDefaultResource() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
}

func (r *TestResources) NewGrafanaContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	requests := newGrafanaContainerDefaultResource().Requests
	var limits corev1.ResourceList
	if cr.Spec.Resources != nil && cr.Spec.Resources.GrafanaResources.Requests != nil {
		requests = cr.Spec.Resources.GrafanaResources.Requests
	} else if cr.Spec.Resources != nil && cr.Spec.Resources.GrafanaResources.Limits != nil {
		checkWithLimit(requests, cr.Spec.Resources.GrafanaResources.Limits)
	}

	if cr.Spec.Resources != nil {
		limits = cr.Spec.Resources.GrafanaResources.Limits
	}

	return &corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

func newReportContainerDefaultResource() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("128m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
}

func (r *TestResources) NewReportContainerResource(cr *model.CryostatInstance) *corev1.ResourceRequirements {
	requests := newReportContainerDefaultResource().Requests
	var limits corev1.ResourceList

	if cr.Spec.ReportOptions != nil {
		reportOptions := cr.Spec.ReportOptions
		if reportOptions.Resources.Requests != nil {
			requests = reportOptions.Resources.Requests
		} else if reportOptions.Resources.Limits != nil {
			checkWithLimit(requests, reportOptions.Resources.Limits)
		}

		limits = reportOptions.Resources.Limits
	}
	return &corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
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

func (r *TestResources) getClusterUniqueName() string {
	prefix := "cryostat-"
	if r.ClusterScoped {
		prefix = "clustercryostat-"
	}
	return prefix + r.clusterUniqueSuffix()
}
