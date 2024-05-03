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
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileSecrets(ctx context.Context, cr *model.CryostatInstance) error {
	if err := r.reconcileAuthProxyCookieSecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileGrafanaSecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileDatabaseConnectionSecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileStorageSecret(ctx, cr); err != nil {
		return err
	}
	return r.reconcileJMXSecret(ctx, cr)
}

func (r *Reconciler) reconcileAuthProxyCookieSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-oauth2-cookie",
			Namespace: cr.InstallNamespace,
		},
	}

	return r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}

		// secret is generated, so don't regenerate it when updating
		if secret.CreationTimestamp.IsZero() {
			secret.StringData["OAUTH2_PROXY_COOKIE_SECRET"] = r.GenPasswd(32)
		}
		return nil
	})
}

func (r *Reconciler) reconcileGrafanaSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana-basic",
			Namespace: cr.InstallNamespace,
		},
	}

	err := r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}
		secret.StringData["GF_SECURITY_ADMIN_USER"] = "admin"

		// Password is generated, so don't regenerate it when updating
		if secret.CreationTimestamp.IsZero() {
			secret.StringData["GF_SECURITY_ADMIN_PASSWORD"] = r.GenPasswd(20)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Set the Grafana secret in the CR status
	cr.Status.GrafanaSecret = secret.Name
	return r.Client.Status().Update(ctx, cr.Object)
}

// jmxSecretNameSuffix is the suffix to be appended to the name of a
// Cryostat CR to name its JMX credentials secret
const jmxSecretNameSuffix = "-jmx-auth"

// jmxSecretUserKey indexes the username within the Cryostat JMX auth secret
const jmxSecretUserKey = "CRYOSTAT_RJMX_USER"

// jmxSecretPassKey indexes the password within the Cryostat JMX auth secret
const jmxSecretPassKey = "CRYOSTAT_RJMX_PASS"

func (r *Reconciler) reconcileJMXSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + jmxSecretNameSuffix,
			Namespace: cr.InstallNamespace,
		},
	}

	return r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}
		secret.StringData[jmxSecretUserKey] = "cryostat"

		// Password is generated, so don't regenerate it when updating
		if secret.CreationTimestamp.IsZero() {
			secret.StringData[jmxSecretPassKey] = r.GenPasswd(20)
		}
		return nil
	})
}

// databaseSecretNameSuffix is the suffix to be appended to the name of a
// Cryostat CR to name its credentials database secret
const databaseSecretNameSuffix = "-db"

// databaseSecretConnectionPassKey indexes the database connection password within the Cryostat database Secret
const databaseSecretConnectionPassKey = "CONNECTION_KEY"

// databaseSecretEncryptionKey indexes the database encryption key within the Cryostat database Secret
const databaseSecretEncryptionKey = "ENCRYPTION_KEY"

func (r *Reconciler) reconcileDatabaseConnectionSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + databaseSecretNameSuffix,
			Namespace: cr.InstallNamespace,
		},
	}

	secretProvided := cr.Spec.JmxCredentialsDatabaseOptions != nil && cr.Spec.JmxCredentialsDatabaseOptions.DatabaseSecretName != nil
	if secretProvided {
		return nil // Do not delete default secret to allow reverting to use default if needed
	}

	return r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}

		// Password is generated, so don't regenerate it when updating
		if secret.CreationTimestamp.IsZero() {
			secret.StringData[databaseSecretConnectionPassKey] = r.GenPasswd(32)
			secret.StringData[databaseSecretEncryptionKey] = r.GenPasswd(32)
		}
		return nil
	})
}

// storageSecretNameSuffix is the suffix to be appended to the name of a
// Cryostat CR to name its object storage secret
const storageSecretNameSuffix = "-storage-secret-key"

// storageSecretUserKey indexes the password within the Cryostat storage Secret
const storageSecretPassKey = "SECRET_KEY"

func (r *Reconciler) reconcileStorageSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + storageSecretNameSuffix,
			Namespace: cr.InstallNamespace,
		},
	}

	// secretProvided := cr.Spec.JmxCredentialsDatabaseOptions != nil && cr.Spec.JmxCredentialsDatabaseOptions.DatabaseSecretName != nil
	// if secretProvided {
	//      return nil // Do not delete default secret to allow reverting to use default if needed
	// }

	return r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}

		// Password is generated, so don't regenerate it when updating
		if secret.CreationTimestamp.IsZero() {
			secret.StringData[storageSecretPassKey] = r.GenPasswd(32)
		}
		return nil
	})
}

func (r *Reconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret, owner metav1.Object,
	delegate controllerutil.MutateFn) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set the Cryostat CR as controller
		if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
			return err
		}
		// Call the delegate for secret-specific mutations
		return delegate()
	})
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("Secret %s", op), "name", secret.Name, "namespace", secret.Namespace)
	return nil
}

func (r *Reconciler) deleteSecret(ctx context.Context, secret *corev1.Secret) error {
	err := r.Client.Delete(ctx, secret)
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, "Could not delete secret", "name", secret.Name, "namespace", secret.Namespace)
		return err
	}
	r.Log.Info("Secret deleted", "name", secret.Name, "namespace", secret.Namespace)
	return nil
}
