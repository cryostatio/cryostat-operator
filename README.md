# container-jfr-operator

A Kubernetes Operator to automate deployment of
[Container-JFR](https://github.com/rh-jmc-team/container-jfr) and provide an
API to manage [JDK Flight Recordings](https://openjdk.java.net/jeps/328).

# Using
Once deployed, the `containerjfr` instance can be accessed via web browser
at the URL provided by
`oc get pod -l kind=containerjfr -o jsonpath='http://{$.items[0].metadata.annotations.redhat\.com/containerJfrUrl} '`.
The Grafana credentials can be obtained with:
`oc get secret containerjfr-grafana-basic -o jsonpath='{$.data.GF_SECURITY_ADMIN_USER}' | base64 -d`
and
`oc get secret containerjfr-grafana-basic -o jsonpath='{$.data.GF_SECURITY_ADMIN_PASSWORD}' | base64 -d`.

# Building
## Requirements
- `go` v1.13
- [`operator-sdk`](https://github.com/operator-framework/operator-sdk) v0.15.2
- [`opm`](https://github.com/operator-framework/operator-registry)
- `podman`

## Instructions
Operator SDK requires the `GOROOT` environment variable to be set to the root
of your Go installation. In Bash, this can be done with
`export GOROOT="$(go env GOROOT)"`.

`make image` will create an OCI image to the local registry, tagged as
`quay.io/rh-jmc-team/container-jfr-operator`. This tag can be overridden by
setting the environment variable `IMAGE_TAG`.

To create a CSV bundle, use `CSV_VERSION=1.2.3 make csv`. This will generate
a CSV and CRDs for a bundle versioned 1.2.3 at
`./deploy/olm-catalog/container-jfr-operator-bundle/1.2.3`.

To (re)build the Operator bundle for packaging in catalogs (ex. OperatorHub),
`CSV_VERSION=1.2.3 make bundle`. This will create an Operator bundle image
containing the required YAML manifest files and metadata for publishing the
bundle, based on the manifest files at
`./deploy/olm-catalog/container-jfr-operator-bundle/1.2.3`. Push the resulting
bundle image to an image registry such as quay.io.

To create an index of these bundles, use `make index`. Push the index to an
image registry such as quay.io.

# Setup / Deployment
## UI-Guided Deployment

The operator can be deployed using the Operator Marketplace in the graphical
OpenShift console by first adding a custom catalog source. A YAML definition
of such a custom source is at `deploy/olm-catalog/catalog-source.yaml`, which
can be added to your cluster by
`oc create -f deploy/olm-catalog/catalog-source.yaml` or simply `make catalog`.
This method allows the latest released version of the operator to be installed
into the cluster with the same method and in the same form that end users would
receive it.

### Configuration

Once deployed, the operator deployment will be active in the cluster, but no
ContainerJFR instance will be created. To trigger its creation, add a
ContainerJFR CR using the UI for operator "provided APIs". The `spec` field
of the CR must contain the boolean property `minimal: true|false`. A full
deployment includes ContainerJFR itself and its web client assets, as well as
jfr-datasource and Grafana containers within the pod. A minimal deployment
uses a ContainerJFR image with the web client assets removed and excludes the
jfr-datasource and Grafana containers, resulting in a ContainerJFR deployment
that can only be used headlessly and which consumes as few cluster resources as
possible.

## Manual Deployment

`make deploy` will deploy the operator using `oc` to whichever OpenShift
cluster is configured with the local client. This also respects the
`IMAGE_TAG` environment variable, so that different versions of the operator
can be easily deployed.

# Development
An invocation like
`export IMAGE_TAG=quay.io/some-user/container-jfr-operator`
`make clean image && podman image push $IMAGE_TAG && make deploy`
is handy for local development testing using ex. CodeReady Containers.

# Testing
## Requirements
- [oc](https://www.okd.io/download.html)
- (optional) [crc](https://github.com/code-ready/crc)

## Instructions
`make test` will run the automated tests. This requires an OpenShift 4 cluster
to be available and logged in with your `oc` OpenShift Client. The recommended
setup for development testing is CodeReady Containers (`crc`).

Before the tests are run, all container-jfr and container-jfr-operator
resources will be deleted to ensure a clean slate.
