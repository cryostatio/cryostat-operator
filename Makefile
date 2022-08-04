# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# OS information
OS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)

# Current Operator version
IMAGE_VERSION ?= 2.2.0-dev
BUNDLE_VERSION ?= $(IMAGE_VERSION)
DEFAULT_NAMESPACE ?= quay.io/cryostat
IMAGE_NAMESPACE ?= $(DEFAULT_NAMESPACE)
OPERATOR_NAME ?= cryostat-operator
CLUSTER_CLIENT ?= kubectl
IMAGE_TAG_BASE ?= $(IMAGE_NAMESPACE)/$(OPERATOR_NAME)

# Default bundle image tag
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(BUNDLE_VERSION)
BUNDLE_IMGS ?= $(BUNDLE_IMG) 

# Default catalog image tag
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(BUNDLE_VERSION) 
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG) 
endif 

# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)
# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(BUNDLE_VERSION) $(BUNDLE_METADATA_OPTS)
# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

IMAGE_BUILDER ?= podman
# Image URL to use all building/pushing image targets
OPERATOR_IMG ?= $(IMAGE_TAG_BASE):$(IMAGE_VERSION)


# Images used by the operator
CORE_NAMESPACE ?= $(DEFAULT_NAMESPACE)
CORE_NAME ?= cryostat
CORE_VERSION ?= latest
export CORE_IMG ?= $(CORE_NAMESPACE)/$(CORE_NAME):$(CORE_VERSION)
DATASOURCE_NAMESPACE ?= $(DEFAULT_NAMESPACE)
DATASOURCE_NAME ?= jfr-datasource
DATASOURCE_VERSION ?= latest
export DATASOURCE_IMG ?= $(DATASOURCE_NAMESPACE)/$(DATASOURCE_NAME):$(DATASOURCE_VERSION)
GRAFANA_NAMESPACE ?= $(DEFAULT_NAMESPACE)
GRAFANA_NAME ?= cryostat-grafana-dashboard
GRAFANA_VERSION ?= latest
export GRAFANA_IMG ?= $(GRAFANA_NAMESPACE)/$(GRAFANA_NAME):$(GRAFANA_VERSION)
REPORTS_NAMESPACE ?= $(DEFAULT_NAMESPACE)
REPORTS_NAME ?= cryostat-reports
REPORTS_VERSION ?= latest
export REPORTS_IMG ?= $(REPORTS_NAMESPACE)/$(REPORTS_NAME):$(REPORTS_VERSION)

CERT_MANAGER_VERSION ?= 1.5.3
CERT_MANAGER_MANIFEST ?= \
	https://github.com/jetstack/cert-manager/releases/download/v$(CERT_MANAGER_VERSION)/cert-manager.yaml

KUSTOMIZE_VERSION ?= 3.8.7
CONTROLLER_GEN_VERSION ?= 0.9.0
ADDLICENSE_VERSION ?= 1.0.0
OPM_VERSION ?= 1.23.0
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION ?= 1.24 

DEPLOY_NAMESPACE ?= cryostat-operator-system

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Check whether this is a development or release version
ifneq (,$(shell echo $(IMAGE_VERSION) | grep -iE '(:latest|SNAPSHOT|dev|BETA[[:digit:]]+)$$'))
PULL_POLICY ?= Always
else
PULL_POLICY ?= IfNotPresent
endif
export PULL_POLICY

# Run tests with Ginkgo CLI if available
GINKGO ?= $(shell go env GOPATH)/bin/ginkgo
GO_TEST ?= go test
ifneq ("$(wildcard $(GINKGO))","")
GO_TEST="$(GINKGO)" -cover -outputdir=.
endif

.PHONY: all
all: manager

# Run tests
.PHONY: test
test: test-envtest test-scorecard

# FIXME remove ACK_GINKGO_DEPRECATIONS when upgrading to ginkgo 2.0
.PHONY: test-envtest
test-envtest: generate manifests fmt vet setup-envtest
ifneq ($(SKIP_TESTS), true)
	ACK_GINKGO_DEPRECATIONS=1.16.5  KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GO_TEST) -v -coverprofile cover.out ./...
endif

.PHONY: test-scorecard
test-scorecard: destroy_cryostat_cr undeploy uninstall
ifneq ($(SKIP_TESTS), true)
	operator-sdk scorecard bundle
endif

# Build manager binary
.PHONY: manager
manager: generate fmt vet
	go build -o bin/manager internal/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate fmt vet manifests
	go run ./internal/main.go

ifndef ignore-not-found
  ignore-not-found = false
endif

# Install CRDs into a cluster
.PHONY: install
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | $(CLUSTER_CLIENT) apply -f -

# Uninstall CRDs from a cluster
.PHONY: uninstall
uninstall: manifests kustomize
	- $(KUSTOMIZE) build config/crd | $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: predeploy
predeploy:
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMG)
	cd config/default && $(KUSTOMIZE) edit set namespace $(DEPLOY_NAMESPACE)

.PHONY: print_deploy_config
print_deploy_config: predeploy
	$(KUSTOMIZE) build config/default

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: check_cert_manager manifests kustomize predeploy
	$(KUSTOMIZE) build config/default | $(CLUSTER_CLIENT) apply -f -
