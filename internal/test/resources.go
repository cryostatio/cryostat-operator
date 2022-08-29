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
	"time"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
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

func NewCryostat() *operatorv1beta1.Cryostat {
	certManager := true
	return &operatorv1beta1.Cryostat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
		},
		Spec: operatorv1beta1.CryostatSpec{
			Minimal:            false,
			EnableCertManager:  &certManager,
			TrustedCertSecrets: []operatorv1beta1.CertificateSecret{},
		},
	}
}

func NewCryostatWithSecrets() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
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

func NewCryostatWithTemplates() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
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

func NewCryostatWithIngress() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	networkConfig := NewNetworkConfigurationList(true)
	cr.Spec.NetworkOptions = &networkConfig
	return cr
}

func NewCryostatWithIngressNoTLS() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	networkConfig := NewNetworkConfigurationList(false)
	cr.Spec.NetworkOptions = &networkConfig
	return cr
}

func NewCryostatWithPVCSpec() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
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

func NewCryostatWithPVCSpecSomeDefault() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Spec: newPVCSpec("", "1Gi"),
		},
	}
	return cr
}

func NewCryostatWithPVCLabelsOnly() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		PVC: &operatorv1beta1.PersistentVolumeClaimConfig{
			Labels: map[string]string{
				"my": "label",
			},
		},
	}
	return cr
}

func NewCryostatWithDefaultEmptyDir() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		EmptyDir: &operatorv1beta1.EmptyDirConfig{
			Enabled: true,
		},
	}
	return cr
}

func NewCryostatWithEmptyDirSpec() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.StorageOptions = &operatorv1beta1.StorageConfiguration{
		EmptyDir: &operatorv1beta1.EmptyDirConfig{
			Enabled:   true,
			Medium:    "Memory",
			SizeLimit: "200Mi",
		},
	}
	return cr
}

func NewCryostatWithCoreSvc() *operatorv1beta1.Cryostat {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	jmxPort := int32(9095)
	cr := NewCryostat()
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

func NewCryostatWithGrafanaSvc() *operatorv1beta1.Cryostat {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(8080)
	cr := NewCryostat()
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

func NewCryostatWithReportsSvc() *operatorv1beta1.Cryostat {
	svcType := corev1.ServiceTypeNodePort
	httpPort := int32(13161)
	cr := NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
		Replicas: 1,
	}
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

func NewCryostatWithCoreNetworkOptions() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		CoreConfig: &operatorv1beta1.NetworkConfiguration{
			Annotations: map[string]string{"custom": "annotation"},
			Labels:      map[string]string{"custom": "label"},
		},
	}
	return cr
}

func NewCryostatWithGrafanaNetworkOptions() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.NetworkOptions = &operatorv1beta1.NetworkConfigurationList{
		GrafanaConfig: &operatorv1beta1.NetworkConfiguration{
			Annotations: map[string]string{"grafana": "annotation"},
			Labels:      map[string]string{"grafana": "label"},
		},
	}
	return cr
}

func NewCryostatWithReportsResources() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
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

func NewCryostatCertManagerDisabled() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	certManager := false
	cr.Spec.EnableCertManager = &certManager
	return cr
}

func NewCryostatCertManagerUndefined() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.EnableCertManager = nil
	return cr
}

func NewCryostatWithResources() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.Resources = operatorv1beta1.ResourceConfigList{
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

func NewCryostatWithAuthProperties() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.AuthProperties = &operatorv1beta1.AuthorizationProperties{
		ConfigMapName:   "authConfigMapName",
		Filename:        "auth.properties",
		ClusterRoleName: "custom-auth-cluster-role",
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

func NewMinimalCryostat() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.Minimal = true
	return cr
}

func NewCryostatWithJmxCacheOptionsSpec() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.JmxCacheOptions = &operatorv1beta1.JmxCacheOptions{
		TargetCacheSize: 10,
		TargetCacheTTL:  20,
	}
	return cr
}

func NewCryostatWithWsConnectionsSpec() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.MaxWsConnections = 10
	return cr
}

func NewCryostatWithReportSubprocessHeapSpec() *operatorv1beta1.Cryostat {
	cr := NewCryostat()
	cr.Spec.ReportOptions = &operatorv1beta1.ReportConfiguration{
		SubProcessMaxHeapSize: 500,
	}
	return cr
}

