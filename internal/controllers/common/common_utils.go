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

package common

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
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
func ClusterUniqueName(gvk *schema.GroupVersionKind, name string, namespace string) string {
	// Use the SHA256 checksum of the namespaced name as a suffix
	nn := types.NamespacedName{Namespace: namespace, Name: name}
	suffix := fmt.Sprintf("%x", sha256.Sum256([]byte(nn.String())))
	return strings.ToLower(gvk.Kind) + "-" + suffix
}
