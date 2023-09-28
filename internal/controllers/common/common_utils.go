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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("common")

// OSUtils is an abstraction on functionality that interacts with the operating system
type OSUtils interface {
	GetEnv(name string) string
	GetFileContents(path string) ([]byte, error)
	GenPasswd(length int) string
}

type defaultOSUtils struct{}

// GetEnv returns the value of the environment variable with the provided name. If no such
// variable exists, the empty string is returned.
func (o *defaultOSUtils) GetEnv(name string) string {
	return os.Getenv(name)
}

// GetFileContents reads and returns the entire file contents specified by the path
func (o *defaultOSUtils) GetFileContents(path string) ([]byte, error) {
	return ioutil.ReadFile(path)
}

// GenPasswd generates a psuedorandom password of a given length.
func (o *defaultOSUtils) GenPasswd(length int) string {
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
func ClusterUniqueName(kind string, name string, namespace string) string {
	// Use the SHA256 checksum of the namespaced name as a suffix
	nn := types.NamespacedName{Namespace: namespace, Name: name}
	suffix := fmt.Sprintf("%x", sha256.Sum256([]byte(nn.String())))
	return strings.ToLower(kind) + "-" + suffix
}

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
