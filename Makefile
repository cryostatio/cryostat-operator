IMAGE_TAG ?= quay.io/rh-jmc-team/container-jfr-operator:0.3.0
CRDS := containerjfr flightrecorder

.DEFAULT_GOAL := bundle

.PHONY: generate
generate: k8s openapi

.PHONY: k8s
k8s:
	operator-sdk generate k8s

.PHONY: openapi
openapi:
	operator-sdk generate openapi

.PHONY: image
image: generate
	operator-sdk build --image-builder podman $(IMAGE_TAG)

.PHONY: bundle
bundle: image copy-crds
	operator-courier verify --ui_validate_io bundle

.PHONY: copy-crds
copy-crds:
	$(foreach res, $(CRDS), cp -f deploy/crds/rhjmc.redhat.com_$(res)s_crd.yaml bundle/$(res)s.rhjmc.redhat.com.crd.yaml;)

.PHONY: test
test: undeploy scorecard

.PHONY: scorecard
scorecard: generate
	operator-sdk scorecard

.PHONY: clean
clean: clean-bundle
	rm -rf build/_output

.PHONY: clean-bundle
clean-bundle:
	rm -f bundle/*.crd.yaml




#########################################
# "Local" (ex. minishift/crc) testing targets #
#########################################

.PHONY: catalog
catalog: remove_catalog
	oc create -f bundle/openshift/operator-source.yaml

.PHONY: remove_catalog
remove_catalog:
	- oc delete -f bundle/openshift/operator-source.yaml

.PHONY: deploy
deploy: undeploy
	oc create -f deploy/service_account.yaml
	oc create -f deploy/role.yaml
	oc create -f deploy/role_binding.yaml
	oc create -f deploy/crds/rhjmc.redhat.com_flightrecorders_crd.yaml
	oc create -f deploy/crds/rhjmc.redhat.com_containerjfrs_crd.yaml
	sed -e 's|REPLACE_IMAGE|$(IMAGE_TAG)|g' deploy/dev_operator.yaml | oc create -f -
	oc set env deployment/container-jfr-operator TLS_VERIFY=false
	oc create -f deploy/crds/rhjmc.redhat.com_v1alpha1_containerjfr_cr.yaml

.PHONY: undeploy
undeploy: undeploy_sample_app undeploy_sample_app2
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
	- oc delete -f bundle/openshift/operator-source.yaml

.PHONY: sample_app
sample_app:
	oc new-app quay.io/andrewazores/vertx-fib-demo

.PHONY: undeploy_sample_app
undeploy_sample_app:
	- oc delete all -l app=vertx-fib-demo

.PHONY: sample_app2
sample_app2:
	oc new-app andrewazores/container-jmx-docker-listener:latest --name=jmx-listener
	oc patch svc/jmx-listener -p '{"spec":{"$setElementOrder/ports":[{"port":7095},{"port":9092},{"port":9093}],"ports":[{"name":"jfr-jmx","port":9093}]}}'

.PHONY: undeploy_sample_app2
undeploy_sample_app2:
	- oc delete all -l app=jmx-listener
