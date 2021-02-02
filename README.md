# container-jfr-operator

A Kubernetes Operator to automate deployment of
[Container-JFR](https://github.com/rh-jmc-team/container-jfr) and provide an
API to manage [JDK Flight Recordings](https://openjdk.java.net/jeps/328).

# Using
Once deployed, the `containerjfr` instance can be accessed via web browser
at the URL provided by:
```
oc get pod -l kind=containerjfr -o jsonpath='https://{$.items[0].metadata.annotations.redhat\.com/containerJfrUrl} '
```
The Grafana credentials can be obtained with:
```shell
CJFR_NAME=$(oc get containerjfr -o jsonpath='{$.items[0].metadata.name}')
# Username
oc get secret ${CJFR_NAME}-grafana-basic -o jsonpath='{$.data.GF_SECURITY_ADMIN_USER}' | base64 -d
# Password
oc get secret ${CJFR_NAME}-grafana-basic -o jsonpath='{$.data.GF_SECURITY_ADMIN_PASSWORD}' | base64 -d
```
The JMX authentication credentials for Container JFR itself can be obtained with:
```shell
CJFR_NAME=$(oc get containerjfr -o jsonpath='{$.items[0].metadata.name}')
# Username
oc get secret ${CJFR_NAME}-jmx-auth -o jsonpath='{$.data.CONTAINER_JFR_RJMX_USER}' | base64 -d
# Password
oc get secret ${CJFR_NAME}-jmx-auth -o jsonpath='{$.data.CONTAINER_JFR_RJMX_PASS}' | base64 -d
```

# Building
## Requirements
- `go` v1.13
- [`operator-sdk`](https://github.com/operator-framework/operator-sdk) v0.19.4
- [`opm`](https://github.com/operator-framework/operator-registry)
- [`cert-manager`](https://github.com/jetstack/cert-manager) v1.0.2 (Recommended)
- `podman`
- `ginkgo` (Optional)

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

## Security

By default, the operator expects cert-manager to be available in the cluster.
This allows the operator to deploy Container JFR with all communication
between its internal services done over HTTPS. If you wish to disable this
feature and not use cert-manager, you can set the environment variable
`DISABLE_SERVICE_TLS=true` when you deploy the operator. We provide
`make cert_manager` and `make remove_cert_manager` targets to easily
install/remove cert-manager from your cluster.


### User Authentication

Users can use `oc whoami --show-token` to retrieve their OpenShift OAuth token
for the currently logged in user account. This token can be used when directly
interacting with the deployed ContainerJFR instance(s), for example on the
web-client login page.

If the current user account does not have sufficient permissions to list
routes, list endpoints, or perform other actions that ContainerJFR requires,
then the user may also try to authenticate using the Operator's service
account. This, of course, assumes that the user has permission to view this
service account's secrets.

`oc get secrets | grep container-jfr-operator-token` will provide at least one
such operator service account token secret name which can be used - for
example, `container-jfr-operator-token-m5rmq`. The token can then be retrieved
for use in authenticating as the operator service account:

```
$ oc describe secret container-jfr-operator-token-m5rmq
Name:         container-jfr-operator-token-m5rmq
Namespace:    default
Labels:       <none>
Annotations:  kubernetes.io/service-account.name: container-jfr-operator
              kubernetes.io/service-account.uid: 18586e70-3e24-11eb-85c4-525400bae188

Type:  kubernetes.io/service-account-token

Data
====
token:           eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImNvbnRhaW5lci1qZnItb3BlcmF0b3ItdG9rZW4tbTVybXEiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiY29udGFpbmVyLWpmci1vcGVyYXRvciIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6IjE4NTg2ZTcwLTNlMjQtMTFlYi04NWM0LTUyNTQwMGJhZTE4OCIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmNvbnRhaW5lci1qZnItb3BlcmF0b3IifQ.ZaEkAxYq_2Sx9-kDZEI9x3VK3GUe9hY3oHDLvmAIGbHcIG5dtDnFytmugw-riVCra5wvl4-F5yGAp3F-h5FZcjMCyI9a7JUCOJ0_YdIxS1Gn5w3gcJj6nw0qRyNM20FjsnNVNbhhHOU5YL1kbYctAqZzs2HfpEhjMYzSqimrLLwkpg6llGmdq0IqkHoFyBXxJPRgVexpsyM1CDz2CvPxtDP3-B6plmgiLov1rWtIfUykGk0B1PCsagqlm3csvqCdzCRvnRxgEFibwwsUKFFM3smOoj829g5KVezaZIc5YURHxRvRvKhW-h1GevhhdvJKi5Qyebst0MbG-Fwh07nB7g
ca.crt:          1070 bytes
namespace:       7 bytes
service-ca.crt:  2186 bytes
```

or more briefly:

```
$ oc get -o jsonpath='{.data.token}' secret container-jfr-operator-token-m5rmq | base64 -d
eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImNvbnRhaW5lci1qZnItb3BlcmF0b3ItdG9rZW4tbTVybXEiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiY29udGFpbmVyLWpmci1vcGVyYXRvciIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6IjE4NTg2ZTcwLTNlMjQtMTFlYi04NWM0LTUyNTQwMGJhZTE4OCIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmNvbnRhaW5lci1qZnItb3BlcmF0b3IifQ.ZaEkAxYq_2Sx9-kDZEI9x3VK3GUe9hY3oHDLvmAIGbHcIG5dtDnFytmugw-riVCra5wvl4-F5yGAp3F-h5FZcjMCyI9a7JUCOJ0_YdIxS1Gn5w3gcJj6nw0qRyNM20FjsnNVNbhhHOU5YL1kbYctAqZzs2HfpEhjMYzSqimrLLwkpg6llGmdq0IqkHoFyBXxJPRgVexpsyM1CDz2CvPxtDP3-B6plmgiLov1rWtIfUykGk0B1PCsagqlm3csvqCdzCRvnRxgEFibwwsUKFFM3smOoj829g5KVezaZIc5YURHxRvRvKhW-h1GevhhdvJKi5Qyebst0MbG-Fwh07nB7g
```

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
`make test` will run all automated tests listed below. `make test-unit` will
run only the operator's unit tests using `ginkgo` if installed, or `go test` if
not. `make test-integration` and `make scorecard` are currently synonyms and
will run only the Operator SDK's scorecard test suite. This requires an
OpenShift 4 cluster to be available and logged in with your `oc` OpenShift
Client. The recommended setup for development testing is CodeReady Containers
(`crc`).

Before the scorecard tests are run, all container-jfr and container-jfr-operator
resources will be deleted to ensure a clean slate.
