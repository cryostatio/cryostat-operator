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

package agent

import (
	"context"
	"errors"
	"fmt"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type podMutator struct {
	client client.Client
	log    *logr.Logger
	gvk    *schema.GroupVersionKind
	common.ReconcilerTLS
}

var _ admission.CustomDefaulter = &podMutator{}

const (
	agentArg      = "-javaagent:/tmp/cryostat-agent/cryostat-agent-shaded.jar"
	podNameEnvVar = "CRYOSTAT_AGENT_POD_NAME"
	podIPEnvVar   = "CRYOSTAT_AGENT_POD_IP"
)

// Default optionally mutates a pod to inject the Cryostat agent
func (r *podMutator) Default(ctx context.Context, obj runtime.Object) error {
	// FIXME Do not return error, it blocks pod creation. Use this:
	// https://github.com/kubernetes-sigs/controller-runtime/blob/3b032e16c0dc19d656626a288cd417d36beaebad/pkg/webhook/admission/defaulter_custom.go#L39
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod, but received a %T", obj)
	}
	// TODO could get very chatty (also name is blank for GenerateName)
	r.log.Info("checking for Cryostat labels", "name", pod.Name, "namespace", pod.Namespace)

	// TODO do this with objectSelector: https://github.com/kubernetes-sigs/controller-tools/issues/553
	// Check for required labels and return early if missing
	if !metav1.HasLabel(pod.ObjectMeta, LabelCryostatName) || !metav1.HasLabel(pod.ObjectMeta, LabelCryostatNamespace) {
		return nil
	}

	// Look up Cryostat
	cr := &operatorv1beta2.Cryostat{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      pod.Labels[LabelCryostatName],
		Namespace: pod.Labels[LabelCryostatNamespace],
	}, cr)
	if err != nil {
		return err
	}

	// Check if this pod is within a target namespace of the CR
	if !isTargetNamespace(cr.Status.TargetNamespaces, pod.Namespace) {
		return fmt.Errorf("pod's namespace \"%s\" is not a target namespace of Cryostat \"%s\" in \"%s\"",
			pod.Namespace, cr.Name, cr.Namespace)
	}

	// Check whether TLS is enabled for this CR
	tlsEnabled := r.IsCertManagerEnabled(model.FromCryostat(cr))

	// Select target container
	if len(pod.Spec.Containers) == 0 {
		return nil
	}
	// TODO make configurable
	container := &pod.Spec.Containers[0]

	// Add init container
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:            "cryostat-agent-init",
		Image:           "quay.io/cryostat/cryostat-agent-init:latest", // TODO related images
		ImagePullPolicy: corev1.PullAlways,                             // TODO change this
		Command:         []string{"cp", "-v", "/cryostat/agent/cryostat-agent-shaded.jar", "/tmp/cryostat-agent/cryostat-agent-shaded.jar"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "cryostat-agent-init",
				MountPath: "/tmp/cryostat-agent",
			},
		}, // TODO Resources?
	})

	// Add emptyDir volume to copy agent into, and mount it
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: "cryostat-agent-init",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{}, // TODO size limit?
		},
	})

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "cryostat-agent-init",
		MountPath: "/tmp/cryostat-agent",
		ReadOnly:  true,
	})

	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_BASEURI",
			Value: cryostatURL(cr, tlsEnabled),
		},
		corev1.EnvVar{
			Name: podNameEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_APP_NAME",
			Value: fmt.Sprintf("$(%s)", podNameEnvVar),
		},
		corev1.EnvVar{
			Name: podIPEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		corev1.EnvVar{
			Name:  "CRYOSTAT_AGENT_API_WRITES_ENABLED",
			Value: "true", // TODO default to writes enabled, separate label?
		},
	)

	// Append callback environment variables
	container.Env = append(container.Env, r.callbackEnv(cr, pod.Namespace, tlsEnabled)...)

	if tlsEnabled {
		// Mount the certificate volume
		readOnlyMode := int32(0440)
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "cryostat-agent-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					// TODO abstract implementation
					SecretName:  common.ClusterUniqueNameWithPrefixTargetNS(r.gvk, "agent", cr.Name, cr.Namespace, pod.Namespace),
					DefaultMode: &readOnlyMode,
				},
			},
		})

		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "cryostat-agent-tls",
			MountPath: "/var/run/secrets/io.cryostat/cryostat-agent",
			ReadOnly:  true,
		})

		// Configure the Cryostat agent to use client certificate authentication
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_CERT_PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSCertKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_CLIENT_AUTH_KEY_PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSPrivateKeyKey),
			},
		)

		// Configure the Cryostat agent to trust the Cryostat CA
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", constants.CAKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__TYPE",
				Value: "X.509",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBCLIENT_TLS_TRUSTSTORE_CERT_0__ALIAS",
				Value: "cryostat",
			},
		)

		// Configure the Cryostat agent to use HTTPS for its callback server
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_FILE",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSCertKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_TYPE",
				Value: "X.509",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_CERT_ALIAS",
				Value: "cryostat",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_PATH",
				Value: fmt.Sprintf("/var/run/secrets/io.cryostat/cryostat-agent/%s", corev1.TLSPrivateKeyKey),
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_TYPE",
				Value: "RSA",
			},
			corev1.EnvVar{
				Name:  "CRYOSTAT_AGENT_WEBSERVER_TLS_KEY_ALIAS",
				Value: "cryostat",
			},
		)
	}
	extended, err := extendJavaToolOptions(container.Env)
	if err != nil {
		return err
	}
	container.Env = extended

	// TODO JAVA_TOOL_OPTIONS for -javaagent?

	// TODO figure out authz. may not be suitable to create a service account in each target ns with pod/exec perms.
	// someone with access to the namespace should not necessarily have access to that permission.
	//
	// TokenRequest API, can we autorenew? Non-expiring token would work but security warns against using it. If
	// using non-expiring token. Remember RBAC is validated against install namespace. Need SA per target namespace,
	// with a RoleBinding in install namespace.
	//
	// could do an authz check on webhook user. if they have pod/exec permissions, then create the sa and/or secret.
	// this would count as a side effect of the webhook. this could still allow other ns users to escalate though.

	return nil
}

