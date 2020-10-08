package common

import (
	"context"
	"errors"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// TODO doc
type TLSReconciler interface {
	IsCertManagerEnabled() bool
	GetContainerJFRCABytes(ctx context.Context, cjfr *rhjmcv1alpha1.ContainerJFR) ([]byte, error)
	GetCertificateSecret(ctx context.Context, name string, namespace string) (*corev1.Secret, error)
}

func (r *commonReconciler) IsCertManagerEnabled() bool {
	return true // TODO
}

var ErrNotReady error = errors.New("Certificate secret not yet ready")

func (r *commonReconciler) GetCertificateSecret(ctx context.Context, name string, namespace string) (*corev1.Secret, error) {
	// Look up named certificate
	cert := &certv1.Certificate{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cert)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, ErrNotReady
		}
		return nil, err
	}

	if !isCertificateReady(cert) {
		return nil, ErrNotReady
	}

	secret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: cert.Spec.SecretName, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (r *commonReconciler) GetContainerJFRCABytes(ctx context.Context, cjfr *rhjmcv1alpha1.ContainerJFR) ([]byte, error) {
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
