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

package common

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("common")

// OSUtils is an abstraction on functionality that interacts with the operating system
type OSUtils interface {
	GetEnv(name string) string
	GetEnvOrDefault(name string, defaultVal string) string
	GetFileContents(path string) ([]byte, error)
	GenPasswd(length int) string
}

type DefaultOSUtils struct{}

// GetEnv returns the value of the environment variable with the provided name. If no such
// variable exists, the empty string is returned.
func (o *DefaultOSUtils) GetEnv(name string) string {
	return os.Getenv(name)
}

// GetEnvOrDefault returns the value of the environment variable with the provided name.
// If no such variable exists, the provided default value is returned.
func (o *DefaultOSUtils) GetEnvOrDefault(name string, defaultVal string) string {
	val := o.GetEnv(name)
	if len(val) > 0 {
		return val
	}
	return defaultVal
}

// GetFileContents reads and returns the entire file contents specified by the path
func (o *DefaultOSUtils) GetFileContents(path string) ([]byte, error) {
	return ioutil.ReadFile(path)
}

// GenPasswd generates a psuedorandom password of a given length.
func (o *DefaultOSUtils) GenPasswd(length int) string {
	rand.Seed(time.Now().UnixNano())
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// ClusterUniqueName returns a name for cluster-scoped objects that is
// uniquely identified by a namespace and name.
func ClusterUniqueName(gvk *schema.GroupVersionKind, name string, namespace string) string {
	return ClusterUniqueNameWithPrefix(gvk, "", name, namespace)
}

// ClusterUniqueNameWithPrefix returns a name for cluster-scoped objects that is
// uniquely identified by a namespace and name. Appends the prefix to the
// provided Kind.
func ClusterUniqueNameWithPrefix(gvk *schema.GroupVersionKind, prefix string, name string, namespace string) string {
	return ClusterUniqueNameWithPrefixTargetNS(gvk, prefix, name, namespace, "")
}

// ClusterUniqueShortName returns a name for cluster-scoped objects that is
// uniquely identified by a namespace and name. Appends the prefix to the
// provided Kind. The total length should be at most 63 characters.
func ClusterUniqueShortNameWithPrefix(gvk *schema.GroupVersionKind, prefix string, name string, namespace string) string {
	return clusterUniqueName(gvk, prefix, name, namespace, "", true)
}

// ClusterUniqueNameWithPrefixTargetNS returns a name for cluster-scoped objects that is
// uniquely identified by a namespace and name, and a target namespace.
// Appends the prefix to the provided Kind.
func ClusterUniqueNameWithPrefixTargetNS(gvk *schema.GroupVersionKind, prefix string, name string, namespace string,
	targetNS string) string {
	return clusterUniqueName(gvk, prefix, name, namespace, targetNS, false)
}

func clusterUniqueName(gvk *schema.GroupVersionKind, prefix string, name string, namespace string, targetNS string,
	short bool) string {
	prefixWithKind := strings.ToLower(gvk.Kind)
	if len(prefix) > 0 {
		prefixWithKind += "-" + prefix
	}

	toHash := namespace + "/" + name
	if len(targetNS) > 0 {
		toHash += "/" + targetNS
	}

	var suffix string
	if short {
		// Use the 128-bit FNV-1 checksum of the namespaced name as a suffix.
		// Suffix is 32 bytes
		hash := fnv.New128()
		hash.Write([]byte(toHash))
		suffix = fmt.Sprintf("%x", hash.Sum([]byte{}))
	} else {
		// Use the SHA256 checksum of the namespaced name as a suffix
		suffix = fmt.Sprintf("%x", sha256.Sum256([]byte(toHash)))
	}
	return prefixWithKind + "-" + suffix
}

// MergeLabelsAndAnnotations copies labels and annotations from a source
// to the destination ObjectMeta, overwriting any existing labels and
// annotations of the same key.
func MergeLabelsAndAnnotations(dest *metav1.ObjectMeta, srcLabels, srcAnnotations map[string]string) {
	// Check and create labels/annotations map if absent
	if dest.Labels == nil {
		dest.Labels = map[string]string{}
	}
	if dest.Annotations == nil {
		dest.Annotations = map[string]string{}
	}

	// Merge labels and annotations, preferring those in the source
	for k, v := range srcLabels {
		dest.Labels[k] = v
	}
	for k, v := range srcAnnotations {
		dest.Annotations[k] = v
	}
}

// LabelsForTargetNamespaceObject returns a set of labels for an object in a
// target namespace that refer back to the CR associated with the object.
func LabelsForTargetNamespaceObject(cr *model.CryostatInstance) map[string]string {
	return map[string]string{
		constants.TargetNamespaceCRNameLabel:      cr.Name,
		constants.TargetNamespaceCRNamespaceLabel: cr.InstallNamespace,
	}
}

// Matches image tags of the form "major.minor.patch"
var develVerRegexp = regexp.MustCompile(`(?i)(:latest|SNAPSHOT|dev|BETA\d+)$`)

// GetPullPolicy returns an image pull policy based on the image tag provided.
func GetPullPolicy(imageTag string) corev1.PullPolicy {
	// Use Always for tags that have a known development suffix
	if develVerRegexp.MatchString(imageTag) {
		return corev1.PullAlways
	}
	// Likely a release, use IfNotPresent
	return corev1.PullIfNotPresent
}

// PopulateResourceRequest configures ResourceRequirements, applying defaults and checking that
// requests are not larger than limits
func PopulateResourceRequest(resources *corev1.ResourceRequirements, defaultCpuRequest, defaultMemoryRequest,
	defaultCpuLimit, defaultMemoryLimit string) {
	// Check if the resources have already been customized
	custom := resources.Requests != nil || resources.Limits != nil

	if resources.Requests == nil {
		resources.Requests = corev1.ResourceList{}
	}
	requests := resources.Requests
	if _, found := requests[corev1.ResourceCPU]; !found {
		requests[corev1.ResourceCPU] = resource.MustParse(defaultCpuRequest)
	}
	if _, found := requests[corev1.ResourceMemory]; !found {
		requests[corev1.ResourceMemory] = resource.MustParse(defaultMemoryRequest)
	}

	// Only add default limits if resources have not been customized
	if !custom {
		resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(defaultCpuLimit),
			corev1.ResourceMemory: resource.MustParse(defaultMemoryLimit),
		}
	}

	// Ensure resource requests do not exceed limits
	checkResourceRequestWithLimit(requests, resources.Limits)
}

