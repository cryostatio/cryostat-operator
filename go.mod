module github.com/rh-jmc-team/container-jfr-operator

require (
	github.com/go-logr/logr v0.2.1-0.20200730175230-ee2de8da5be6
	github.com/google/go-cmp v0.4.1
	github.com/jetstack/cert-manager v1.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v0.0.0-00010101000000-000000000000
	github.com/operator-framework/operator-sdk v0.19.0
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.2
)

// Pinned to kubernetes-1.16.2
replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.19.2 // Required by prometheus-operator
)

replace github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309 // Required by Helm

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad // Required until https://github.com/operator-framework/operator-lifecycle-manager/pull/1241 is resolved

// Required for cert-manager until we upgrade Operator SDK
replace github.com/go-logr/zapr => github.com/go-logr/zapr v0.2.0

go 1.13
