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
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
// uniquely identified by a namespace and name. The total length should be
// at most 63 characters.
func ClusterUniqueShortName(gvk *schema.GroupVersionKind, name string, namespace string) string {
	return clusterUniqueName(gvk, "", name, namespace, "", true)
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
	if short {
		// Prefix is 5 characters: 4 from kind, and hyphen
		prefixWithKind = prefixWithKind[:4] // TODO can we use a weaker and shorter hash and keep the full kind?
	}
	if len(prefix) > 0 {
		prefixWithKind += "-" + prefix
	}

	toHash := namespace + "/" + name
	if len(targetNS) > 0 {
		toHash += "/" + targetNS
	}

	var suffix string
	if short {
		// Use the SHA224 checksum of the namespaced name as a suffix.
		// Suffix is 56 bytes
		suffix = fmt.Sprintf("%x", sha256.Sum224([]byte(toHash)))
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