func checkResourceRequestWithLimit(requests, limits corev1.ResourceList) {
	if limits != nil {
		if limitCpu, found := limits[corev1.ResourceCPU]; found && limitCpu.Cmp(*requests.Cpu()) < 0 {
			requests[corev1.ResourceCPU] = limitCpu.DeepCopy()
		}
		if limitMemory, found := limits[corev1.ResourceMemory]; found && limitMemory.Cmp(*requests.Memory()) < 0 {
			requests[corev1.ResourceMemory] = limitMemory.DeepCopy()
		}
	}
}

const annotationSecretHash = "io.cryostat/secret-hash"
const annotationConfigMapHash = "io.cryostat/config-map-hash"

// AnnotateWithObjRefHashes annotates the provided pod template with hashes of the secret and config map data used
// by this pod template. This allows the pod template parent to automatically roll out a new revision when
// the hashed data changes.
func AnnotateWithObjRefHashes(ctx context.Context, client client.Client, namespace string, template *corev1.PodTemplateSpec) error {
	if template.Annotations == nil {
		template.Annotations = map[string]string{}
	}

	// Collect names of secrets and config maps used by this pod template
	secrets := newObjectSet[string]()
	configMaps := newObjectSet[string]()

	// Look for secrets and config maps references in environment variables
	for _, container := range template.Spec.Containers {
		// Look through Env[].ValueFrom for secret/config map refs
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.SecretKeyRef != nil {
					secrets.add(env.ValueFrom.SecretKeyRef.Name)
				} else if env.ValueFrom.ConfigMapKeyRef != nil {
					configMaps.add(env.ValueFrom.ConfigMapKeyRef.Name)
				}
			}
		}
		// Look through EnvFrom for secret/config map refs
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				secrets.add(envFrom.SecretRef.Name)
			} else if envFrom.ConfigMapRef != nil {
				configMaps.add(envFrom.ConfigMapRef.Name)
			}
		}
	}

	// Look for secrets and config maps references in volumes
	for _, vol := range template.Spec.Volumes {
		if vol.Secret != nil {
			// Look for secret volumes
			secrets.add(vol.Secret.SecretName)
		} else if vol.ConfigMap != nil {
			// Look for config map volumes
			configMaps.add(vol.ConfigMap.Name)
		} else if vol.Projected != nil {
			// Also look for secret/config map sources in projected volumes
			for _, source := range vol.Projected.Sources {
				if source.Secret != nil {
					secrets.add(source.Secret.Name)
				} else if source.ConfigMap != nil {
					configMaps.add(source.ConfigMap.Name)
				}
			}
		}
	}

	// Hash the discovered secrets and config maps
	secretHash, err := hashSecrets(ctx, client, namespace, secrets)
	if err != nil {
		return err
	}
	configMapHash, err := hashConfigMaps(ctx, client, namespace, configMaps)
	if err != nil {
		return err
	}

	// Apply the hashes as annotations to the pod template
	template.Annotations[annotationSecretHash] = *secretHash
	template.Annotations[annotationConfigMapHash] = *configMapHash
	return nil
}

