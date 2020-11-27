// Copyright (c) 2020 Red Hat, Inc.
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

package containerjfr

import (
	"context"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	rhjmcv1beta1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1beta1"
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/common"
	resources "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/containerjfr/resource_definitions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileContainerJFR) setupTLS(ctx context.Context, cr *rhjmcv1beta1.ContainerJFR) (*resources.TLSConfig, error) {
	// Create self-signed issuer used to bootstrap CA
	selfSignedIssuer := resources.NewSelfSignedIssuer(cr)
	if err := controllerutil.SetControllerReference(cr, selfSignedIssuer, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: selfSignedIssuer.Name, Namespace: selfSignedIssuer.Namespace},
		&certv1.Issuer{}, selfSignedIssuer); err != nil {
		return nil, err
	}

	// Create CA certificate for Container JFR using the self-signed issuer
	caCert := resources.NewContainerJFRCACert(cr)
	if err := controllerutil.SetControllerReference(cr, caCert, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: caCert.Name, Namespace: caCert.Namespace},
		&certv1.Certificate{}, caCert); err != nil {
		return nil, err
	}

	// Create CA issuer using the CA cert just created
	caIssuer := resources.NewContainerJFRCAIssuer(cr)
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

	// Create a certificate for Container JFR signed by the CA just created
	cjfrCert := resources.NewContainerJFRCert(cr)
	if err := controllerutil.SetControllerReference(cr, cjfrCert, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: cjfrCert.Name, Namespace: cjfrCert.Namespace},
		&certv1.Certificate{}, cjfrCert); err != nil {
		return nil, err
	}

	// Create a certificate for Grafana signed by the Container JFR CA
	grafanaCert := resources.NewGrafanaCert(cr)
	if err := controllerutil.SetControllerReference(cr, grafanaCert, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: grafanaCert.Name, Namespace: grafanaCert.Namespace},
		&certv1.Certificate{}, grafanaCert); err != nil {
		return nil, err
	}

	// Update owner references of TLS secrets created by cert-manager to ensure proper cleanup
	err := r.setCertSecretOwner(context.Background(), cr, caCert, cjfrCert, grafanaCert)
	if err != nil {
		return nil, err
	}

	return &resources.TLSConfig{
		ContainerJFRSecret: cjfrCert.Spec.SecretName,
		GrafanaSecret:      grafanaCert.Spec.SecretName,
		KeystorePassSecret: cjfrCert.Spec.Keystores.PKCS12.PasswordSecretRef.Name,
	}, nil
}

func (r *ReconcileContainerJFR) setCertSecretOwner(ctx context.Context, cr *rhjmcv1beta1.ContainerJFR, certs ...*certv1.Certificate) error {
	// Make ContainerJFR CR controller of secrets created by cert-manager
	for _, cert := range certs {
		secret, err := r.GetCertificateSecret(ctx, cert.Name, cert.Namespace)
		if err != nil {
			if err == common.ErrCertNotReady {
				log.Info("Certificate not yet ready", "name", cert.Name, "namespace", cert.Namespace)
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
			log.Info("Set ContainerJFR CR as owner reference of secret", "name", secret.Name, "namespace", secret.Namespace)
		}
	}
	return nil
}
