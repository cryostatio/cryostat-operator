# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# OS information
OS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)

# Current Operator version
IMAGE_VERSION ?= 2.4.0-dev
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
BUNDLE_INSTALL_MODE ?= AllNamespaces

IMAGE_BUILDER ?= podman
# Image URL to use all building/pushing image targets
OPERATOR_IMG ?= $(IMAGE_TAG_BASE):$(IMAGE_VERSION)

# Name of the application deployed by the operator
export APP_NAME ?= Cryostat

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

CERT_MANAGER_VERSION ?= 1.7.1
CERT_MANAGER_MANIFEST ?= \
	https://github.com/jetstack/cert-manager/releases/download/v$(CERT_MANAGER_VERSION)/cert-manager.yaml

KUSTOMIZE_VERSION ?= 3.8.7
CONTROLLER_TOOLS_VERSION ?= 0.11.1
ADDLICENSE_VERSION ?= 1.0.0
OPM_VERSION ?= 1.23.0
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION ?= 1.26

# Scorecard ImagePullPolicy is hardcoded to IfNotPresent
# See: https://github.com/operator-framework/operator-sdk/pull/4762
#
# Suffix is the timestamp of the image build, compute with: date -u '+%Y%m%d%H%M%S'
CUSTOM_SCORECARD_VERSION ?= 2.4.0-$(shell date -u '+%Y%m%d%H%M%S')
export CUSTOM_SCORECARD_IMG ?= $(IMAGE_TAG_BASE)-scorecard:$(CUSTOM_SCORECARD_VERSION)

DEPLOY_NAMESPACE ?= cryostat-operator-system
TARGET_NAMESPACES ?= $(DEPLOY_NAMESPACE)
SCORECARD_NAMESPACE ?= cryostat-operator-scorecard

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
GO_TEST="$(GINKGO)" -cover -output-dir=.
endif

.PHONY: all
all: manager

# Run tests
.PHONY: test
test: test-envtest test-scorecard

.PHONY: test-envtest
test-envtest: generate manifests fmt vet setup-envtest
ifneq ($(SKIP_TESTS), true)
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GO_TEST) -v -coverprofile cover.out ./...
endif

.PHONY: test-scorecard
test-scorecard: check_cert_manager kustomize
ifneq ($(SKIP_TESTS), true)
	$(call scorecard-setup)
	$(call scorecard-cleanup); \
	trap cleanup EXIT; \
	operator-sdk scorecard -n $(SCORECARD_NAMESPACE) -s cryostat-scorecard -w 20m $(BUNDLE_IMG) --pod-security=restricted
endif

.PHONY: clean-scorecard
clean-scorecard:
	- $(call scorecard-cleanup); cleanup

ifneq ($(and $(SCORECARD_REGISTRY_SERVER),$(SCORECARD_REGISTRY_USERNAME),$(SCORECARD_REGISTRY_PASSWORD)),)
SCORECARD_ARGS := --pull-secret-name registry-key --service-account cryostat-scorecard
endif

define scorecard-setup
@$(CLUSTER_CLIENT) get namespace $(SCORECARD_NAMESPACE) >/dev/null 2>&1 &&\
	echo "$(SCORECARD_NAMESPACE) namespace already exists, please remove it with \"make clean-scorecard\"" >&2 && exit 1 || true
$(CLUSTER_CLIENT) create namespace $(SCORECARD_NAMESPACE)
cd internal/images/custom-scorecard-tests/rbac/ && $(KUSTOMIZE) edit set namespace $(SCORECARD_NAMESPACE)
$(KUSTOMIZE) build internal/images/custom-scorecard-tests/rbac/ | $(CLUSTER_CLIENT) apply -f -
@if [ -n "$(SCORECARD_ARGS)" ]; then \
	$(CLUSTER_CLIENT) create -n $(SCORECARD_NAMESPACE) secret docker-registry registry-key --docker-server="$(SCORECARD_REGISTRY_SERVER)" \
		--docker-username="$(SCORECARD_REGISTRY_USERNAME)" --docker-password="$(SCORECARD_REGISTRY_PASSWORD)"; \
	$(CLUSTER_CLIENT) patch sa cryostat-scorecard -n $(SCORECARD_NAMESPACE) -p '{"imagePullSecrets": [{"name": "registry-key"}]}'; \
