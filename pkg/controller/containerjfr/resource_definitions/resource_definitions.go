package resource_definitions

import (
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServiceSpecs struct {
	CoreAddress       string
	CommandAddress    string
	GrafanaAddress    string
	DatasourceAddress string
}

func NewPersistentVolumeClaimForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.PersistentVolumeClaim {
	storageClassName := ""
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": *resource.NewQuantity(500*1024*1024, resource.BinarySI),
				},
			},
		},
	}
}

func NewPodForCR(cr *rhjmcv1alpha1.ContainerJFR, specs *ServiceSpecs) *corev1.Pod {
	var containers []corev1.Container
	if cr.Spec.Minimal {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs),
		}
	} else {
		containers = []corev1.Container{
			NewCoreContainer(cr, specs),
			NewGrafanaContainer(cr),
			NewJfrDatasourceContainer(cr),
		}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":  cr.Name,
				"kind": "containerjfr",
			},
			Annotations: map[string]string{
				"redhat.com/containerJfrUrl": specs.CoreAddress,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "container-jfr-operator",
			Volumes: []corev1.Volume{
				{
					Name: cr.Name,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: cr.Name,
						},
					},
				},
			},
			Containers: containers,
		},
	}
}

func NewCoreContainer(cr *rhjmcv1alpha1.ContainerJFR, specs *ServiceSpecs) corev1.Container {
	envs := []corev1.EnvVar{
		{
			Name:  "CONTAINER_JFR_SSL_PROXIED",
			Value: "true",
		},
		{
			Name:  "CONTAINER_JFR_ALLOW_UNTRUSTED_SSL",
			Value: "true",
		},
		{
			Name:  "CONTAINER_JFR_WEB_PORT",
			Value: "8181",
		},
		{
			Name:  "CONTAINER_JFR_EXT_WEB_PORT",
			Value: "443",
		},
		{
			Name:  "CONTAINER_JFR_WEB_HOST",
			Value: specs.CoreAddress,
		},
		{
			Name:  "CONTAINER_JFR_LISTEN_PORT",
			Value: "9090",
		},
		{
			Name:  "CONTAINER_JFR_EXT_LISTEN_PORT",
			Value: "443",
		},
		{
			Name:  "CONTAINER_JFR_LISTEN_HOST",
			Value: specs.CommandAddress,
		},
		{
			Name:  "GRAFANA_DASHBOARD_URL",
			Value: specs.GrafanaAddress,
		},
		{
			Name:  "GRAFANA_DATASOURCE_URL",
			Value: specs.DatasourceAddress,
		},
	}
	imageTag := "quay.io/rh-jmc-team/container-jfr:0.8.0"
	if cr.Spec.Minimal {
		imageTag += "-minimal"
		envs = append(envs, corev1.EnvVar{
			Name:  "USE_LOW_MEM_PRESSURE_STREAMING",
			Value: "true",
		})
	}
	return corev1.Container{
		Name:  cr.Name,
		Image: imageTag,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      cr.Name,
				MountPath: "flightrecordings",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8181,
			},
			{
				ContainerPort: 9090,
			},
			{
				ContainerPort: 9091,
			},
		},
		Env: envs,
	}
}

func NewGrafanaContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-grafana",
		Image: "docker.io/grafana/grafana:6.4.4",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3000,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "GF_INSTALL_PLUGINS",
				Value: "grafana-simple-json-datasource",
			},
		},
	}
}

func NewJfrDatasourceContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-jfr-datasource",
		Image: "quay.io/rh-jmc-team/jfr-datasource:0.0.1",
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8080,
			},
		},
		Env: []corev1.EnvVar{},
	}
}

func NewExporterService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "container-jfr",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "8181-tcp",
					Port:       8181,
					TargetPort: intstr.IntOrString{IntVal: 8181},
				},
				{
					Name:       "9091-tcp",
					Port:       9091,
					TargetPort: intstr.IntOrString{IntVal: 9091},
				},
			},
		},
	}
}

func NewCommandChannelService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-command",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "command-channel",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "9090-tcp",
					Port:       9090,
					TargetPort: intstr.IntOrString{IntVal: 9090},
				},
			},
		},
	}
}

func NewGrafanaService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "grafana",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "3000-tcp",
					Port:       3000,
					TargetPort: intstr.IntOrString{IntVal: 3000},
				},
			},
		},
	}
}

func NewJfrDatasourceService(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-jfr-datasource",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app":       cr.Name,
				"component": "jfr-datasource",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": cr.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "8080-tcp",
					Port:       8080,
					TargetPort: intstr.IntOrString{IntVal: 8080},
				},
			},
		},
	}
}
