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

package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryostatio/cryostat-operator/internal/controller/constants"
	"github.com/cryostatio/cryostat-operator/internal/controller/model"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileSecrets(ctx context.Context, cr *model.CryostatInstance) error {
	if err := r.reconcileAuthProxyCookieSecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileDatabaseConnectionSecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileAgentGatewaySecret(ctx, cr); err != nil {
		return err
	}
	if err := r.reconcileUserProxySecret(ctx, cr); err != nil {
		return err
	}
	return r.reconcileStorageSecret(ctx, cr)
}

// reconcileAgentGatewaySecret generates the shared secret that the agent gateway stamps onto
// every request it forwards, and that the Cryostat core container compares against.
func (r *Reconciler) reconcileAgentGatewaySecret(ctx context.Context, cr *model.CryostatInstance) error {
	return r.reconcileProvenanceSecret(ctx, cr, constants.AgentGatewaySecretNameSuffix,
		constants.AgentGatewaySecretKey, constants.AgentGatewayConfFileName,
		constants.AgentGatewayAuthHeader)
}

// reconcileUserProxySecret generates the shared secret that the auth-strip proxy stamps onto
// every request it forwards. It is deliberately distinct from the agent gateway's secret: a
// single shared value would prove only that "some Operator-rendered proxy forwarded this",
// which is precisely the distinction that matters.
func (r *Reconciler) reconcileUserProxySecret(ctx context.Context, cr *model.CryostatInstance) error {
	return r.reconcileProvenanceSecret(ctx, cr, constants.UserProxySecretNameSuffix,
		constants.UserProxySecretKey, constants.UserProxyConfFileName,
		constants.UserProxyAuthHeader)
}

// reconcileProvenanceSecret generates a Secret holding a proxy hop's provenance stamp. The
// Secret carries two keys derived from one value: the raw secret, passed to the Cryostat core
// container by secretKeyRef, and a one-line nginx include that stamps it as a header. Two keys
// rather than one because nginx cannot interpolate a file or environment variable into a
// directive value.
func (r *Reconciler) reconcileProvenanceSecret(ctx context.Context, cr *model.CryostatInstance,
	nameSuffix string, secretKey string, confFileName string, header string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + nameSuffix,
			Namespace: cr.InstallNamespace,
		},
	}

	return r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}

		// Reuse the existing value across reconciles; rotating it would break every
		// in-flight request until both the proxy and core containers restarted together.
		// Read it back from Data, which the client populates, rather than the write-only
		// StringData.
		value := string(secret.Data[secretKey])
		if len(value) == 0 {
			value = r.GenPasswd(32)
			secret.StringData[secretKey] = value
		}

		// Re-render the nginx include unconditionally: it is derived, and an upgrade from
		// a version that did not write this key must backfill it.
		//
		// %q escapes for Go, not for nginx. It is safe only because GenPasswd's alphabet
		// cannot produce "$", '"', or "\", each of which nginx treats specially inside a
		// double-quoted directive value. See common.DefaultOSUtils.GenPasswd.
		secret.StringData[confFileName] = fmt.Sprintf("proxy_set_header %s %q;\n", header, value)
		return nil
	})
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

const (
	// The suffix to be appended to the name of a Cryostat CR to name its database secret
	databaseSecretNameSuffix          = "-db"
	eventDatabaseSecretMismatchedType = "DatabaseSecretMismatched"
	eventDatabaseMismatchMsg          = "\"databaseOptions.secretName\" field cannot be updated, please revert its value or re-create this Cryostat custom resource"
)

var errDatabaseSecretUpdated = errors.New("database secret cannot be updated, but another secret is specified")

func (r *Reconciler) reconcileDatabaseConnectionSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + databaseSecretNameSuffix,
			Namespace: cr.InstallNamespace,
		},
	}
	secretName := secret.Name
	secretProvided := cr.Spec.DatabaseOptions != nil && cr.Spec.DatabaseOptions.SecretName != nil
	if secretProvided {
		secretName = *cr.Spec.DatabaseOptions.SecretName
	}

	// If the CR status contains the secret name, emit an Event in case the configured secret's name does not match.
	if len(cr.Status.DatabaseSecret) > 0 && cr.Status.DatabaseSecret != secretName {
		r.EventRecorder.Event(cr.Object, corev1.EventTypeWarning, eventDatabaseSecretMismatchedType, eventDatabaseMismatchMsg)
		return errDatabaseSecretUpdated
	}

	if !secretProvided {
		err := r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
			if secret.StringData == nil {
				secret.StringData = map[string]string{}
			}

			// Password is generated, so don't regenerate it when updating
			if secret.CreationTimestamp.IsZero() {
				secret.StringData[constants.DatabaseSecretConnectionKey] = r.GenPasswd(32)
				secret.StringData[constants.DatabaseSecretEncryptionKey] = r.GenPasswd(32)
			}

			secret.Immutable = &[]bool{true}[0]
			return nil
		})

		if err != nil {
			return err
		}
	}

	cr.Status.DatabaseSecret = secretName
	return r.Status().Update(ctx, cr.Object)
}

// storageSecretNameSuffix is the suffix to be appended to the name of a
// Cryostat CR to name its object storage secret
const storageSecretNameSuffix = "-storage"

// storageSecretAccessKey indexes the username within the Cryostat storage Secret
const storageSecretAccessKey = "ACCESS_KEY"

// storageSecretUserKey indexes the password within the Cryostat storage Secret
const storageSecretPassKey = "SECRET_KEY"

func (r *Reconciler) reconcileStorageSecret(ctx context.Context, cr *model.CryostatInstance) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + storageSecretNameSuffix,
			Namespace: cr.InstallNamespace,
		},
	}

	err := r.createOrUpdateSecret(ctx, secret, cr.Object, func() error {
		if secret.StringData == nil {
			secret.StringData = map[string]string{}
		}

		// Password is generated, so don't regenerate it when updating
		r.setDataIfNotPresent(secret, storageSecretAccessKey, func() string {
			return constants.DefaultStorageAccessKey
		})
		r.setDataIfNotPresent(secret, storageSecretPassKey, func() string {
			return r.GenPasswd(32)
		})
		return nil
	})

	if err != nil {
		return err
	}

	cr.Status.StorageSecret = secret.Name
	return r.Status().Update(ctx, cr.Object)
}

func (r *Reconciler) setDataIfNotPresent(secret *corev1.Secret, key string, valueFunc func() string) {
	if _, pres := secret.Data[key]; !pres {
		secret.StringData[key] = valueFunc()
	}
}

func (r *Reconciler) createOrUpdateSecret(ctx context.Context, secret *corev1.Secret, owner metav1.Object,
	delegate controllerutil.MutateFn) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if owner != nil {
			// Set the Cryostat CR as controller
			if err := controllerutil.SetControllerReference(owner, secret, r.Scheme); err != nil {
				return err
			}
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
	err := r.Delete(ctx, secret)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		r.Log.Error(err, "Could not delete secret", "name", secret.Name, "namespace", secret.Namespace)
		return err
	}
	r.Log.Info("Secret deleted", "name", secret.Name, "namespace", secret.Namespace)
	return nil
}
