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
