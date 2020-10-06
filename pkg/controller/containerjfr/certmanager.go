package containerjfr

import (
	"context"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	resources "github.com/rh-jmc-team/container-jfr-operator/pkg/controller/containerjfr/resource_definitions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileContainerJFR) setupTLS(ctx context.Context, cr *rhjmcv1alpha1.ContainerJFR) (*resources.TLSConfig, error) {
	// Create self-signed issuer used to bootstrap CA
	selfSignedIssuer := resources.NewSelfSignedIssuer(cr)
	if err := controllerutil.SetControllerReference(cr, selfSignedIssuer, r.scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: selfSignedIssuer.Name, Namespace: selfSignedIssuer.Namespace},
		&certv1.Issuer{}, selfSignedIssuer); err != nil {
		return nil, err
	}

	// Create CA certificate for Container JFR using the self-signed issuer
	caCert := resources.NewContainerJFRCACert(cr)
	if err := controllerutil.SetControllerReference(cr, caCert, r.scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: caCert.Name, Namespace: caCert.Namespace},
		&certv1.Certificate{}, caCert); err != nil {
		return nil, err
	}

	// Create CA issuer using the CA cert just created
	caIssuer := resources.NewContainerJFRCAIssuer(cr)
	if err := controllerutil.SetControllerReference(cr, caIssuer, r.scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: caIssuer.Name, Namespace: caIssuer.Namespace},
		&certv1.Issuer{}, caIssuer); err != nil {
		return nil, err
	}

	// Create secret to hold keystore password
	keystorePassSecret := resources.NewKeystoreSecretForCR(cr)
	if err := controllerutil.SetControllerReference(cr, keystorePassSecret, r.scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: keystorePassSecret.Name, Namespace: keystorePassSecret.Namespace},
		&corev1.Secret{}, keystorePassSecret); err != nil {
		return nil, err
	}

	// Create a certificate for Container JFR signed by the CA just created
	cert := resources.NewContainerJFRCert(cr)
	if err := controllerutil.SetControllerReference(cr, cert, r.scheme); err != nil {
		return nil, err
	}
	if err := r.createObjectIfNotExists(ctx, types.NamespacedName{Name: cert.Name, Namespace: cert.Namespace},
		&certv1.Certificate{}, cert); err != nil {
		return nil, err
	}

	return &resources.TLSConfig{
		CertSecretName:         cert.Spec.SecretName,
		KeystorePassSecretName: cert.Spec.Keystores.PKCS12.PasswordSecretRef.Name,
	}, nil
}

func (r *ReconcileContainerJFR) setCertSecretOwner(ctx context.Context, cr *rhjmcv1alpha1.ContainerJFR) error {
	// Make ContainerJFR CR controller of secrets created by cert-manager
	secretNames := []string{cr.Name + "-tls", cr.Name + "-ca"}
	for _, name := range secretNames {
		secret := &corev1.Secret{}
		err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: cr.Namespace}, secret)
		if err != nil {
			return err
		}
		err = controllerutil.SetControllerReference(cr, secret, r.scheme)
		if err != nil {
			return err
		}
		err = r.client.Update(ctx, secret)
		if err != nil {
			return err
		}
		log.Info("Set ContainerJFR CR as owner reference of secret", "name", secret.Name, "namespace", secret.Namespace)
	}
	return nil
}

func (r *ReconcileContainerJFR) certManagerAvailable() bool {
	// TODO Implement check
	return true
}