func NewFlightRecorder() *operatorv1beta1.FlightRecorder {
	return newFlightRecorder(&operatorv1beta1.JMXAuthSecret{
		SecretName: "test-jmx-auth",
	})
}

func NewFlightRecorderNoJMXAuth() *operatorv1beta1.FlightRecorder {
	return newFlightRecorder(nil)
}

func NewFlightRecorderBadJMXUserKey() *operatorv1beta1.FlightRecorder {
	key := "not-username"
	return newFlightRecorder(&operatorv1beta1.JMXAuthSecret{
		SecretName:  "test-jmx-auth",
		UsernameKey: &key,
	})
}

func NewFlightRecorderBadJMXPassKey() *operatorv1beta1.FlightRecorder {
	key := "not-password"
	return newFlightRecorder(&operatorv1beta1.JMXAuthSecret{
		SecretName:  "test-jmx-auth",
		PasswordKey: &key,
	})
}

func NewFlightRecorderForCryostat() *operatorv1beta1.FlightRecorder {
	userKey := "CRYOSTAT_RJMX_USER"
	passKey := "CRYOSTAT_RJMX_PASS"
	recorder := newFlightRecorder(&operatorv1beta1.JMXAuthSecret{
		SecretName:  "cryostat-jmx-auth",
		UsernameKey: &userKey,
		PasswordKey: &passKey,
	})
	recorder.Name = "cryostat-pod"
	recorder.Labels = map[string]string{"app": "cryostat-pod"}
	recorder.OwnerReferences[0].Name = "cryostat-pod"
	recorder.Spec.RecordingSelector.MatchLabels = map[string]string{"operator.cryostat.io/flightrecorder": "cryostat-pod"}
	return recorder
}

func newFlightRecorder(jmxAuth *operatorv1beta1.JMXAuthSecret) *operatorv1beta1.FlightRecorder {
	return &operatorv1beta1.FlightRecorder{
		TypeMeta: metav1.TypeMeta{
			Kind:       "FlightRecorder",
			APIVersion: "operator.cryostat.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test-pod",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       "test-pod",
					UID:        "",
				},
			},
		},
		Spec: operatorv1beta1.FlightRecorderSpec{
			JMXCredentials:    jmxAuth,
			RecordingSelector: metav1.AddLabelToSelector(&metav1.LabelSelector{}, operatorv1beta1.RecordingLabel, "test-pod"),
		},
		Status: operatorv1beta1.FlightRecorderStatus{
			Target: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       "test-pod",
				Namespace:  "default",
			},
			Port: 8001,
		},
	}
}

func NewRecording() *operatorv1beta1.Recording {
	return newRecording(getDuration(false), nil, nil, false)
}

func NewContinuousRecording() *operatorv1beta1.Recording {
	return newRecording(getDuration(true), nil, nil, false)
}

func NewRunningRecording() *operatorv1beta1.Recording {
	running := operatorv1beta1.RecordingStateRunning
	return newRecording(getDuration(false), &running, nil, false)
}

func NewRunningContinuousRecording() *operatorv1beta1.Recording {
	running := operatorv1beta1.RecordingStateRunning
	return newRecording(getDuration(true), &running, nil, false)
}

func NewRecordingToStop() *operatorv1beta1.Recording {
	running := operatorv1beta1.RecordingStateRunning
	stopped := operatorv1beta1.RecordingStateStopped
	return newRecording(getDuration(true), &running, &stopped, false)
}

func NewStoppedRecordingToArchive() *operatorv1beta1.Recording {
	stopped := operatorv1beta1.RecordingStateStopped
	return newRecording(getDuration(false), &stopped, nil, true)
}

func NewRecordingToStopAndArchive() *operatorv1beta1.Recording {
	running := operatorv1beta1.RecordingStateRunning
	stopped := operatorv1beta1.RecordingStateStopped
	return newRecording(getDuration(true), &running, &stopped, true)
}

func NewArchivedRecording() *operatorv1beta1.Recording {
	stopped := operatorv1beta1.RecordingStateStopped
	rec := newRecording(getDuration(false), &stopped, nil, true)
	savedDownloadURL := "http://path/to/saved-test-recording.jfr"
	savedReportURL := "http://path/to/saved-test-recording.html"
	rec.Status.DownloadURL = &savedDownloadURL
	rec.Status.ReportURL = &savedReportURL
	return rec
}

