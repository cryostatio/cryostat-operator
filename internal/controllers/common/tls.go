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
	"context"
	"errors"
	"strings"

	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerTLS contains methods a reconciler may wish to use when configuring
// TLS-related functionality
type ReconcilerTLS interface {
	IsCertManagerEnabled(cr *model.CryostatInstance) bool
	GetCertificateSecret(ctx context.Context, cert *certv1.Certificate) (*corev1.Secret, error)
	OSUtils
}

// ReconcilerTLSConfig contains parameters used to create a ReconcilerTLS
type ReconcilerTLSConfig struct {
	// This client, initialized using mgr.Client(), is a split client
	// that reads objects from the cache and writes to the apiserver
	Client client.Client
	// Optional field to override the default behaviour when interacting
	// with the operating system
	OSUtils
}

type reconcilerTLS struct {
	*ReconcilerTLSConfig
}

// blank assignment to verify that tlsReconciler implements ReconcilerTLS
var _ ReconcilerTLS = &reconcilerTLS{}

// Environment variable to disable TLS for services
const disableServiceTLS = "DISABLE_SERVICE_TLS"

// NewReconcilerTLS creates a new ReconcilerTLS using the provided configuration
func NewReconcilerTLS(config *ReconcilerTLSConfig) ReconcilerTLS {
	configCopy := *config
	if config.OSUtils == nil {
		configCopy.OSUtils = &defaultOSUtils{}
	}
	return &reconcilerTLS{
		ReconcilerTLSConfig: &configCopy,
	}
}

// IsCertManagerEnabled returns whether TLS using cert-manager is enabled
// for this operator
func (r *reconcilerTLS) IsCertManagerEnabled(cr *model.CryostatInstance) bool {
	// First check if cert-manager is explicitly enabled or disabled in CR
	if cr.Spec.EnableCertManager != nil {
		return *cr.Spec.EnableCertManager
	}

	// Otherwise, fall back to DISABLE_SERVICE_TLS environment variable
	return strings.ToLower(r.GetEnv(disableServiceTLS)) != "true"
}

// ErrCertNotReady is returned when cert-manager has not marked the certificate
// as ready, and no TLS secret has been populated yet.
var ErrCertNotReady error = errors.New("Certificate secret not yet ready")

// GetCertificateSecret returns the Secret corresponding to the named
// cert-manager Certificate. This can return ErrCertNotReady if the
// certificate secret is not available yet.
func (r *reconcilerTLS) GetCertificateSecret(ctx context.Context, cert *certv1.Certificate) (*corev1.Secret, error) {
	if !isCertificateReady(cert) {
		return nil, ErrCertNotReady
	}

	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cert.Spec.SecretName, Namespace: cert.Namespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func isCertificateReady(cert *certv1.Certificate) bool {
	// Check if the certificate has a condition where Ready == True
	for _, condition := range cert.Status.Conditions {
		if condition.Type == certv1.CertificateConditionReady && condition.Status == certMeta.ConditionTrue {
			return true
		}
	}
	return false
}
