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
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileSecrets(ctx context.Context, cr *model.CryostatInstance) error {
	if err := r.reconcileGrafanaSecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileDatabaseSecret(ctx, cr); err != nil {
		return err
	}
	return r.reconcileJMXSecret(ctx, cr)
}

func (r *Reconciler) reconcileGrafanaSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana-basic",
			Namespace: cr.InstallNamespace,
		},
	}

	var secretName string
	if cr.Spec.Minimal {
		err := r.deleteSecret(ctx, secret)
		if err != nil {
			return err
		}
		secretName = ""
	} else {
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
		secretName = secret.Name
	}

	// Set the Grafana secret in the CR status
	cr.Status.GrafanaSecret = secretName
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
const databaseSecretNameSuffix = "-jmx-credentials-db"

// dbSecretUserKey indexes the password within the Cryostat credentials database Secret
const databaseSecretPassKey = "CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD"

func (r *Reconciler) reconcileDatabaseSecret(ctx context.Context, cr *model.CryostatInstance) error {
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
			secret.StringData[databaseSecretPassKey] = r.GenPasswd(32)
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