func NewDeletedArchivedRecording() *operatorv1beta1.Recording {
	rec := NewArchivedRecording()
	delTime := metav1.Unix(0, 1598045501618*int64(time.Millisecond))
	rec.DeletionTimestamp = &delTime
	return rec
}

func newRecording(duration time.Duration, currentState *operatorv1beta1.RecordingState,
	requestedState *operatorv1beta1.RecordingState, archive bool) *operatorv1beta1.Recording {
	finalizers := []string{}
	status := operatorv1beta1.RecordingStatus{}
	if currentState != nil {
		downloadUrl := "http://path/to/test-recording.jfr"
		reportUrl := "http://path/to/test-recording.html"
		finalizers = append(finalizers, "operator.cryostat.io/recording.finalizer")
		status = operatorv1beta1.RecordingStatus{
			State:       currentState,
			StartTime:   metav1.Unix(0, 1597090030341*int64(time.Millisecond)),
			Duration:    metav1.Duration{Duration: duration},
			DownloadURL: &downloadUrl,
			ReportURL:   &reportUrl,
		}
	}
	return &operatorv1beta1.Recording{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-recording",
			Namespace:  "default",
			Finalizers: finalizers,
		},
		Spec: operatorv1beta1.RecordingSpec{
			Name: "test-recording",
			EventOptions: []string{
				"jdk.socketRead:enabled=true",
				"jdk.socketWrite:enabled=true",
			},
			Duration: metav1.Duration{Duration: duration},
			State:    requestedState,
			Archive:  archive,
			FlightRecorder: &corev1.LocalObjectReference{
				Name: "test-pod",
			},
		},
		Status: status,
	}
}

func getDuration(continuous bool) time.Duration {
	seconds := 0
	if !continuous {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func NewTargetPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			PodIP: "1.2.3.4",
		},
	}
}

func NewCryostatPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			PodIP: "1.2.3.4",
		},
	}
}

func NewTestEndpoints() *corev1.Endpoints {
	target := &corev1.ObjectReference{
		Kind:      "Pod",
		Name:      "test-pod",
		Namespace: "default",
	}
	ports := []corev1.EndpointPort{
		{
			Name: "jfr-jmx",
			Port: 1234,
		},
		{
			Name: "other-port",
			Port: 9091,
		},
	}
	return newTestEndpoints(target, ports)
}

func NewTestEndpointsNoTargetRef() *corev1.Endpoints {
	ports := []corev1.EndpointPort{
		{
			Name: "jfr-jmx",
			Port: 1234,
		},
		{
			Name: "other-port",
			Port: 9091,
		},
	}
	return newTestEndpoints(nil, ports)
}

func NewTestEndpointsNoPorts() *corev1.Endpoints {
	target := &corev1.ObjectReference{
		Kind:      "Pod",
		Name:      "test-pod",
		Namespace: "default",
	}
	return newTestEndpoints(target, nil)
}

func NewTestEndpointsNoJMXPort() *corev1.Endpoints {
	target := &corev1.ObjectReference{
		Kind:      "Pod",
		Name:      "test-pod",
		Namespace: "default",
	}
	ports := []corev1.EndpointPort{
		{
			Name: "other-port",
			Port: 9091,
		},
	}
	return newTestEndpoints(target, ports)
}

func newTestEndpoints(targetRef *corev1.ObjectReference, ports []corev1.EndpointPort) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP:        "1.2.3.4",
						Hostname:  "test-pod",
						TargetRef: targetRef,
					},
				},
				Ports: ports,
			},
		},
	}
}

func NewCryostatEndpoints() *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP:       "1.2.3.4",
						Hostname: "cryostat-pod",
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Name:      "cryostat-pod",
							Namespace: "default",
						},
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Name: "jfr-jmx",
						Port: 1234,
					},
				},
			},
		},
	}
}

