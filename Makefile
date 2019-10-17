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
	oc create -f deploy/operator_service_account.yaml
	oc create -f deploy/exposecontroller_service_account.yaml
	oc create -f deploy/operator_role.yaml
	oc create -f deploy/exposecontroller_role.yaml
	oc create -f deploy/operator_role_binding.yaml
	oc create -f deploy/exposecontroller_role_binding.yaml
	oc create -f deploy/exposecontroller_role_binding_cluster_admin.yaml
	oc create -f deploy/exposecontroller_role_binding_cluster_reader.yaml
	oc create -f deploy/crds/rhjmc_v1alpha1_flightrecorder_crd.yaml
	oc create -f deploy/crds/rhjmc_v1alpha1_containerjfr_crd.yaml
	sed -e 's|REPLACE_IMAGE|$(IMAGE_TAG)|g' deploy/dev_operator.yaml | oc create -f -
	oc create -f deploy/crds/rhjmc_v1alpha1_containerjfr_cr.yaml
	oc create -f deploy/exposecontroller.yaml

.PHONY: undeploy
undeploy: undeploy_sample_app
	- oc delete deployment exposecontroller
	- oc delete all -l project=exposecontroller
	- oc delete routes -l generator=exposecontroller
	- oc delete deployment container-jfr-operator
	- oc delete containerjfr --all
	- oc delete flightrecorder --all
	- oc delete all -l name=container-jfr-operator
	- oc delete all -l app=containerjfr
	- oc delete persistentvolumeclaims -l app=containerjfr
	- oc delete persistentvolumes -l app=containerjfr
	- oc delete configmaps -l app=containerjfr
	- oc delete role container-jfr-operator
	- oc delete role exposecontroller
	- oc delete rolebinding container-jfr-operator
	- oc delete rolebinding exposecontroller
	- oc delete clusterrolebinding exposecontroller-cluster-admin
	- oc delete clusterrolebinding serviceaccounts-cluster-reader
	- oc delete serviceaccount container-jfr-operator
	- oc delete serviceaccount exposecontroller
	- oc delete crd flightrecorders.rhjmc.redhat.com
	- oc delete crd containerjfrs.rhjmc.redhat.com

.PHONY: sample_app
sample_app:
	oc new-app andrewazores/container-jmx-docker-listener:latest --name=jmx-listener

.PHONY: undeploy_sample_app
undeploy_sample_app:
	- oc delete all -l app=jmx-listener
