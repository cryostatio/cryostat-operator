package resource_definitions

import (
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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

func NewCoreConfigMap(cr *rhjmcv1alpha1.ContainerJFR) *corev1.ConfigMap {
	return NewConfigMap(cr.Name, cr.Name, cr.Namespace, map[string]string{"expose.config.fabric8.io/host-key": "CONTAINER_JFR_WEB_HOST"})
}

func NewCommandChannelConfigMap(cr *rhjmcv1alpha1.ContainerJFR) *corev1.ConfigMap {
	return NewConfigMap(cr.Name+"-command", cr.Name, cr.Namespace, map[string]string{"expose.config.fabric8.io/host-key": "CONTAINER_JFR_LISTEN_HOST"})
}

func NewJfrDatasourceConfigMap(cr *rhjmcv1alpha1.ContainerJFR) *corev1.ConfigMap {
	return NewConfigMap(cr.Name+"-jfr-datasource", cr.Name, cr.Namespace, map[string]string{"expose.config.fabric8.io/url-key": "GRAFANA_DATASOURCE_URL"})
}

func NewGrafanaConfigMap(cr *rhjmcv1alpha1.ContainerJFR) *corev1.ConfigMap {
	return NewConfigMap(cr.Name+"-grafana", cr.Name, cr.Namespace, map[string]string{"expose.config.fabric8.io/url-key": "GRAFANA_DASHBOARD_URL"})
}

func NewConfigMap(name string, appName string, namespace string, annotations map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
			Labels: map[string]string{
				"app": appName,
			},
		},
	}
}

func NewPodForCR(cr *rhjmcv1alpha1.ContainerJFR) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app": cr.Name,
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
			Containers: []corev1.Container{
				NewCoreContainer(cr),
				NewGrafanaContainer(cr),
				NewJfrDatasourceContainer(cr),
			},
		},
	}
}

func NewCoreContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name,
		Image: "quay.io/rh-jmc-team/container-jfr:0.4.7",
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
		Env: []corev1.EnvVar{
			{
				Name:  "CONTAINER_JFR_WEB_PORT",
				Value: "8181",
			},
			{
				Name:  "CONTAINER_JFR_EXT_WEB_PORT",
				Value: "80",
			},
			{
				Name: "CONTAINER_JFR_WEB_HOST",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr",
						},
						Key: "CONTAINER_JFR_WEB_HOST",
					},
				},
			},
			{
				Name:  "CONTAINER_JFR_LISTEN_PORT",
				Value: "9090",
			},
			{
				Name:  "CONTAINER_JFR_EXT_LISTEN_PORT",
				Value: "80",
			},
			{
				Name: "CONTAINER_JFR_LISTEN_HOST",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr-command",
						},
						Key: "CONTAINER_JFR_LISTEN_HOST",
					},
				},
			},
			{
				Name: "GRAFANA_DASHBOARD_URL",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr-grafana",
						},
						Key: "GRAFANA_DASHBOARD_URL",
					},
				},
			},
			{
				Name: "GRAFANA_DATASOURCE_URL",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "containerjfr-jfr-datasource",
						},
						Key: "GRAFANA_DATASOURCE_URL",
					},
				},
			},
		},
	}
}

func NewGrafanaContainer(cr *rhjmcv1alpha1.ContainerJFR) corev1.Container {
	return corev1.Container{
		Name:  cr.Name + "-grafana",
		Image: "docker.io/grafana/grafana:6.2.2",
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
		Image: "quay.io/rh-jmc-team/jfr-datasource",
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
				"app": cr.Name,
			},
			Annotations: map[string]string{
				"fabric8.io/expose":     "true",
				"fabric8.io/exposePort": "8181",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
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
				"app": cr.Name,
			},
			Annotations: map[string]string{
				"fabric8.io/expose":     "true",
				"fabric8.io/exposePort": "9090",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
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
			Annotations: map[string]string{
				"fabric8.io/expose":     "true",
				"fabric8.io/exposePort": "3000",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
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
				"component": "grafana",
			},
			Annotations: map[string]string{
				"fabric8.io/expose": "true",
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