func cryostatURL(cr *operatorv1beta2.Cryostat, tls bool) string {
	scheme := "https"
	if !tls {
		scheme = "http"
	}
	// TODO see if this can be easily refactored
	port := constants.AgentProxyContainerPort
	if cr.Spec.ServiceOptions != nil && cr.Spec.ServiceOptions.AgentConfig != nil && cr.Spec.ServiceOptions.AgentConfig.HTTPPort != nil {
		port = *cr.Spec.ServiceOptions.AgentConfig.HTTPPort
	}
	return fmt.Sprintf("%s://%s-agent.%s.svc:%d", scheme, cr.Name, cr.Namespace, // TODO maybe use agent service instead of CR meta
		port)
}

func (r *podMutator) callbackEnv(cr *operatorv1beta2.Cryostat, namespace string, tls bool) []corev1.EnvVar {
	scheme := "https"
	if !tls {
		scheme = "http"
	}
	envs := []corev1.EnvVar{
		{
			Name:  "CRYOSTAT_AGENT_CALLBACK_SCHEME",
			Value: scheme,
		},
		{
			Name:  "CRYOSTAT_AGENT_CALLBACK_HOST_NAME",
			Value: fmt.Sprintf("$(%s), $(%s)[replace(\".\"\\, \"-\")]", podNameEnvVar, podIPEnvVar),
		},
		{
			Name:  "CRYOSTAT_AGENT_CALLBACK_DOMAIN_NAME",
			Value: fmt.Sprintf("%s.%s.svc", common.ClusterUniqueShortName(r.gvk, cr.Name, cr.Namespace), namespace),
		},
		{
			Name:  "CRYOSTAT_AGENT_CALLBACK_PORT",
			Value: "9977",
		},
	}

	return envs
}

func extendJavaToolOptions(envs []corev1.EnvVar) ([]corev1.EnvVar, error) {
	existing, err := findJavaToolOptions(envs)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		existing.Value += " " + agentArg
	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: agentArg,
		})
	}

	return envs, nil
}

var errJavaToolOptionsValueFrom error = errors.New("environment variable JAVA_TOOL_OPTIONS uses \"valueFrom\" and cannot be extended")

func findJavaToolOptions(envs []corev1.EnvVar) (*corev1.EnvVar, error) {
	for i, env := range envs {
		if env.Name == "JAVA_TOOL_OPTIONS" {
			if env.ValueFrom != nil {
				return nil, errJavaToolOptionsValueFrom
			}
			return &envs[i], nil
		}
	}
	return nil, nil
}

func isTargetNamespace(targetNamespaces []string, podNamespace string) bool {
	for _, ns := range targetNamespaces {
		if ns == podNamespace {
			return true
		}
	}
	return false
}