func NewCryostatService() *corev1.Service {
	c := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: operatorv1beta1.GroupVersion.String(),
					Kind:       "Cryostat",
					Name:       "cryostat",
					UID:        "",
					Controller: &c,
				},
			},
			Labels: map[string]string{
				"app":       "cryostat",
				"component": "cryostat",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":       "cryostat",
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

func NewGrafanaService() *corev1.Service {
	c := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-grafana",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: operatorv1beta1.GroupVersion.String(),
					Kind:       "Cryostat",
					Name:       "cryostat",
					UID:        "",
					Controller: &c,
				},
			},
			Labels: map[string]string{
				"app":       "cryostat",
				"component": "cryostat",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":       "cryostat",
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

func NewReportsService() *corev1.Service {
	c := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-reports",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: operatorv1beta1.GroupVersion.String(),
					Kind:       "Cryostat",
					Name:       "cryostat-reports",
					UID:        "",
					Controller: &c,
				},
			},
			Labels: map[string]string{
				"app":       "cryostat",
				"component": "reports",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app":       "cryostat",
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

func NewCustomizedCoreService() *corev1.Service {
	svc := NewCryostatService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 8080
	svc.Spec.Ports[1].Port = 9095
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":       "cryostat",
		"component": "cryostat",
		"my":        "label",
	}
	return svc
}

func NewCustomizedGrafanaService() *corev1.Service {
	svc := NewGrafanaService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 8080
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":       "cryostat",
		"component": "cryostat",
		"my":        "label",
	}
	return svc
}

func NewCustomizedReportsService() *corev1.Service {
	svc := NewReportsService()
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].Port = 13161
	svc.Annotations = map[string]string{
		"my/custom": "annotation",
	}
	svc.Labels = map[string]string{
		"app":       "cryostat",
		"component": "reports",
		"my":        "label",
	}
	return svc
}

func NewTestService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
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

func NewCryostatCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat.default.svc",
			DNSNames: []string{
				"cryostat",
				"cryostat.default.svc",
				"cryostat.default.svc.cluster.local",
			},
			SecretName: "cryostat-tls",
			Keystores: &certv1.CertificateKeystores{
				PKCS12: &certv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: certMeta.SecretKeySelector{
						LocalObjectReference: certMeta.LocalObjectReference{
							Name: "cryostat-keystore",
						},
						Key: "KEYSTORE_PASS",
					},
				},
			},
			IssuerRef: certMeta.ObjectReference{
				Name: "cryostat-ca",
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

func NewGrafanaCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-grafana",
			Namespace: "default",
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat-grafana.default.svc",
			DNSNames: []string{
				"cryostat-grafana",
				"cryostat-grafana.default.svc",
				"cryostat-grafana.default.svc.cluster.local",
				"cryostat-health.local",
			},
			SecretName: "cryostat-grafana-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: "cryostat-ca",
			},
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
			},
		},
	}
}

func NewReportsCert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-reports",
			Namespace: "default",
		},
		Spec: certv1.CertificateSpec{
			CommonName: "cryostat-reports.default.svc",
			DNSNames: []string{
				"cryostat-reports",
				"cryostat-reports.default.svc",
				"cryostat-reports.default.svc.cluster.local",
			},
			SecretName: "cryostat-reports-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: "cryostat-ca",
			},
			Usages: []certv1.KeyUsage{
				certv1.UsageDigitalSignature,
				certv1.UsageKeyEncipherment,
				certv1.UsageServerAuth,
			},
		},
	}
}

func NewCACert() *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-ca",
			Namespace: "default",
		},
		Spec: certv1.CertificateSpec{
			CommonName: "ca.cryostat.cert-manager",
			SecretName: "cryostat-ca",
			IssuerRef: certMeta.ObjectReference{
				Name: "cryostat-self-signed",
			},
			IsCA: true,
		},
	}
}

func newCASecret(certData []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-ca",
			Namespace: "default",
		},
		Data: map[string][]byte{
			corev1.TLSCertKey: certData,
		},
	}
}

func NewSelfSignedIssuer() *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-self-signed",
			Namespace: "default",
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				SelfSigned: &certv1.SelfSignedIssuer{},
			},
		},
	}
}

func NewCryostatCAIssuer() *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-ca",
			Namespace: "default",
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				CA: &certv1.CAIssuer{
					SecretName: "cryostat-ca",
				},
			},
		},
	}
}

func NewJMXAuthSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-jmx-auth",
			Namespace: "default",
		},
		Data: map[string][]byte{
			operatorv1beta1.DefaultUsernameKey: []byte("hello"),
			operatorv1beta1.DefaultPasswordKey: []byte("world"),
		},
	}
}

