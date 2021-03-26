# Current Operator version
IMAGE_VERSION ?= 1.0.0-beta5
BUNDLE_VERSION ?= $(IMAGE_VERSION)
IMAGE_NAMESPACE ?= quay.io/rh-jmc-team
OPERATOR_NAME ?= container-jfr-operator
CLUSTER_CLIENT ?= kubectl

# Default bundle image tag
BUNDLE_IMG ?= $(IMAGE_NAMESPACE)/$(OPERATOR_NAME)-bundle:$(BUNDLE_VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

IMAGE_BUILDER ?= podman
# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_NAMESPACE)/$(OPERATOR_NAME):$(IMAGE_VERSION)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

CERT_MANAGER_VERSION ?= 1.1.0
CERT_MANAGER_MANIFEST ?= \
	https://github.com/jetstack/cert-manager/releases/download/v$(CERT_MANAGER_VERSION)/cert-manager.yaml

DEPLOY_NAMESPACE ?= container-jfr-operator-system

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Run tests with Ginkgo CLI if available
GINKGO ?= $(shell go env GOPATH)/bin/ginkgo
GO_TEST ?= go test
ifneq ("$(wildcard $(GINKGO))","")
GO_TEST="$(GINKGO)" -cover -outputdir=.
endif

all: manager

# Run tests
.PHONY: test
test: test-envtest test-scorecard

.PHONY: test-envtest
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test-envtest: generate fmt vet manifests
	mkdir -p $(ENVTEST_ASSETS_DIR)
	test -f $(ENVTEST_ASSETS_DIR)/setup-envtest.sh || curl -sSLo $(ENVTEST_ASSETS_DIR)/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.2/hack/setup-envtest.sh
	source $(ENVTEST_ASSETS_DIR)/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); $(GO_TEST) -v -coverprofile cover.out ./...

.PHONY: test-scorecard
test-scorecard: destroy_containerjfr_cr undeploy uninstall
	operator-sdk scorecard bundle



# Build manager binary
.PHONY: manager
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
.PHONY: install
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | $(CLUSTER_CLIENT) apply -f -

# Uninstall CRDs from a cluster
.PHONY: uninstall
uninstall: manifests kustomize
	- $(KUSTOMIZE) build config/crd | $(CLUSTER_CLIENT) delete -f -

.PHONY: predeploy
predeploy:
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	cd config/default && $(KUSTOMIZE) edit set namespace $(DEPLOY_NAMESPACE)

.PHONY: print_deploy_config
print_deploy_config: predeploy
	$(KUSTOMIZE) build config/default

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests kustomize predeploy
	$(KUSTOMIZE) build config/default | $(CLUSTER_CLIENT) apply -f -
ifeq ($(DISABLE_SERVICE_TLS), true)
	$(CLUSTER_CLIENT) -n $(DEPLOY_NAMESPACE) set env deployment/container-jfr-operator-controller-manager DISABLE_SERVICE_TLS=true
endif

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
.PHONY: undeploy
undeploy:
	- $(CLUSTER_CLIENT) delete recording --all
	- $(CLUSTER_CLIENT) delete -f config/samples/rhjmc_v1beta1_containerjfr.yaml
	- $(KUSTOMIZE) build config/default | $(CLUSTER_CLIENT) delete -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
.PHONY: fmt
fmt:
	go fmt ./...

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the OCI image
.PHONY: oci-build
oci-build: generate manifests manager test-envtest
	BUILDAH_FORMAT=docker $(IMAGE_BUILDER) build -t $(IMG) .


.PHONY: cert_manager
cert_manager: remove_cert_manager
	$(CLUSTER_CLIENT) create --validate=false -f $(CERT_MANAGER_MANIFEST)

.PHONY: remove_cert_manager
remove_cert_manager:
	- $(CLUSTER_CLIENT) delete -f $(CERT_MANAGER_MANIFEST)



# Download controller-gen locally if necessary
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

# Download kustomize locally if necessary
KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize:
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(BUNDLE_VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	$(IMAGE_BUILDER) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: deploy_bundle
deploy_bundle: undeploy_bundle
	operator-sdk run bundle $(IMAGE_NAMESPACE)/$(OPERATOR_NAME)-bundle:$(IMAGE_VERSION)
ifeq ($(DISABLE_SERVICE_TLS), true)
	$(CLUSTER_CLIENT) set env deployment/container-jfr-operator-controller-manager DISABLE_SERVICE_TLS=true
endif

.PHONY: undeploy_bundle
undeploy_bundle:
	- operator-sdk cleanup $(OPERATOR_NAME)

.PHONY: create_containerjfr_cr
create_containerjfr_cr: destroy_containerjfr_cr
	$(CLUSTER_CLIENT) create -f config/samples/rhjmc_v1beta1_containerjfr.yaml

.PHONY: destroy_containerjfr_cr
destroy_containerjfr_cr:
	- $(CLUSTER_CLIENT) delete -f config/samples/rhjmc_v1beta1_containerjfr.yaml



# Local development/testing helpers

.PHONY: sample_app
sample_app: undeploy_sample_app
	$(call new-sample-app,quay.io/andrewazores/vertx-fib-demo:0.1.0)

.PHONY: undeploy_sample_app
undeploy_sample_app:
	- $(CLUSTER_CLIENT) delete all -l app=vertx-fib-demo

.PHONY: sample_app2
sample_app2: undeploy_sample_app2
	$(call new-sample-app,quay.io/andrewazores/container-jmx-docker-listener:0.1.0 --name=jmx-listener)
	$(CLUSTER_CLIENT) patch svc/jmx-listener -p '{"spec":{"$setElementOrder/ports":[{"port":7095},{"port":9092},{"port":9093}],"ports":[{"name":"jfr-jmx","port":9093}]}}'

.PHONY: undeploy_sample_app2
undeploy_sample_app2:
	- $(CLUSTER_CLIENT) delete all -l app=jmx-listener

.PHONY: sample_app_quarkus
sample_app_quarkus: undeploy_sample_app_quarkus
	$(call new-sample-app,quay.io/andrewazores/quarkus-test:0.0.2)
	$(CLUSTER_CLIENT) patch svc/quarkus-test -p '{"spec":{"$setElementOrder/ports":[{"port":9096},{"port":9999}],"ports":[{"name":"jfr-jmx","port":9096}]}}'

.PHONY: undeploy_sample_app_quarkus
undeploy_sample_app_quarkus:
	- $(CLUSTER_CLIENT) delete all -l app=quarkus-test

define new-sample-app
@if [ ! "$(CLUSTER_CLIENT)" = "oc" ]; then echo "CLUSTER_CLIENT must be 'oc' for sample app deployments" && exit 1; fi
$(CLUSTER_CLIENT) new-app $(1)
endef