fi
operator-sdk run bundle -n $(SCORECARD_NAMESPACE) --timeout 20m $(BUNDLE_IMG) $(SCORECARD_ARGS)
endef

define scorecard-cleanup
function cleanup { \
	(\
	set +e; \
	operator-sdk cleanup -n $(SCORECARD_NAMESPACE) $(OPERATOR_NAME); \
	$(KUSTOMIZE) build internal/images/custom-scorecard-tests/rbac/ | $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f -; \
	$(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -n $(SCORECARD_NAMESPACE) secret registry-key; \
	$(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) namespace $(SCORECARD_NAMESPACE); \
	)\
}
endef

# Build manager binary
.PHONY: manager
manager: manifests generate fmt vet
	go build -o bin/manager internal/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: manifests generate fmt vet
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

# Undeploy controller from the configured Kubernetes cluster in ~/.kube/config
.PHONY: undeploy
undeploy:
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f config/samples/operator_v1beta1_cryostat.yaml
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f config/samples/operator_v1beta1_clustercryostat.yaml
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
oci-build: manifests generate fmt vet test-envtest
	BUILDAH_FORMAT=docker $(IMAGE_BUILDER) build --build-arg TARGETOS=$(OS) --build-arg TARGETARCH=$(ARCH) -t $(OPERATOR_IMG) .

# PLATFORMS defines the target platforms for the manager image to provide support to multiple
# architectures. (i.e. make oci-buildx OPERATOR_IMG=quay.io/cryostat/cryostat-operator:latest).
# You need to be able to push the image for your registry (i.e. if you do not inform a valid value via OPERATOR_IMG=<myregistry/image:<tag>> than the export will fail)
# If IMAGE_BUILDER is docker, you need to:
# - able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# If IMAGE_BUILDER is podman, you need to:
# - install qemu-user-static.
# To properly provided solutions that supports more than one platform you should use this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: oci-buildx
oci-buildx: manifests generate fmt vet test-envtest ## Build OCI image for the manager for cross-platform support
ifeq ($(IMAGE_BUILDER), docker)
# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag $(OPERATOR_IMG) -f Dockerfile.cross .
	- docker buildx rm project-v3-builder
	rm Dockerfile.cross
else ifeq ($(IMAGE_BUILDER), podman)
	for platform in $$(echo $(PLATFORMS) | sed "s/,/ /g"); do \
		os=$$(echo $${platform} | cut -d/ -f 1); \
		arch=$$(echo $${platform} | cut -d/ -f 2); \
		BUILDAH_FORMAT=docker podman buildx build --manifest $(OPERATOR_IMG) --platform $${platform} --build-arg TARGETOS=$${os} --build-arg TARGETARCH=$${arch} . ; \
	done
	podman manifest push $(OPERATOR_IMG) $(OPERATOR_IMG)
else
	$(error unsupported IMAGE_BUILDER: $(IMAGE_BUILDER))
endif

.PHONY: cert_manager
cert_manager: remove_cert_manager
	$(CLUSTER_CLIENT) create --validate=false -f $(CERT_MANAGER_MANIFEST)

.PHONY: remove_cert_manager
remove_cert_manager:
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f $(CERT_MANAGER_MANIFEST)

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
PHONY: local-bin
local-bin:
	mkdir -p $(LOCALBIN)

# Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
CONTROLLER_GEN = $(LOCALBIN)/controller-gen
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): local-bin
	test -s $(CONTROLLER_GEN) && $(CONTROLLER_GEN) --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@v$(CONTROLLER_TOOLS_VERSION)

# Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
KUSTOMIZE = $(LOCALBIN)/kustomize
.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): local-bin
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(KUSTOMIZE) || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

# Download addlicense locally if necessary
ADDLICENSE = $(LOCALBIN)/addlicense
.PHONY: addlicense
addlicense: $(ADDLICENSE)
$(ADDLICENSE): local-bin
	test -s $(ADDLICENSE) || GOBIN=$(LOCALBIN) go install github.com/google/addlicense@v$(ADDLICENSE_VERSION)