func NewJMXAuthSecretForCryostat() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat-jmx-auth",
			Namespace: "default",
		},
		Data: map[string][]byte{
			operatorv1beta1.DefaultUsernameKey: []byte("hello"),
			operatorv1beta1.DefaultPasswordKey: []byte("world"),
		},
	}
}

func newPVC(spec *corev1.PersistentVolumeClaimSpec, labels map[string]string,
	annotations map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cryostat",
			Namespace:   "default",
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: *spec,
	}
}

func NewDefaultPVC() *corev1.PersistentVolumeClaim {
	return newPVC(&corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("500Mi"),
			},
		},
	}, map[string]string{
		"app": "cryostat",
	}, nil)
}

func NewCustomPVC() *corev1.PersistentVolumeClaim {
	storageClass := "cool-storage"
	return newPVC(&corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
		},
	}, map[string]string{
		"my":  "label",
		"app": "cryostat",
	}, map[string]string{
		"my/custom": "annotation",
	})
}

func NewCustomPVCSomeDefault() *corev1.PersistentVolumeClaim {
	storageClass := ""
	return newPVC(&corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClass,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			},
		},
	}, map[string]string{
		"app": "cryostat",
	}, nil)
}

func NewDefaultPVCWithLabel() *corev1.PersistentVolumeClaim {
	return newPVC(&corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("500Mi"),
			},
		},
	}, map[string]string{
		"app": "cryostat",
		"my":  "label",
	}, nil)
}

func NewDefaultEmptyDir() *corev1.EmptyDirVolumeSource {
	sizeLimit := resource.MustParse("0")
	return &corev1.EmptyDirVolumeSource{
		SizeLimit: &sizeLimit,
	}
}

func NewEmptyDirWithSpec() *corev1.EmptyDirVolumeSource {
	sizeLimit := resource.MustParse("200Mi")
	return &corev1.EmptyDirVolumeSource{
		Medium:    "Memory",
		SizeLimit: &sizeLimit,
	}
}

func NewCorePorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 8181,
		},
		{
			ContainerPort: 9091,
		},
	}
}

func NewGrafanaPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 3000,
		},
	}
}

func NewDatasourcePorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 8080,
		},
	}
}

func NewReportsPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			ContainerPort: 10000,
		},
	}
}

func NewCoreEnvironmentVariables(minimal bool, tls bool, externalTLS bool, openshift bool, reportsUrl string, authProps bool) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_WEB_PORT",
			Value: "8181",
		},
		{
			Name:  "CRYOSTAT_WEB_HOST",
			Value: "cryostat.example.com",
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
	}

	if externalTLS {
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

	if !minimal {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "GRAFANA_DATASOURCE_URL",
				Value: "http://127.0.0.1:8080",
			})
		if externalTLS {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_EXT_URL",
					Value: "https://cryostat-grafana.example.com",
				})
		} else {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "GRAFANA_DASHBOARD_EXT_URL",
					Value: "http://cryostat-grafana.example.com",
				})
		}
		if tls {
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
	if !tls {
		envs = append(envs,
			corev1.EnvVar{
				Name:  "CRYOSTAT_DISABLE_SSL",
				Value: "true",
			})
		if externalTLS {
			envs = append(envs,
				corev1.EnvVar{
					Name:  "CRYOSTAT_SSL_PROXIED",
					Value: "true",
				})
		}
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "KEYSTORE_PATH",
			Value: "/var/run/secrets/operator.cryostat.io/cryostat-tls/keystore.p12",
		})
	}
	if openshift {
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
				Value: "cryostat",
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

func NewGrafanaEnvironmentVariables(tls bool) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "JFR_DATASOURCE_URL",
			Value: "http://127.0.0.1:8080",
		},
	}
	if tls {
		envs = append(envs, corev1.EnvVar{
			Name:  "GF_SERVER_PROTOCOL",
			Value: "https",
		}, corev1.EnvVar{
			Name:  "GF_SERVER_CERT_KEY",
			Value: "/var/run/secrets/operator.cryostat.io/cryostat-grafana-tls/tls.key",
		}, corev1.EnvVar{
			Name:  "GF_SERVER_CERT_FILE",
			Value: "/var/run/secrets/operator.cryostat.io/cryostat-grafana-tls/tls.crt",
		})
	}
	return envs
}

