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
	"strings"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	resources "github.com/cryostatio/cryostat-operator/internal/controllers/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	caCert := resources.NewCryostatCACert(r.gvk, cr)
	err = r.createOrUpdateCertificate(ctx, caCert, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create CA issuer using the CA cert just created
	err = r.createOrUpdateIssuer(ctx, resources.NewCryostatCAIssuer(r.gvk, cr), cr.Object)
	if err != nil {
		return nil, err
	}

	// Create secret to hold keystore password
	coreKeystoreSecret := newCoreKeystoreSecret(cr)
	err = r.createOrUpdateKeystoreSecret(ctx, coreKeystoreSecret, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create a certificate for Cryostat signed by the CA just created
	cryostatCert := resources.NewCryostatCert(cr, coreKeystoreSecret.Name)
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

	// Create a certificate for the Cryostat database signed by the Cryostat CA
	databaseCert := resources.NewDatabaseCert(cr)
	err = r.createOrUpdateCertificate(ctx, databaseCert, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create secret to hold keystore password
	storageKeystoreSecretComponent := "storage"
	storageKeystoreSecret := newKeystoreSecret(cr, &storageKeystoreSecretComponent)
	err = r.createOrUpdateKeystoreSecret(ctx, storageKeystoreSecret, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create a certificate for Cryostat storage signed by the Cryostat CA
	storageCert := resources.NewStorageCert(cr, storageKeystoreSecret.Name)
	err = r.createOrUpdateCertificate(ctx, storageCert, cr.Object)
	if err != nil {
		return nil, err
	}

	// Create a certificate for the agent proxy signed by the Cryostat CA
	agentProxyCert := resources.NewAgentProxyCert(cr)
	err = r.createOrUpdateCertificate(ctx, agentProxyCert, cr.Object)
	if err != nil {
		return nil, err
	}

	// List of certificates whose secrets should be owned by this CR
	certificates := []*certv1.Certificate{caCert, cryostatCert, reportsCert, agentProxyCert}

	// Get the Cryostat CA certificate bytes from certificate secret
	caBytes, err := r.getCertficateBytes(ctx, caCert)
	if err != nil {
		return nil, err
	}

	tlsConfig := &resources.TLSConfig{
		CryostatSecret:            cryostatCert.Spec.SecretName,
		DatabaseSecret:            databaseCert.Spec.SecretName,
		StorageSecret:             storageCert.Spec.SecretName,
		ReportsSecret:             reportsCert.Spec.SecretName,
		AgentProxySecret:          agentProxyCert.Spec.SecretName,
		KeystorePassSecret:        cryostatCert.Spec.Keystores.PKCS12.PasswordSecretRef.Name,
		StorageKeystorePassSecret: storageCert.Spec.Keystores.PKCS12.PasswordSecretRef.Name,
		StorageKeystorePassKey:    storageCert.Spec.Keystores.PKCS12.PasswordSecretRef.Key,
		CACert:                    caBytes,
	}

	agentCertsNotReady := []string{}
	for _, ns := range cr.TargetNamespaces {
		// Copy Cryostat CA secret in each target namespace
		if ns != cr.InstallNamespace {
			namespaceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      caCert.Spec.SecretName,
					Namespace: ns,
				},
				Type: corev1.SecretTypeOpaque,
			}
			err = r.createOrUpdateCertSecret(ctx, namespaceSecret, caBytes,
				common.LabelsForTargetNamespaceObject(cr))
			if err != nil {
				return nil, err
			}
		}

		// Create a certificate for Cryostat agents in each target namespace
		agentCert := resources.NewAgentCert(cr, ns, r.gvk)
		err := r.reconcileAgentCertificate(ctx, agentCert, cr, ns)
		if err != nil {
			if err == common.ErrCertNotReady {
				// Continue with other namespaces if the cert isn't ready
				agentCertsNotReady = append(agentCertsNotReady, agentCert.Name)
			} else {
				return nil, err
			}
		}
		certificates = append(certificates, agentCert)
	}

	if len(agentCertsNotReady) > 0 {
		// One or more agent certificates weren't ready, so log a message and return
		r.Log.Info("Not all agent certificates were ready", "not ready", strings.Join(agentCertsNotReady, ", "))
		return nil, common.ErrCertNotReady
	}

	// Update owner references of TLS secrets created by cert-manager to ensure proper cleanup
	err = r.setCertSecretOwner(ctx, cr.Object, certificates...)
	if err != nil {
		return nil, err
	}

	// Clean up resources from target namespaces that are no longer requested
	for _, ns := range toDelete(cr) {
		// Delete any Cryostat CA secret copies in removed namespaces
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

		// Delete any agent certificates removed target namespaces
		agentCert := resources.NewAgentCert(cr, ns, r.gvk)

		// Delete namespace copy
		if ns != cr.InstallNamespace {
			namespaceAgentSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentCert.Spec.SecretName,
					Namespace: ns,
				},
			}
			err = r.deleteSecret(ctx, namespaceAgentSecret)
			if err != nil {
				return nil, err
			}
		}

		// Delete certificate with original secret
		err := r.deleteCertWithSecret(ctx, agentCert)
		if err != nil {
			return nil, err
		}
	}

	return tlsConfig, nil
}