ifeq ($(DISABLE_SERVICE_TLS), true)
	@echo "Disabling TLS for in-cluster communication between Services"
	@$(CLUSTER_CLIENT) -n $(DEPLOY_NAMESPACE) set env deployment/cryostat-operator-controller-manager DISABLE_SERVICE_TLS=true
endif

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
.PHONY: undeploy
undeploy:
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) recording --all
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f config/samples/operator_v1beta1_cryostat.yaml
	- $(KUSTOMIZE) build config/default | $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	envsubst < hack/image_tag_patch.yaml.in > config/default/image_tag_patch.yaml
	envsubst < hack/image_pull_patch.yaml.in > config/default/image_pull_patch.yaml

# Run go fmt against code
.PHONY: fmt
fmt: add-license
	go fmt ./...

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate: controller-gen
	go generate ./...
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Check and add (if missing) license header
LICENSE_FILE = $(shell pwd)/LICENSE
GO_PACKAGES := $(shell go list -test -f '{{.Dir}}' ./... | sed -e "s|^$$(pwd)||" | cut -d/ -f2 | sort -u)
.PHONY: add-license 
add-license: addlicense
	@echo "Checking/Adding license..."
	$(ADDLICENSE) -v -f $(LICENSE_FILE) ${GO_PACKAGES}

# Build the OCI image
.PHONY: oci-build
oci-build: generate manifests manager test-envtest
	BUILDAH_FORMAT=docker $(IMAGE_BUILDER) build -t $(OPERATOR_IMG) .


.PHONY: cert_manager
cert_manager: remove_cert_manager
	$(CLUSTER_CLIENT) create --validate=false -f $(CERT_MANAGER_MANIFEST)

.PHONY: remove_cert_manager
remove_cert_manager:
	- $(CLUSTER_CLIENT) delete -f $(CERT_MANAGER_MANIFEST)

.PHONY: check_cert_manager
check_cert_manager:
	@api_versions=$$($(CLUSTER_CLIENT) api-versions) &&\
       if [ $$(echo "$${api_versions}" | grep -c '^cert-manager.io/v1$$') -eq 0 ]; then if [ "$${DISABLE_SERVICE_TLS}" != "true" ]; then\
                       echo 'cert-manager is not installed, install using "make cert_manager" or disable TLS for services by setting DISABLE_SERVICE_TLS to true' >&2\
                       && exit 1;\
               fi;\
       fi

# Location to install dependencies
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Download controller-gen locally if necessary
CONTROLLER_GEN = $(LOCALBIN)/controller-gen
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@v$(CONTROLLER_GEN_VERSION)

# Download kustomize locally if necessary
KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
KUSTOMIZE = $(LOCALBIN)/kustomize
.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): $(LOCALBIN)
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

# Download addlicense locally if necessary
ADDLICENSE = $(LOCALBIN)/addlicense
.PHONY: addlicense
addlicense: $(ADDLICENSE)
$(ADDLICENSE): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/google/addlicense@v$(ADDLICENSE_VERSION)

# Download setup-envtest locally if necessary
ENVTEST = $(LOCALBIN)/setup-envtest
.PHONY: setup-envtest
setup-envtest: $(ENVTEST)
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download opm locally if necessary
OPM = $(LOCALBIN)/opm
.PHONY: opm
opm: $(OPM)
$(OPM): $(LOCALBIN)
	{ \
	set -e ;\
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v$(OPM_VERSION)/$(OS)-$(ARCH)-opm ;\
	chmod +x $(OPM) ;\
	}

.PHONY: catalog-build
catalog-build: opm
	$(OPM) index add --container-tool $(IMAGE_BUILDER) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle $(BUNDLE_GEN_FLAGS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	$(IMAGE_BUILDER) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: deploy_bundle
deploy_bundle: check_cert_manager undeploy_bundle
	operator-sdk run bundle $(BUNDLE_IMG)
ifeq ($(DISABLE_SERVICE_TLS), true)
	@echo "Disabling TLS for in-cluster communication between Services"
	@current_ns=`$(CLUSTER_CLIENT) config view --minify -o 'jsonpath={.contexts[0].context.namespace}'` && \
	if [ -z "$${current_ns}" ]; then \
		echo "Failed to determine Namespace in current context" >&2; \
		exit 1; \
	fi; \
	set -f -- `$(CLUSTER_CLIENT) get sub -l "operators.coreos.com/$(OPERATOR_NAME).$${current_ns}" -o name` && \
	if [ "$${#}" -ne 1 ]; then \
		echo -e "Expected 1 Subscription, found $${#}:\n$${@}" >&2; \
		exit 1; \
	fi; \
	$(CLUSTER_CLIENT) patch --type=merge -p '{"spec":{"config":{"env":[{"name":"DISABLE_SERVICE_TLS","value":"true"}]}}}' "$${1}"
endif

.PHONY: undeploy_bundle
undeploy_bundle:
	- operator-sdk cleanup $(OPERATOR_NAME)

# Deploy a Cryostat instance
.PHONY: create_cryostat_cr
create_cryostat_cr: destroy_cryostat_cr
	$(CLUSTER_CLIENT) create -f config/samples/operator_v1beta1_cryostat.yaml

# Undeploy a Cryostat instance
.PHONY: destroy_cryostat_cr
destroy_cryostat_cr:
	- $(CLUSTER_CLIENT) delete -f config/samples/operator_v1beta1_cryostat.yaml

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
