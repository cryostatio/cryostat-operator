IMAGE_TAG ?= quay.io/rh-jmc-team/container-jfr-operator:0.1.1

.DEFAULT_GOAL := image

CRD_DIR=deploy/crds
CRDS=$(patsubst $(CRD_DIR)/%,%,$(wildcard $(CRD_DIR)/*.crd.yaml))

.PHONY: compile
compile:
	operator-sdk generate k8s

.PHONY: image
image: compile
	operator-sdk build $(IMAGE_TAG)

.PHONY: bundle
bundle: image $(CRDS)
	@echo "Bundle prepared, use operator-courier to push it to Quay"

%.crd.yaml:
	cp -f $(CRD_DIR)/$@ bundle/

.PHONY: clean
clean: clean-bundle
	rm -rf build/_output

.PHONY: clean-bundle
clean-bundle:
	rm -f bundle/$(CRDS)




#########################################
# "Local" (ex. MiniShift) testing targets #
#########################################

.PHONY: deploy
deploy: undeploy
	oc create -f deploy/operator_service_account.yaml
	oc create -f deploy/operator_role.yaml
	oc create -f deploy/operator_role_binding.yaml
	oc create -f deploy/crds/flightrecorders.rhjmc.redhat.com.crd.yaml
	oc create -f deploy/crds/containerjfrs.rhjmc.redhat.com.crd.yaml
	sed -e 's|REPLACE_IMAGE|$(IMAGE_TAG)|g' deploy/dev_operator.yaml | oc create -f -
	oc create -f deploy/crds/rhjmc_v1alpha1_containerjfr_cr.yaml

.PHONY: undeploy
undeploy: undeploy_sample_app
	- oc delete deployment container-jfr-operator
	- oc delete containerjfr --all
	- oc delete flightrecorder --all
	- oc delete all -l name=container-jfr-operator
	- oc delete all -l app=containerjfr
	- oc delete persistentvolumeclaims -l app=containerjfr
	- oc delete persistentvolumes -l app=containerjfr
	- oc delete configmaps -l app=containerjfr
	- oc delete role container-jfr-operator
	- oc delete rolebinding container-jfr-operator
	- oc delete serviceaccount container-jfr-operator
	- oc delete crd flightrecorders.rhjmc.redhat.com
	- oc delete crd containerjfrs.rhjmc.redhat.com

.PHONY: sample_app
sample_app:
	oc new-app andrewazores/container-jmx-docker-listener:latest --name=jmx-listener

.PHONY: undeploy_sample_app
undeploy_sample_app:
	- oc delete all -l app=jmx-listener
