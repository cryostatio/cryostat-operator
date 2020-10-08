package resource_definitions

import (
	"fmt"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const CAKey = certMeta.TLSCAKey

func NewSelfSignedIssuer(cr *rhjmcv1alpha1.ContainerJFR) *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-self-signed",
			Namespace: cr.Namespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				SelfSigned: &certv1.SelfSignedIssuer{},
			},
		},
	}
}

func NewContainerJFRCAIssuer(cr *rhjmcv1alpha1.ContainerJFR) *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca",
			Namespace: cr.Namespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				CA: &certv1.CAIssuer{
					SecretName: cr.Name + "-ca",
				},
			},
		},
	}
}

func NewContainerJFRCACert(cr *rhjmcv1alpha1.ContainerJFR) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca",
			Namespace: cr.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("ca.%s.cert-manager", cr.Name),
			SecretName: cr.Name + "-ca",
			IssuerRef: certMeta.ObjectReference{
				Name: cr.Name + "-self-signed",
			},
			IsCA: true,
		},
	}
}

func NewContainerJFRCert(cr *rhjmcv1alpha1.ContainerJFR) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s.%s.svc", cr.Name, cr.Namespace),
			DNSNames: []string{
				cr.Name,
				fmt.Sprintf("%s.%s.svc", cr.Name, cr.Namespace),
				fmt.Sprintf("%s.%s.svc.cluster.local", cr.Name, cr.Namespace),
			},
			SecretName: cr.Name + "-tls",
			Keystores: &certv1.CertificateKeystores{
				PKCS12: &certv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: certMeta.SecretKeySelector{
						LocalObjectReference: certMeta.LocalObjectReference{
							Name: cr.Name + "-keystore",
						},
						Key: "KEYSTORE_PASS",
					},
				},
			},
			IssuerRef: certMeta.ObjectReference{
				Name: cr.Name + "-ca",
			},
			Usages: append(certv1.DefaultKeyUsages(),
				certv1.UsageServerAuth,
				certv1.UsageClientAuth,
			),
		},
	}
}
