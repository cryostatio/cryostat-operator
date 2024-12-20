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
	"context"
	"errors"
	"strings"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerTLS contains methods a reconciler may wish to use when configuring
// TLS-related functionality
type ReconcilerTLS interface {
	IsCertManagerEnabled(cr *model.CryostatInstance) bool
	GetCertificateSecret(ctx context.Context, cert *certv1.Certificate) (*corev1.Secret, error)
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
		configCopy.OS = &DefaultOSUtils{}
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
	return strings.ToLower(r.OS.GetEnv(disableServiceTLS)) != "true"
}

// ErrCertNotReady is returned when cert-manager has not marked the certificate
// as ready, and no TLS secret has been populated yet.
var ErrCertNotReady error = errors.New("certificate secret not yet ready")

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
