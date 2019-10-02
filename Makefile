IMAGE_TAG ?= quay.io/rh-jmc-team/container-jfr-operator:0.1.0

.DEFAULT_GOAL := image

.PHONY: compile
compile:
	operator-sdk generate k8s

.PHONY: image
image: compile
	operator-sdk build $(IMAGE_TAG)

.PHONY: clean
clean:
	rm -rf build/_output

.PHONY: deploy
deploy: undeploy
	oc create -f deploy/service_account.yaml
	oc create -f deploy/role.yaml
	oc create -f deploy/role_binding.yaml
	oc create -f deploy/crds/rhjmc_v1alpha1_flightrecorder_crd.yaml
	oc create -f deploy/crds/rhjmc_v1alpha1_containerjfr_crd.yaml
	oc create -f deploy/containerjfr_grafana_config_map.yaml
	oc create -f deploy/containerjfr_command_config_map.yaml
	oc create -f deploy/containerjfr_config_map.yaml
	sed -e 's|REPLACE_IMAGE|$(IMAGE_TAG)|g' deploy/operator.yaml | oc create -f -
	oc create -f deploy/crds/rhjmc_v1alpha1_containerjfr_cr.yaml
	oc create -f deploy/exposecontroller_config_map.yaml
	sed -e 's|REPLACE_PROJECT|$(shell oc project -q)|g' deploy/exposecontroller.yaml | oc create -f -

.PHONY: undeploy
undeploy:
	- oc delete deployment exposecontroller
	- oc delete configmap exposecontroller
	- oc delete all -l project=exposecontroller
	- oc delete routes -l generator=exposecontroller
	- oc delete deployment container-jfr-operator
	- oc delete containerjfr --all
	- oc delete flightrecorder --all
	- oc delete all -l name=container-jfr-operator
	- oc delete all -l app=containerjfr
	- oc delete configmaps -l app=containerjfr
	- oc delete role container-jfr-operator
	- oc delete rolebinding container-jfr-operator
	- oc delete serviceaccount container-jfr-operator
	- oc delete crd flightrecorders.rhjmc.redhat.com
	- oc delete crd containerjfrs.rhjmc.redhat.com
