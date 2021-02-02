IMAGE_STREAM ?= quay.io/rh-jmc-team/container-jfr-operator
IMAGE_VERSION ?= 0.5.0
IMAGE_TAG ?= $(IMAGE_STREAM):$(IMAGE_VERSION)

BUNDLE_STREAM ?= $(IMAGE_STREAM)-bundle
CSV_VERSION ?= $(IMAGE_VERSION)
PREV_CSV_VERSION ?= $(CSV_VERSION)

INDEX_STREAM ?= $(IMAGE_STREAM)-index
INDEX_VERSION ?= 0.0.2
PREV_INDEX_VERSION ?= $(INDEX_VERSION)

BUILDER ?= podman

GINKGO ?= $(shell go env GOPATH)/bin/ginkgo

CERT_MANAGER_VERSION ?= 1.0.2
CERT_MANAGER_MANIFEST ?= \
	https://github.com/jetstack/cert-manager/releases/download/v$(CERT_MANAGER_VERSION)/cert-manager.yaml

.DEFAULT_GOAL := bundle

.PHONY: generate
generate: k8s crds

.PHONY: k8s
k8s:
	operator-sdk generate k8s

.PHONY: crds
crds:
	operator-sdk generate crds

.PHONY: csv
csv: generate
ifeq ($(CSV_VERSION), $(PREV_CSV_VERSION))
	operator-sdk generate csv \
		--csv-config ./deploy/olm-catalog/csv-config.yaml \
		--csv-version $(CSV_VERSION) \
		--csv-channel alpha \
		--default-channel \
		--update-crds \
		--operator-name container-jfr-operator-bundle
else
	# TODO
	# from-version should be programatically determined
	operator-sdk generate csv \
		--from-version $(PREV_CSV_VERSION) \
		--csv-config ./deploy/olm-catalog/csv-config.yaml \
		--csv-version $(CSV_VERSION) \
		--csv-channel alpha \
		--default-channel \
		--update-crds \
		--operator-name container-jfr-operator-bundle
endif

.PHONY: images
images: image
	sh images/build-all.sh

.PHONY: image
image: generate
	operator-sdk build --image-builder $(BUILDER) $(IMAGE_TAG)

.PHONY: bundle
bundle:
	operator-sdk bundle create \
		$(BUNDLE_STREAM):$(CSV_VERSION) \
		--image-builder $(BUILDER) \
		--directory ./deploy/olm-catalog/container-jfr-operator-bundle/$(CSV_VERSION) \
		--default-channel alpha \
		--channels alpha \
		--package container-jfr-operator-bundle

.PHONY: validate
validate:
	operator-sdk bundle validate \
		--image-builder $(BUILDER) \
		$(IMAGE_TAG)

.PHONY: index
index:
ifeq ($(INDEX_VERSION), $(PREV_INDEX_VERSION))
	opm index add \
		--bundles $(BUNDLE_STREAM):$(CSV_VERSION) \
		--tag $(INDEX_STREAM):$(INDEX_VERSION)
else
	# TODO
	# previous index version should be programatically determined
	opm index add \
		--from-index $(INDEX_STREAM):$(PREV_INDEX_VERSION) \
		--bundles $(BUNDLE_STREAM):$(CSV_VERSION) \
		--tag $(INDEX_STREAM):$(INDEX_VERSION)
endif

.PHONY: test
test: undeploy test-unit test-integration

.PHONY: test-unit
test-unit:
# Run tests with Ginkgo CLI if available
ifneq ("$(wildcard $(GINKGO))","")
	"$(GINKGO)" -v ./...
else
	go test -v ./...
endif

.PHONY: test-integration
test-integration: scorecard

.PHONY: scorecard
scorecard:
	operator-sdk scorecard

.PHONY: clean
clean:
	rm -rf build/_output




#########################################
# "Local" (ex. minishift/crc) testing targets #
#########################################

.PHONY: catalog
catalog: remove_catalog
	oc create -f deploy/olm-catalog/catalog-source.yaml

.PHONY: remove_catalog
remove_catalog:
	- oc delete -f deploy/olm-catalog/catalog-source.yaml

.PHONY: deploy
deploy: undeploy
ifeq ($(shell oc api-versions | grep -c '^cert-manager.io/v1$$'), 0)
ifneq ($(DISABLE_SERVICE_TLS), true)
	$(error cert-manager is not installed, install using "make cert_manager" or disable TLS for services by setting DISABLE_SERVICE_TLS to true)
endif
endif
	oc create -f deploy/service_account.yaml
	oc create -f deploy/role.yaml
	oc create -f deploy/role_binding.yaml
	oc create -f deploy/crds/rhjmc.redhat.com_flightrecorders_crd.yaml
	oc create -f deploy/crds/rhjmc.redhat.com_recordings_crd.yaml
	oc create -f deploy/crds/rhjmc.redhat.com_containerjfrs_crd.yaml
	sed -e 's|REPLACE_IMAGE|$(IMAGE_TAG)|g' deploy/dev_operator.yaml | oc create -f -
	oc create -f deploy/crds/rhjmc.redhat.com_v1beta1_containerjfr_cr.yaml

.PHONY: undeploy
undeploy: undeploy_sample_app undeploy_sample_app2
	- oc delete recording --all
	- oc delete flightrecorder --all
	- oc delete containerjfr --all
	- oc delete deployment container-jfr-operator
	- oc delete all -l name=container-jfr-operator
	- oc delete all -l app=containerjfr
	- oc delete persistentvolumeclaims -l app=containerjfr
	- oc delete persistentvolumes -l app=containerjfr
	- oc delete configmaps -l app=containerjfr
	- oc delete role container-jfr-operator
	- oc delete rolebinding container-jfr-operator
	- oc delete serviceaccount container-jfr-operator
	- oc delete crd flightrecorders.rhjmc.redhat.com
	- oc delete crd recordings.rhjmc.redhat.com
	- oc delete crd containerjfrs.rhjmc.redhat.com
	- oc delete -f deploy/olm-catalog/catalog-source.yaml

.PHONY: cert_manager
cert_manager:
	oc create --validate=false -f $(CERT_MANAGER_MANIFEST)

.PHONY: remove_cert_manager
remove_cert_manager:
	- oc delete -f $(CERT_MANAGER_MANIFEST)

.PHONY: sample_app
sample_app:
	oc new-app quay.io/andrewazores/vertx-fib-demo:0.1.0

.PHONY: undeploy_sample_app
undeploy_sample_app:
	- oc delete all -l app=vertx-fib-demo

.PHONY: sample_app2
sample_app2:
	oc new-app quay.io/andrewazores/container-jmx-docker-listener:0.1.0 --name=jmx-listener
	oc patch svc/jmx-listener -p '{"spec":{"$setElementOrder/ports":[{"port":7095},{"port":9092},{"port":9093}],"ports":[{"name":"jfr-jmx","port":9093}]}}'

.PHONY: undeploy_sample_app2
undeploy_sample_app2:
	- oc delete all -l app=jmx-listener
