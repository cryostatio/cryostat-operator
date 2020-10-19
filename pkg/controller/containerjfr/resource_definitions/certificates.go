// Copyright (c) 2020 Red Hat, Inc.
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

package resource_definitions

import (
	"fmt"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	certMeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	rhjmcv1alpha1 "github.com/rh-jmc-team/container-jfr-operator/pkg/apis/rhjmc/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CAKey is the key for a CA certificate within a TLS secret
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

func NewGrafanaCert(cr *rhjmcv1alpha1.ContainerJFR) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-grafana",
			Namespace: cr.Namespace,
		},
		Spec: certv1.CertificateSpec{
			CommonName: fmt.Sprintf("%s-grafana.%s.svc", cr.Name, cr.Namespace),
			DNSNames: []string{
				cr.Name,
				fmt.Sprintf("%s-grafana.%s.svc", cr.Name, cr.Namespace),
				fmt.Sprintf("%s-grafana.%s.svc.cluster.local", cr.Name, cr.Namespace),
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
