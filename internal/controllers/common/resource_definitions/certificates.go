package resource_definitions

import (
	"fmt"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/constants"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewSelfSignedIssuer(cr *model.CryostatInstance) *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-self-signed",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				SelfSigned: &certv1.SelfSignedIssuer{},
			},
		},
	}
}

func NewCryostatCAIssuer(cr *model.CryostatInstance) *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca",
			Namespace: cr.InstallNamespace,
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

func NewCryostatCACert(cr *model.CryostatInstance) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca",
			Namespace: cr.InstallNamespace,
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

func NewCryostatCert(cr *model.CryostatInstance, keystoreSecretName string) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s.%s.svc", cr.Name, cr.InstallNamespace),
			DNSNames: []string{
				cr.Name,
				fmt.Sprintf("%s.%s.svc", cr.Name, cr.InstallNamespace),
				fmt.Sprintf("%s.%s.svc.cluster.local", cr.Name, cr.InstallNamespace),
			},
			SecretName: cr.Name + "-tls",
			Keystores: &certv1.CertificateKeystores{
				PKCS12: &certv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: certMeta.SecretKeySelector{
						LocalObjectReference: certMeta.LocalObjectReference{
							Name: keystoreSecretName,
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

func NewGrafanaCert(cr *model.CryostatInstance) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s-grafana.%s.svc", cr.Name, cr.InstallNamespace),
			DNSNames: []string{
				cr.Name + "-grafana",
				fmt.Sprintf("%s-grafana.%s.svc", cr.Name, cr.InstallNamespace),
				fmt.Sprintf("%s-grafana.%s.svc.cluster.local", cr.Name, cr.InstallNamespace),
				constants.HealthCheckHostname,
			},
			SecretName: cr.Name + "-grafana-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: cr.Name + "-ca",
			},
			Usages: append(certv1.DefaultKeyUsages(),
				certv1.UsageServerAuth,
			),
		},
	}
}

func NewReportsCert(cr *model.CryostatInstance) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-reports",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s-reports.%s.svc", cr.Name, cr.InstallNamespace),
			DNSNames: []string{
				cr.Name + "-reports",
				fmt.Sprintf("%s-reports.%s.svc", cr.Name, cr.InstallNamespace),
				fmt.Sprintf("%s-reports.%s.svc.cluster.local", cr.Name, cr.InstallNamespace),
			},
			SecretName: cr.Name + "-reports-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: cr.Name + "-ca",
			},
			Usages: append(certv1.DefaultKeyUsages(),
				certv1.UsageServerAuth,
			),
		},
	}
}