func (r *Reconciler) finalizeTLS(ctx context.Context, cr *model.CryostatInstance) error {
	caCert := resources.NewCryostatCACert(r.gvk, cr)
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

			// Delete any agent certificate secrets in target namespaces
			agentCert := resources.NewAgentCert(cr, ns, r.gvk)
			namespaceAgentSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentCert.Spec.SecretName,
					Namespace: ns,
				},
			}
			err = r.deleteSecret(ctx, namespaceAgentSecret)
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
		// Check if the issuer's CA has changed
		if !issuer.CreationTimestamp.IsZero() && r.issuerCAChanged(issuer.Spec.CA, issuerSpec.CA) {
			// Issuer CA has changed, delete all certificates the previous CA issued
			err := r.deleteCertChain(ctx, issuer.Namespace, issuerSpec.CA.SecretName, owner)
			if err != nil {
				return err
			}
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

func (r *Reconciler) issuerCAChanged(current *certv1.CAIssuer, updated *certv1.CAIssuer) bool {
	// Compare the .spec.ca.secretName in the current and updated Issuer. Return whether they differ.
	if current == nil {
		return false
	}
	currentSecret := current.SecretName
	updatedSecret := ""
	if updated != nil {
		updatedSecret = updated.SecretName
	}

	if currentSecret != updatedSecret {
		r.Log.Info("certificate authority has changed, deleting issued certificates",
			"current", currentSecret, "updated", updatedSecret)
		return true
	}
	return false
}

func (r *Reconciler) deleteCertChain(ctx context.Context, namespace string, caSecretName string, owner metav1.Object) error {
	// Look up all certificates in this namespace
	certs := &certv1.CertificateList{}
	err := r.Client.List(ctx, certs, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return err
	}

	for i, cert := range certs.Items {
		// Is the certificate owned by this CR, and not the CA itself?
		if metav1.IsControlledBy(&certs.Items[i], owner) && cert.Spec.SecretName != caSecretName {
			err := r.deleteCertWithSecret(ctx, &certs.Items[i])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Reconciler) deleteCertWithSecret(ctx context.Context, cert *certv1.Certificate) error {
	// Clean up secret referenced by the cert
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cert.Spec.SecretName,
			Namespace: cert.Namespace,
		},
	}
	err := r.deleteSecret(ctx, secret)
	if err != nil {
		return err
	}

	// Delete the certificate
	err = r.deleteCertificate(ctx, cert)
	if err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) reconcileAgentCertificate(ctx context.Context, cert *certv1.Certificate, cr *model.CryostatInstance, namespace string) error {
	// Create the Agent certificate in the install namespace
	err := r.createOrUpdateCertificate(ctx, cert, cr.Object)
	if err != nil {
		return err
	}

	// Fetch the certificate secret and create a copy in the target namespace (if not the install namespace)
	if namespace != cr.InstallNamespace {
		secret, err := r.GetCertificateSecret(ctx, cert)
		if err != nil {
			return err
		}

		targetSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret.Name,
				Namespace: namespace,
			},
		}
		err = r.createOrUpdateSecret(ctx, targetSecret, nil, func() error {
			common.MergeLabelsAndAnnotations(&targetSecret.ObjectMeta,
				common.LabelsForTargetNamespaceObject(cr), map[string]string{})
			targetSecret.Data = secret.Data
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

var errCertificateModified error = errors.New("certificate has been modified")

func (r *Reconciler) createOrUpdateCertificate(ctx context.Context, cert *certv1.Certificate, owner metav1.Object) error {
	certCopy := cert.DeepCopy()
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cert, func() error {
		if owner != nil {
			if err := controllerutil.SetControllerReference(owner, cert, r.Scheme); err != nil {
				return err
			}
		}

		if cert.CreationTimestamp.IsZero() {
			cert.Spec = certCopy.Spec
		} else if !cmp.Equal(cert.Spec, certCopy.Spec) {
			return errCertificateModified
		}

		return nil
	})
	if err != nil {
		if err == errCertificateModified {
			return r.recreateCertificate(ctx, certCopy, owner)
		}
		return err
	}
	r.Log.Info(fmt.Sprintf("Certificate %s", op), "name", cert.Name, "namespace", cert.Namespace)
	return nil
}

func (r *Reconciler) recreateCertificate(ctx context.Context, cert *certv1.Certificate, owner metav1.Object) error {
	err := r.deleteCertWithSecret(ctx, cert)
	if err != nil {
		return err
	}
	return r.createOrUpdateCertificate(ctx, cert, owner)
}

func newCoreKeystoreSecret(cr *model.CryostatInstance) *corev1.Secret {
	return newKeystoreSecret(cr, nil)
}

func newKeystoreSecret(cr *model.CryostatInstance, component *string) *corev1.Secret {
	tag := ""
	if component != nil {
		tag = fmt.Sprintf("-%s", *component)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + tag + "-keystore",
			Namespace: cr.InstallNamespace,
		},
	}
}

func (r *Reconciler) createOrUpdateKeystoreSecret(ctx context.Context, secret *corev1.Secret, owner metav1.Object) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
			return err
		}

		// Don't modify secret data, since the password is pseudorandomly generated
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

func (r *Reconciler) createOrUpdateCertSecret(ctx context.Context, secret *corev1.Secret, cert []byte,
	labels map[string]string) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		common.MergeLabelsAndAnnotations(&secret.ObjectMeta, labels, map[string]string{})
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[corev1.TLSCertKey] = cert
		return nil
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Secret %s", op), "name", secret.Name, "namespace", secret.Namespace)
	return nil
}

func (r *Reconciler) getCertficateBytes(ctx context.Context, cert *certv1.Certificate) ([]byte, error) {
	secret, err := r.GetCertificateSecret(ctx, cert)
	if err != nil {
		return nil, err
	}
	return secret.Data[corev1.TLSCertKey], nil
}

func (r *Reconciler) deleteCertificate(ctx context.Context, cert *certv1.Certificate) error {
	err := r.Client.Delete(ctx, cert)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		r.Log.Error(err, "Could not delete certificate", "name", cert.Name, "namespace", cert.Namespace)
		return err
	}
	r.Log.Info("deleted Certificate", "name", cert.Name, "namespace", cert.Namespace)
	return nil
}