func NewDatasourceEnvironmentVariables() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "LISTEN_HOST",
			Value: "127.0.0.1",
		},
	}
}

func NewReportsEnvironmentVariables(tls bool, resources corev1.ResourceRequirements) []corev1.EnvVar {
	opts := "-XX:+PrintCommandLineFlags -XX:ActiveProcessorCount=1 -Dorg.openjdk.jmc.flightrecorder.parser.singlethreaded=true"
	if !resources.Limits.Cpu().IsZero() {
		// Assume 2 CPU limit
		opts = "-XX:+PrintCommandLineFlags -XX:ActiveProcessorCount=2 -Dorg.openjdk.jmc.flightrecorder.parser.singlethreaded=false"
	}
	envs := []corev1.EnvVar{
		{
			Name:  "QUARKUS_HTTP_HOST",
			Value: "0.0.0.0",
		},
		{
			Name:  "JAVA_OPTIONS",
			Value: opts,
		},
	}
	if tls {
		envs = append(envs, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_PORT",
			Value: "10000",
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_KEY_FILE",
			Value: "/var/run/secrets/operator.cryostat.io/cryostat-reports-tls/tls.key",
		}, corev1.EnvVar{
			Name:  "QUARKUS_HTTP_SSL_CERTIFICATE_FILE",
			Value: "/var/run/secrets/operator.cryostat.io/cryostat-reports-tls/tls.crt",
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

func NewCoreEnvFromSource(tls bool) []corev1.EnvFromSource {
	envsFrom := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cryostat-jmx-auth",
				},
			},
		},
	}
	if tls {
		envsFrom = append(envsFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cryostat-keystore",
				},
			},
		})
	}
	return envsFrom
}

func NewGrafanaEnvFromSource() []corev1.EnvFromSource {
	return []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cryostat-grafana-basic",
				},
			},
		},
	}
}

func NewWsConnectionsEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_MAX_WS_CONNECTIONS",
			Value: "10",
		},
	}
}

func NewReportSubprocessHeapEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_REPORT_GENERATION_MAX_HEAP",
			Value: "500",
		},
	}
}

func NewJmxCacheOptionsEnv() []corev1.EnvVar {
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

func NewCoreVolumeMounts(tls bool) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "cryostat",
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/conf.d",
			SubPath:   "config",
		},
		{
			Name:      "cryostat",
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/recordings.d",
			SubPath:   "flightrecordings",
		},
		{
			Name:      "cryostat",
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/templates.d",
			SubPath:   "templates",
		},
		{
			Name:      "cryostat",
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/clientlib.d",
			SubPath:   "clientlib",
		},
		{
			Name:      "cryostat",
			ReadOnly:  false,
			MountPath: "/opt/cryostat.d/probes.d",
			SubPath:   "probes",
		},
		{
			Name:      "cryostat",
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
	if tls {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "keystore",
				ReadOnly:  true,
				MountPath: "/var/run/secrets/operator.cryostat.io/cryostat-tls",
			})
	}
	return mounts
}

func NewGrafanaVolumeMounts(tls bool) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	if tls {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "grafana-tls-secret",
				MountPath: "/var/run/secrets/operator.cryostat.io/cryostat-grafana-tls",
				ReadOnly:  true,
			})
	}
	return mounts
}

func NewReportsVolumeMounts(tls bool) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}
	if tls {
		mounts = append(mounts,
			corev1.VolumeMount{
				Name:      "reports-tls-secret",
				MountPath: "/var/run/secrets/operator.cryostat.io/cryostat-reports-tls",
				ReadOnly:  true,
			})
	}
	return mounts
}

func NewVolumeMountsWithTemplates(tls bool) []corev1.VolumeMount {
	return append(NewCoreVolumeMounts(tls),
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

func NewVolumeMountsWithAuthProperties(tls bool) []corev1.VolumeMount {
	return append(NewCoreVolumeMounts(tls), NewAuthPropertiesVolumeMount())
}

func NewAuthPropertiesVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "auth-properties-authConfigMapName",
		ReadOnly:  true,
		MountPath: "/app/resources/io/cryostat/net/openshift/OpenShiftAuthManager.properties",
		SubPath:   "OpenShiftAuthManager.properties",
	}
}

func NewCoreLivenessProbe(tls bool) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: newCoreProbeHandler(tls),
	}
}

