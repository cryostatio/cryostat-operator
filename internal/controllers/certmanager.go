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

package controllers

import (
	"context"
	"errors"
	"fmt"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const eventCertManagerUnavailableType = reasonCertManagerUnavailable

var errCertManagerMissing = errors.New("cert-manager integration is enabled, but cert-manager is unavailable")

const eventCertManagerUnavailableMsg = "cert-manager is not detected in the cluster, please install cert-manager or disable it by setting " +
	"\"enableCertManager\" in this Cryostat custom resource to false."

func (r *Reconciler) setupTLS(ctx context.Context, cr *model.CryostatInstance) (*resources.TLSConfig, error) {
	// If cert-manager is not available, emit an Event to inform the user
	available, err := r.certManagerAvailable()
	if err != nil {
		return nil, err
	}
	if !available {
		r.EventRecorder.Event(cr.Object, corev1.EventTypeWarning, eventCertManagerUnavailableType, eventCertManagerUnavailableMsg)
		return nil, errCertManagerMissing
	}

	// Create self-signed issuer used to bootstrap CA
	err = r.createOrUpdateIssuer(ctx, resources.NewSelfSignedIssuer(cr), cr.Object)
	if err != nil {
		return nil, err
	}

	// Create CA certificate for Cryostat using the self-signed issuer
	caCert := resources.NewCryostatCACert(cr)
	err = r.createOrUpdateCertificate(ctx, caCert, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create CA issuer using the CA cert just created
	err = r.createOrUpdateIssuer(ctx, resources.NewCryostatCAIssuer(cr), cr.Object)
	if err != nil {
		return nil, err
	}

	// Create secret to hold keystore password
	keystoreSecret := newKeystoreSecret(cr)
	err = r.createOrUpdateKeystoreSecret(ctx, keystoreSecret, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create a certificate for Cryostat signed by the CA just created
	cryostatCert := resources.NewCryostatCert(cr, keystoreSecret.Name)
	err = r.createOrUpdateCertificate(ctx, cryostatCert, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create a certificate for the reports generator signed by the Cryostat CA
	reportsCert := resources.NewReportsCert(cr)
	err = r.createOrUpdateCertificate(ctx, reportsCert, cr.Object)
	if err != nil {
		return nil, err
	}
	tlsConfig := &resources.TLSConfig{
		CryostatSecret:     cryostatCert.Spec.SecretName,
		ReportsSecret:      reportsCert.Spec.SecretName,
		KeystorePassSecret: cryostatCert.Spec.Keystores.PKCS12.PasswordSecretRef.Name,
	}
	certificates := []*certv1.Certificate{caCert, cryostatCert, reportsCert}
	// Create a certificate for Grafana signed by the Cryostat CA
	if !cr.Spec.Minimal {
		grafanaCert := resources.NewGrafanaCert(cr)
		err = r.createOrUpdateCertificate(ctx, grafanaCert, cr.Object)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, grafanaCert)
		tlsConfig.GrafanaSecret = grafanaCert.Spec.SecretName
	} else {
		grafanaCert := resources.NewGrafanaCert(cr)
		secret := secretForCertificate(grafanaCert)
		err = r.deleteSecret(ctx, secret)
		if err != nil {
			return nil, err
		}
		err = r.deleteCert(ctx, grafanaCert)
		if err != nil {
			return nil, err
		}
	}

	// Update owner references of TLS secrets created by cert-manager to ensure proper cleanup
	err = r.setCertSecretOwner(ctx, cr.Object, certificates...)
	if err != nil {
		return nil, err
	}

	secret, err := r.GetCertificateSecret(ctx, caCert)
	if err != nil {
		return nil, err
	}
	// Copy Cryostat CA secret in each target namespace
	for _, ns := range cr.TargetNamespaces {
		if ns != cr.InstallNamespace {
			namespaceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caCert.Spec.SecretName,
					Namespace: ns,
				},
				Type: secret.Type,
			}
			err = r.createOrUpdateSecret(ctx, namespaceSecret, cr.Object, func() error {
				namespaceSecret.Data = secret.Data
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	// Delete any Cryostat CA secrets in target namespaces that are no longer requested
	for _, ns := range toDelete(cr) {
		if ns != cr.InstallNamespace {
			namespaceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caCert.Spec.SecretName,
					Namespace: ns,
				},
			}
			err = r.deleteSecret(ctx, namespaceSecret)
			if err != nil {
				return nil, err
			}
		}
	}

	// Get the Cryostat CA certificate bytes from certificate secret
	caBytes, err := r.getCertficateBytes(ctx, caCert)
	if err != nil {
		return nil, err
	}
	tlsConfig.CACert = caBytes
	return tlsConfig, nil
}

func (r *Reconciler) finalizeTLS(ctx context.Context, cr *model.CryostatInstance) error {
	caCert := resources.NewCryostatCACert(cr)
	for _, ns := range cr.TargetNamespaces {
		if ns != cr.InstallNamespace {
			namespaceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caCert.Spec.SecretName,
					Namespace: ns,
				},
			}
			err := r.deleteSecret(ctx, namespaceSecret)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) setCertSecretOwner(ctx context.Context, owner metav1.Object, certs ...*certv1.Certificate) error {
	// Make Cryostat CR controller of secrets created by cert-manager
	for _, cert := range certs {
		secret, err := r.GetCertificateSecret(ctx, cert)
		if err != nil {
			if err == common.ErrCertNotReady {
				r.Log.Info("Certificate not yet ready", "name", cert.Name, "namespace", cert.Namespace)
			}
			return err
		}
		if !metav1.IsControlledBy(secret, owner) {
			err = controllerutil.SetControllerReference(owner, secret, r.Scheme)
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

func secretForCertificate(cert *certv1.Certificate) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cert.Spec.SecretName,
			Namespace: cert.Namespace,
		},
	}
}

func (r *Reconciler) certManagerAvailable() (bool, error) {
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

func (r *Reconciler) createOrUpdateIssuer(ctx context.Context, issuer *certv1.Issuer, owner metav1.Object) error {
	issuerSpec := issuer.Spec.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, issuer, func() error {
		if err := controllerutil.SetControllerReference(owner, issuer, r.Scheme); err != nil {
			return err
		}
		// Update Issuer spec
		issuer.Spec = *issuerSpec
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Issuer %s", op), "name", issuer.Name, "namespace", issuer.Namespace)
	return nil
}

func (r *Reconciler) createOrUpdateCertificate(ctx context.Context, cert *certv1.Certificate, owner metav1.Object) error {
	certSpec := cert.Spec.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cert, func() error {
		if err := controllerutil.SetControllerReference(owner, cert, r.Scheme); err != nil {
			return err
		}
		// Update Certificate spec
		cert.Spec = *certSpec
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Certificate %s", op), "name", cert.Name, "namespace", cert.Namespace)
	return nil
}

func newKeystoreSecret(cr *model.CryostatInstance) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-keystore",
			Namespace: cr.InstallNamespace,
		},
	}
}

func (r *Reconciler) createOrUpdateKeystoreSecret(ctx context.Context, secret *corev1.Secret, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
			return err
		}

		// Don't modify secret data, since the password is psuedorandomly generated
		if secret.CreationTimestamp.IsZero() {
			secret.StringData = map[string]string{
				"KEYSTORE_PASS": r.GenPasswd(20),
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Secret %s", op), "name", secret.Name, "namespace", secret.Namespace)
	return nil
}

func (r *Reconciler) deleteCert(ctx context.Context, cert *certv1.Certificate) error {
	err := r.Client.Delete(ctx, cert)
	if err != nil && !kerrors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete certificate", "name", cert.Name, "namespace", cert.Namespace)
		return err
	}
	r.Log.Info("Cert deleted", "name", cert.Name, "namespace", cert.Namespace)
	return nil
}

func (r *Reconciler) getCertficateBytes(ctx context.Context, cert *certv1.Certificate) ([]byte, error) {
	secret, err := r.GetCertificateSecret(ctx, cert)
	if err != nil {
		return nil, err
	}
	return secret.Data[corev1.TLSCertKey], nil
}