func hashSecrets(ctx context.Context, client client.Client, namespace string, secrets *objectSet[string]) (*string, error) {
	// Collect the JSON of all secret data, sorted by object name
	combinedJSON := []byte{}
	for _, name := range secrets.toSortedSlice() {
		secret := &corev1.Secret{}
		err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
		if err != nil {
			return nil, err
		}
		// Marshal secret data as JSON. Keys are sorted, see: [json.Marshal]
		buf, err := json.Marshal(secret.Data)
		if err != nil {
			return nil, err
		}
		combinedJSON = append(combinedJSON, buf...)
	}
	// Hash the JSON with SHA256
	hashed := fmt.Sprintf("%x", sha256.Sum256(combinedJSON))
	return &hashed, nil
}

func hashConfigMaps(ctx context.Context, client client.Client, namespace string, configMaps *objectSet[string]) (*string, error) {
	// Collect the JSON of all config map data, sorted by object name
	combinedJSON := []byte{}
	for _, name := range configMaps.toSortedSlice() {
		cm := &corev1.ConfigMap{}
		err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm)
		if err != nil {
			return nil, err
		}
		// Marshal config map data as JSON. Keys are sorted, see: [json.Marshal]
		buf, err := json.Marshal(cm.Data)
		if err != nil {
			return nil, err
		}
		combinedJSON = append(combinedJSON, buf...)
	}
	// Hash the JSON with FNV-1
	hash := fnv.New128()
	hash.Write([]byte(combinedJSON))
	hashed := fmt.Sprintf("%x", hash.Sum([]byte{}))
	return &hashed, nil
}

// SeccompProfile returns a SeccompProfile for the restricted
// Pod Security Standard that, on OpenShift, is backwards-compatible
// with OpenShift < 4.11.
// TODO Remove once OpenShift < 4.11 support is dropped
func SeccompProfile(openshift bool) *corev1.SeccompProfile {
	// For backward-compatibility with OpenShift < 4.11,
	// leave the seccompProfile empty. In OpenShift >= 4.11,
	// the restricted-v2 SCC will populate it for us.
	if openshift {
		return nil
	}
	return &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}
}

// Set abstraction for collecting names of secrets and config maps used by a pod
type objectSet[T cmp.Ordered] struct {
	impl map[T]struct{}
}

func newObjectSet[T cmp.Ordered]() *objectSet[T] {
	return &objectSet[T]{
		impl: map[T]struct{}{},
	}
}

func (s *objectSet[T]) add(obj T) {
	s.impl[obj] = struct{}{}
}

func (s *objectSet[T]) toSortedSlice() []T {
	// Convert set to a sorted slice
	slice := make([]T, 0, len(s.impl))
	for k := range s.impl {
		slice = append(slice, k)
	}
	slices.Sort(slice)
	return slice
}
