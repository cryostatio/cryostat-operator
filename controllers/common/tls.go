// Copyright (c) 2021 Red Hat, Inc.
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

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerTLS contains methods a reconciler may wish to use when configuring
// TLS-related functionality
type ReconcilerTLS interface {
	IsCertManagerEnabled() bool
	GetContainerJFRCABytes(ctx context.Context, cjfr *rhjmcv1beta1.ContainerJFR) ([]byte, error)
	GetCertificateSecret(ctx context.Context, name string, namespace string) (*corev1.Secret, error)
}

// ReconcilerTLSConfig contains parameters used to create a ReconcilerTLS
type ReconcilerTLSConfig struct {
	// This client, initialized using mgr.Client(), is a split client
	// that reads objects from the cache and writes to the apiserver
	Client client.Client
	// Optional field to override the default behaviour when interacting
	// with the operating system
	OS OSUtils
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
	if config.OS == nil {
		configCopy.OS = &defaultOSUtils{}
	}
	return &reconcilerTLS{
		ReconcilerTLSConfig: &configCopy,
	}
}

// IsCertManagerEnabled returns whether TLS using cert-manager is enabled
// for this operator
func (r *reconcilerTLS) IsCertManagerEnabled() bool {
	// FIXME hardcoded off - new operator-sdk has cert-manager already set up
	// somehow, with "CA injection"?

	// Check if the user has explicitly requested cert-manager to be disabled
	return strings.ToLower(r.OS.GetEnv(disableServiceTLS)) != "true"
}

// ErrCertNotReady is returned when cert-manager has not marked the certificate
// as ready, and no TLS secret has been populated yet.
var ErrCertNotReady error = errors.New("Certificate secret not yet ready")

// GetCertificateSecret returns the Secret corresponding to the named
// cert-manager Certificate. This can return ErrCertNotReady if the
// certificate secret is not available yet.
func (r *reconcilerTLS) GetCertificateSecret(ctx context.Context, name string, namespace string) (*corev1.Secret, error) {
	// Look up named certificate
	cert := &certv1.Certificate{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cert)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, ErrCertNotReady
		}
		return nil, err
	}

	if !isCertificateReady(cert) {
		return nil, ErrCertNotReady
	}

	secret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: cert.Spec.SecretName, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// GetContainerJFRCABytes returns the CA certificate created for the provided
// ContainerJFR CR, as a byte slice.
func (r *reconcilerTLS) GetContainerJFRCABytes(ctx context.Context, cjfr *rhjmcv1beta1.ContainerJFR) ([]byte, error) {
	caName := cjfr.Name + "-ca"
	secret, err := r.GetCertificateSecret(ctx, caName, cjfr.Namespace)
	if err != nil {
		return nil, err
	}
	return secret.Data[corev1.TLSCertKey], nil
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