func NewCoreStartupProbe(tls bool) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:     newCoreProbeHandler(tls),
		FailureThreshold: 18,
	}
}

func newCoreProbeHandler(tls bool) corev1.ProbeHandler {
	protocol := corev1.URISchemeHTTPS
	if !tls {
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

func NewGrafanaLivenessProbe(tls bool) *corev1.Probe {
	protocol := corev1.URISchemeHTTPS
	if !tls {
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

func NewDatasourceLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"curl", "--fail", "http://127.0.0.1:8080"},
			},
		},
	}
}

func NewReportsLivenessProbe(tls bool) *corev1.Probe {
	protocol := corev1.URISchemeHTTPS
	if !tls {
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

func NewMainDeploymentSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       "cryostat",
			"kind":      "cryostat",
			"component": "cryostat",
		},
	}
}

func NewReportsDeploymentSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"app":       "cryostat",
			"kind":      "cryostat",
			"component": "reports",
		},
	}
}

func OtherDeployment() *appsv1.Deployment {
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
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
					Name:      "cryostat",
					Namespace: "default",
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
			Selector: metav1.AddLabelToSelector(&metav1.LabelSelector{}, "other", "label"),
			Replicas: &replicas,
		},
	}
}

func NewVolumes(minimal bool, tls bool) []corev1.Volume {
	return newVolumes(minimal, tls, nil)
}

func NewVolumesWithSecrets(tls bool) []corev1.Volume {
	mode := int32(0440)
	return newVolumes(false, tls, []corev1.VolumeProjection{
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

func NewVolumesWithTemplates(tls bool) []corev1.Volume {
	mode := int32(0440)
	return append(NewVolumes(false, tls),
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

func NewVolumeWithAuthProperties(tls bool) []corev1.Volume {
	return append(NewVolumes(false, tls), NewAuthPropertiesVolume())
}

func NewAuthPropertiesVolume() corev1.Volume {
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

func newVolumes(minimal bool, tls bool, certProjections []corev1.VolumeProjection) []corev1.Volume {
	readOnlymode := int32(0440)
	volumes := []corev1.Volume{
		{
			Name: "cryostat",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "cryostat",
					ReadOnly:  false,
				},
			},
		},
	}
	projs := append([]corev1.VolumeProjection{}, certProjections...)
	if tls {
		projs = append(projs, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cryostat-tls",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "ca.crt",
						Path: "cryostat-ca.crt",
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
						SecretName: "cryostat-tls",
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
		if !minimal {
			volumes = append(volumes,
				corev1.Volume{
					Name: "grafana-tls-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "cryostat-grafana-tls",
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

func NewReportsVolumes(tls bool) []corev1.Volume {
	if !tls {
		return nil
	}
	return []corev1.Volume{
		{
			Name: "reports-tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "cryostat-reports-tls",
				},
			},
		},
	}
}

func NewPodSecurityContext() *corev1.PodSecurityContext {
	fsGroup := int64(18500)
	return &corev1.PodSecurityContext{
		FSGroup: &fsGroup,
	}
}

func NewCoreRoute(tls bool) *routev1.Route {
	return newRoute("cryostat", 8181, tls)
}

func NewCustomCoreRoute(tls bool) *routev1.Route {
	route := NewCoreRoute(tls)
	route.Annotations = map[string]string{"custom": "annotation"}
	route.Labels = map[string]string{"custom": "label"}
	return route
}

func NewGrafanaRoute(tls bool) *routev1.Route {
	return newRoute("cryostat-grafana", 3000, tls)
}

func NewCustomGrafanaRoute(tls bool) *routev1.Route {
	route := NewGrafanaRoute(tls)
	route.Annotations = map[string]string{"grafana": "annotation"}
	route.Labels = map[string]string{"grafana": "label"}
	return route
}

func newRoute(name string, port int, tls bool) *routev1.Route {
	var routeTLS *routev1.TLSConfig
	if !tls {
		routeTLS = &routev1.TLSConfig{
			Termination:                   routev1.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		}
	} else {
		routeTLS = &routev1.TLSConfig{
			Termination:              routev1.TLSTerminationReencrypt,
			DestinationCACertificate: "cryostat-ca-bytes",
		}
	}
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
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

func OtherCoreRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cryostat",
			Namespace:   "default",
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

func OtherGrafanaRoute() *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cryostat-grafana",
			Namespace:   "default",
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

func OtherCoreIngress() *netv1.Ingress {
	pathtype := netv1.PathTypePrefix
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cryostat",
			Namespace:   "default",
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

func OtherGrafanaIngress() *netv1.Ingress {
	pathtype := netv1.PathTypePrefix
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cryostat-grafana",
			Namespace:   "default",
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

func NewNetworkConfigurationList(tls bool) operatorv1beta1.NetworkConfigurationList {
	coreSVC := NewCryostatService()
	coreIng := NewNetworkConfiguration(coreSVC.Name, coreSVC.Spec.Ports[0].Port, tls)

	grafanaSVC := NewGrafanaService()
	grafanaIng := NewNetworkConfiguration(grafanaSVC.Name, grafanaSVC.Spec.Ports[0].Port, tls)

	return operatorv1beta1.NetworkConfigurationList{
		CoreConfig:    &coreIng,
		GrafanaConfig: &grafanaIng,
	}
}

func NewNetworkConfiguration(svcName string, svcPort int32, tls bool) operatorv1beta1.NetworkConfiguration {
	pathtype := netv1.PathTypePrefix
	host := svcName + ".example.com"

	var ingressTLS []netv1.IngressTLS
	if tls {
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

func NewServiceAccount(isOpenShift bool) *corev1.ServiceAccount {
	var annotations map[string]string
	if isOpenShift {
		annotations = map[string]string{
			"serviceaccounts.openshift.io/oauth-redirectreference.route": `{"metadata":{"creationTimestamp":null},"reference":{"group":"","kind":"Route","name":"cryostat"}}`,
		}
	}

	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
			Labels: map[string]string{
				"app": "cryostat",
			},
			Annotations: annotations,
		},
	}
}

func OtherServiceAccount() *corev1.ServiceAccount {
	disable := false
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
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
				Name: "cryostat-dockercfg-abcde",
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: "cryostat-dockercfg-abcde",
			},
			{
				Name: "cryostat-token-abcde",
			},
		},
		AutomountServiceAccountToken: &disable,
	}
}

func NewRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{
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
		},
	}
}

func NewAuthClusterRole() *rbacv1.ClusterRole {
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

func NewRoleBinding() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cryostat",
			Namespace: "default",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "cryostat",
				Namespace: "default",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "cryostat",
		},
	}
}

func NewClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cryostat-9ecd5050500c2566765bc593edfcce12434283e5da32a27476bc4a1569304a02",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "cryostat",
				Namespace: "default",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cryostat-operator-cryostat",
		},
	}
}

func NewTemplateConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "templateCM1",
			Namespace: "default",
		},
		Data: map[string]string{
			"template.jfc": "XML template data",
		},
	}
}

func NewOtherTemplateConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "templateCM2",
			Namespace: "default",
		},
		Data: map[string]string{
			"other-template.jfc": "more XML template data",
		},
	}
}

func NewAuthPropertiesConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "authConfigMapName",
			Namespace: "default",
		},
		Data: map[string]string{
			"auth.properties": "CRYOSTAT_RESOURCE=resources.group\nANOTHER_CRYOSTAT_RESOURCE=another_resources.another_group",
		},
	}
}

func NewNamespace() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
}

func NewNamespaceWithSCCSupGroups() *corev1.Namespace {
	ns := NewNamespace()
	ns.Annotations = map[string]string{
		securityv1.SupplementalGroupsAnnotation: "1000130000/10000",
	}
	return ns
}

func NewConsoleLink() *consolev1.ConsoleLink {
	return &consolev1.ConsoleLink{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cryostat-9ecd5050500c2566765bc593edfcce12434283e5da32a27476bc4a1569304a02",
		},
		Spec: consolev1.ConsoleLinkSpec{
			Link: consolev1.Link{
				Text: "Cryostat",
				Href: "https://cryostat.example.com",
			},
			Location: consolev1.NamespaceDashboard,
			NamespaceDashboard: &consolev1.NamespaceDashboardSpec{
				Namespaces: []string{"default"},
			},
		},
	}
}

func NewApiServer() *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			AdditionalCORSAllowedOrigins: []string{"https://an-existing-user-specified\\.allowed\\.origin\\.com"},
		},
	}
}
