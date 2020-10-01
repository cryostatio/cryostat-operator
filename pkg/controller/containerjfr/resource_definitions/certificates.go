package resource_definitions

import (
	certv1beta1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1beta1"
)

var _ = certv1beta1.Issuer{}
