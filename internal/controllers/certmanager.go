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

package controllers

import (
	"context"
	"errors"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const eventCertManagerUnavailable = "CertManagerUnavailable"

func (r *CryostatReconciler) setupTLS(ctx context.Context, cr *operatorv1beta1.Cryostat) (*resources.TLSConfig, error) {
	// If cert-manager is not available, emit an Event to inform the user
	available, err := r.certManagerAvailable()
	if err != nil {
		return nil, err
	}
	if !available {
		r.EventRecorder.Event(cr, corev1.EventTypeWarning, eventCertManagerUnavailable,
			"cert-manager is not detected in the cluster, please install cert-manager or disable it by setting "+
				"\"enableCertManager\" in this Cryostat custom resource to false")
		return nil, errors.New("cert-manager integration is enabled, but cert-manager is unavailable")
	}

	// Create self-signed issuer used to bootstrap CA
	selfSignedIssuer := resources.NewSelfSignedIssuer(cr)
	if err := controllerutil.SetControllerReference(cr, selfSignedIssuer, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: selfSignedIssuer.Name, Namespace: selfSignedIssuer.Namespace},
		&certv1.Issuer{}, selfSignedIssuer); err != nil {
		return nil, err
	}

	// Create CA certificate for Cryostat using the self-signed issuer
	caCert := resources.NewCryostatCACert(cr)
	if err := controllerutil.SetControllerReference(cr, caCert, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: caCert.Name, Namespace: caCert.Namespace},
		&certv1.Certificate{}, caCert); err != nil {
		return nil, err
	}

	// Create CA issuer using the CA cert just created
	caIssuer := resources.NewCryostatCAIssuer(cr)
	if err := controllerutil.SetControllerReference(cr, caIssuer, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: caIssuer.Name, Namespace: caIssuer.Namespace},
		&certv1.Issuer{}, caIssuer); err != nil {
		return nil, err
	}

	// Create secret to hold keystore password
	keystorePassSecret := resources.NewKeystoreSecretForCR(cr)
	if err := controllerutil.SetControllerReference(cr, keystorePassSecret, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: keystorePassSecret.Name, Namespace: keystorePassSecret.Namespace},
		&corev1.Secret{}, keystorePassSecret); err != nil {
		return nil, err
	}

	// Create a certificate for Cryostat signed by the CA just created
	cryostatCert := resources.NewCryostatCert(cr)
	if err := controllerutil.SetControllerReference(cr, cryostatCert, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: cryostatCert.Name, Namespace: cryostatCert.Namespace},
		&certv1.Certificate{}, cryostatCert); err != nil {
		return nil, err
	}

	// Create a certificate for Grafana signed by the Cryostat CA
	grafanaCert := resources.NewGrafanaCert(cr)
	if err := controllerutil.SetControllerReference(cr, grafanaCert, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: grafanaCert.Name, Namespace: grafanaCert.Namespace},
		&certv1.Certificate{}, grafanaCert); err != nil {
		return nil, err
	}

	// Update owner references of TLS secrets created by cert-manager to ensure proper cleanup
	err = r.setCertSecretOwner(context.Background(), cr, caCert, cryostatCert, grafanaCert)
	if err != nil {
		return nil, err
	}

	return &resources.TLSConfig{
		CryostatSecret:     cryostatCert.Spec.SecretName,
		GrafanaSecret:      grafanaCert.Spec.SecretName,
		KeystorePassSecret: cryostatCert.Spec.Keystores.PKCS12.PasswordSecretRef.Name,
	}, nil
}

func (r *CryostatReconciler) setCertSecretOwner(ctx context.Context, cr *operatorv1beta1.Cryostat, certs ...*certv1.Certificate) error {
	// Make Cryostat CR controller of secrets created by cert-manager
	for _, cert := range certs {
		secret, err := r.GetCertificateSecret(ctx, cert.Name, cert.Namespace)
		if err != nil {
			if err == common.ErrCertNotReady {
				r.Log.Info("Certificate not yet ready", "name", cert.Name, "namespace", cert.Namespace)
			}
			return err
		}
		if !metav1.IsControlledBy(secret, cr) {
			err = controllerutil.SetControllerReference(cr, secret, r.Scheme)
			if err != nil {
				return err
			}
			err = r.Client.Update(ctx, secret)
			if err != nil {
				return err
			}
			r.Log.Info("Set Cryostat CR as owner reference of secret", "name", secret.Name, "namespace", secret.Namespace)
		}
	}
	return nil
}

func (r *CryostatReconciler) certManagerAvailable() (bool, error) {
	// Check if cert-manager API is available. Checking just one should be enough.
	_, err := r.RESTMapper.RESTMapping(schema.GroupKind{
		Group: certv1.SchemeGroupVersion.Group,
		Kind:  certv1.IssuerKind,
	}, certv1.SchemeGroupVersion.Version)
	if err != nil {
		// No matches for Issuer GVK
		if meta.IsNoMatchError(err) {
			return false, nil
		}
		// Unexpected error occurred
		return false, err
	}
	return true, nil
}