# Download setup-envtest locally if necessary
ENVTEST = $(LOCALBIN)/setup-envtest
.PHONY: setup-envtest
setup-envtest: $(ENVTEST)
$(ENVTEST): local-bin
	test -s $(ENVTEST) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download opm locally if necessary
OPM = $(LOCALBIN)/opm
.PHONY: opm
opm: $(OPM)
$(OPM): local-bin
	test -s $(OPM) || \
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
	operator-sdk run bundle --install-mode $(BUNDLE_INSTALL_MODE) $(BUNDLE_IMG)
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

.PHONY: create_clustercryostat_cr
create_clustercryostat_cr: destroy_clustercryostat_cr
	target_ns_json=$$(jq -nc '$$ARGS.positional' --args -- $(TARGET_NAMESPACES)) && \
	$(CLUSTER_CLIENT) patch -f config/samples/operator_v1beta1_clustercryostat.yaml --local=true --type=merge \
	-p "{\"spec\": {\"installNamespace\": \"$(DEPLOY_NAMESPACE)\", \"targetNamespaces\": $$target_ns_json}}" -o yaml | \
	oc apply -f -

# Undeploy a Cryostat instance
.PHONY: destroy_cryostat_cr
destroy_cryostat_cr:
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f config/samples/operator_v1beta1_cryostat.yaml

.PHONY: destroy_clustercryostat_cr
destroy_clustercryostat_cr:
	- $(CLUSTER_CLIENT) delete --ignore-not-found=$(ignore-not-found) -f config/samples/operator_v1beta1_clustercryostat.yaml

# Build custom scorecard tests
.PHONY: custom-scorecard-tests
custom-scorecard-tests: fmt vet
	cd internal/images/custom-scorecard-tests/ && \
	go build -o bin/cryostat-scorecard-tests main.go

# Build the custom scorecard OCI image
.PHONY: scorecard-build
scorecard-build: custom-scorecard-tests
	printf '# Code generated by hack/custom.config.yaml.in. DO NOT EDIT.\n' > config/scorecard/patches/custom.config.yaml
	envsubst < hack/custom.config.yaml.in >> config/scorecard/patches/custom.config.yaml
	BUILDAH_FORMAT=docker $(IMAGE_BUILDER) build -t $(CUSTOM_SCORECARD_IMG) \
	-f internal/images/custom-scorecard-tests/Dockerfile .

# Local development/testing helpers
ifneq ($(origin SAMPLE_APP_NAMESPACE), undefined)
SAMPLE_APP_FLAGS += -n $(SAMPLE_APP_NAMESPACE)
endif

.PHONY: sample_app
sample_app:
	$(CLUSTER_CLIENT) apply $(SAMPLE_APP_FLAGS) -f config/samples/sample-app.yaml 

.PHONY: undeploy_sample_app
undeploy_sample_app:
	$(CLUSTER_CLIENT) delete $(SAMPLE_APP_FLAGS) --ignore-not-found=$(ignore-not-found) -f config/samples/sample-app.yaml

.PHONY: sample_app_agent
sample_app_agent: undeploy_sample_app_agent
	@if [ -z "${AUTH_TOKEN}" ]; then \
		if [ "${CLUSTER_CLIENT}" = "oc" ]; then\
			AUTH_TOKEN=`oc whoami -t | base64`; \
		else \
			echo "'AUTH_TOKEN' must be specified."; \
			exit 1; \
		fi; \
	fi; \
	$(CLUSTER_CLIENT) apply $(SAMPLE_APP_FLAGS) -f config/samples/sample-app-agent.yaml; \
	$(CLUSTER_CLIENT) set env $(SAMPLE_APP_FLAGS) deployment/quarkus-test-agent CRYOSTAT_AGENT_AUTHORIZATION="Bearer $(AUTH_TOKEN)"

.PHONY: undeploy_sample_app_agent
undeploy_sample_app_agent:
	- $(CLUSTER_CLIENT) delete $(SAMPLE_APP_FLAGS) --ignore-not-found=$(ignore-not-found) -f config/samples/sample-app-agent.yaml
