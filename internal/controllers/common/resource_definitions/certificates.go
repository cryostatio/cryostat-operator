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

package resource_definitions

import (
	"fmt"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/cryostatio/cryostat-operator/internal/controllers/common"
	"github.com/cryostatio/cryostat-operator/internal/controllers/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func NewCryostatCAIssuer(gvk *schema.GroupVersionKind, cr *model.CryostatInstance) *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				CA: &certv1.CAIssuer{
					SecretName: NewCryostatCACert(gvk, cr).Spec.SecretName,
				},
			},
		},
	}
}

func NewCryostatCACert(gvk *schema.GroupVersionKind, cr *model.CryostatInstance) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ca",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("ca.%s.cert-manager", cr.Name),
			SecretName: common.ClusterUniqueNameWithPrefix(gvk, "ca", cr.Name, cr.InstallNamespace),
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

/**
func NewDatabaseCert(cr *model.CryostatInstance) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-database",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s-database.%s.svc", cr.Name, cr.InstallNamespace),
			DNSNames: []string{
				cr.Name + "-database",
				fmt.Sprintf("%s-database.%s.svc", cr.Name, cr.InstallNamespace),
				fmt.Sprintf("%s-database.%s.svc.cluster.local", cr.Name, cr.InstallNamespace),
			},
			SecretName: cr.Name + "-database-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: cr.Name + "-ca",
			},
			Usages: append(certv1.DefaultKeyUsages(),
				certv1.UsageServerAuth,
			),
		},
	}
}

func NewStorageCert(cr *model.CryostatInstance) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-storage",
			Namespace: cr.InstallNamespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s-storage.%s.svc", cr.Name, cr.InstallNamespace),
			DNSNames: []string{
				cr.Name + "-storage",
				fmt.Sprintf("%s-storage.%s.svc", cr.Name, cr.InstallNamespace),
				fmt.Sprintf("%s-storage.%s.svc.cluster.local", cr.Name, cr.InstallNamespace),
			},
			SecretName: cr.Name + "-storage-tls",
			IssuerRef: certMeta.ObjectReference{
				Name: cr.Name + "-ca",
			},
			Usages: append(certv1.DefaultKeyUsages(),
				certv1.UsageServerAuth,
			),
		},
	}
}
**/
