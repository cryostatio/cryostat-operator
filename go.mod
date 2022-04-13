module github.com/cryostatio/cryostat-operator

go 1.16

require (
	github.com/go-logr/logr v0.3.0
	github.com/jetstack/cert-manager v1.1.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/openshift/api v3.9.0+incompatible
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v0.19.2
	sigs.k8s.io/controller-runtime v0.7.2
)

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20200618202633-7192180f496a

// Fix for CVE-2020-26160, revisit when upgrading client-go
require github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
replace github.com/dgrijalva/jwt-go => github.com/golang-jwt/jwt v3.2.1+incompatible

// Fix for CVE-2021-3121, revisit when upgrading client-go
replace github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
