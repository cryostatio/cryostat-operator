# cryostat-operator

[![CI build](https://github.com/cryostatio/cryostat-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/cryostatio/cryostat-operator/actions/workflows/ci.yaml)

A Kubernetes Operator to automate deployment of
[Cryostat](https://github.com/cryostatio/cryostat) and provide an
API to manage [JDK Flight Recordings](https://openjdk.java.net/jeps/328).

# Using
Once deployed, the `cryostat` instance can be accessed via web browser
at the URL provided by:
```
kubectl get cryostat -o jsonpath='{$.items[0].status.applicationUrl}'
```
The Grafana credentials can be obtained with:
```shell
CRYOSTAT_NAME=$(kubectl get cryostat -o jsonpath='{$.items[0].metadata.name}')
# Username
kubectl get secret ${CRYOSTAT_NAME}-grafana-basic -o jsonpath='{$.data.GF_SECURITY_ADMIN_USER}' | base64 -d
# Password
kubectl get secret ${CRYOSTAT_NAME}-grafana-basic -o jsonpath='{$.data.GF_SECURITY_ADMIN_PASSWORD}' | base64 -d
```
The JMX authentication credentials for Cryostat itself can be obtained with:
```shell
CRYOSTAT_NAME=$(kubectl get cryostat -o jsonpath='{$.items[0].metadata.name}')
# Username
kubectl get secret ${CRYOSTAT_NAME}-jmx-auth -o jsonpath='{$.data.CRYOSTAT_RJMX_USER}' | base64 -d
# Password
kubectl get secret ${CRYOSTAT_NAME}-jmx-auth -o jsonpath='{$.data.CRYOSTAT_RJMX_PASS}' | base64 -d
```

## Kubernetes API
The operator provides a Kubernetes API to directly create and manage Flight Recordings.
See [Kubernetes API Overview](docs/api.md) for more details.

# Building
## Requirements
- `go` v1.15
- [`operator-sdk`](https://github.com/operator-framework/operator-sdk) v1.5.0
- [`cert-manager`](https://github.com/jetstack/cert-manager) v1.1.0 (Recommended)
- `podman` or `docker`
- `ginkgo` (Optional)

## Instructions
`make generate manifests manager` will trigger code/YAML generation and compile
the operator controller manager, along with running some code quality checks.

`make oci-build` will build an OCI image from the generated YAML and compiled
binary to the local registry, tagged as
`quay.io/crystatio/cryostat-operator`. This tag can be overridden by
setting the environment variables `IMAGE_NAMESPACE` and `OPERATOR_NAME`.
`IMAGE_VERSION` can also be set to override the tagged version.

To create an OLM bundle, use `make bundle`. This will generate a CSV, CRDs and
other manifests, and other required configurations for an OLM bundle versioned
with version `$IMAGE_VERSION` in the `bundle/` directory. `make bundle-build`
will create an OCI image of this bundle, which can then be pushed to an image
repository such as `quay.io`.

# Setup / Deployment
## Bundle Deployment

The operator can be deployed using OLM using `make deploy_bundle`. This will
deploy `quay.io/cryostat/cryostat-operator-bundle:$IMAGE_VERSION` to
your configured cluster using `oc` or `kubectl` (`kubeconfig`). You can set the
variables `IMAGE_NAMESPACE` or `IMAGE_VERSION` to deploy different builds of
the bundle. Once this is complete, the Cryostat Operator will be deployed
and running in your cluster.

### Configuration

Once deployed, the operator deployment will be active in the cluster, but no
Cryostat instance will be created. To trigger its creation, add a
Cryostat CR using the UI for operator "provided APIs". Full details on the
configuration options in the Cryostat CRD can be found in
[Configuring Cryostat](docs/config.md). When running on Kubernetes, see
[Network Options](docs/config.md#network-options) for additional
mandatory configuration in order to access Cryostat outside of the cluster.

For convenience, a full deployment can be created using
`kubectl create -f config/samples/operator_v1beta1_cryostat.yaml`, or more
simply, `make create_cryostat_cr`.

The container images used by the operator for the core application,
jfr-datasource, and the Grafana dashboard can be overridden by setting the
`RELATED_IMAGE_CORE`, `RELATED_IMAGE_DATASOURCE`, and `RELATED_IMAGE_GRAFANA`
environment variables, respectively, in the operator deployment.

## Security

By default, the operator expects cert-manager to be available in the cluster.
This allows the operator to deploy Cryostat with all communication
between its internal services done over HTTPS. If you wish to disable this
feature and not use cert-manager, you can set the environment variable
`DISABLE_SERVICE_TLS=true` when you deploy the operator. We provide
`make cert_manager` and `make remove_cert_manager` targets to easily
install/remove cert-manager from your cluster.


### User Authentication

Users can use `oc whoami --show-token` to retrieve their OpenShift OAuth token
for the currently logged in user account. This token can be used when directly
interacting with the deployed Cryostat instance(s), for example on the
web-client login page.

If the current user account does not have sufficient permissions to list
routes, list endpoints, or perform other actions that Cryostat requires,
then the user may also try to authenticate using the Operator's service
account. This, of course, assumes that the user has permission to view this
service account's secrets.

`oc get secrets | grep cryostat-operator-token` will provide at least one
such operator service account token secret name which can be used - for
example, `cryostat-operator-token-m5rmq`. The token can then be retrieved
for use in authenticating as the operator service account:

```
$ oc describe secret cryostat-operator-token-m5rmq
Name:         cryostat-operator-token-m5rmq
Namespace:    default
Labels:       <none>
Annotations:  kubernetes.io/service-account.name: cryostat-operator
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
$ oc get -o jsonpath='{.data.token}' secret cryostat-operator-token-m5rmq | base64 -d
eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImNvbnRhaW5lci1qZnItb3BlcmF0b3ItdG9rZW4tbTVybXEiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC5uYW1lIjoiY29udGFpbmVyLWpmci1vcGVyYXRvciIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50LnVpZCI6IjE4NTg2ZTcwLTNlMjQtMTFlYi04NWM0LTUyNTQwMGJhZTE4OCIsInN1YiI6InN5c3RlbTpzZXJ2aWNlYWNjb3VudDpkZWZhdWx0OmNvbnRhaW5lci1qZnItb3BlcmF0b3IifQ.ZaEkAxYq_2Sx9-kDZEI9x3VK3GUe9hY3oHDLvmAIGbHcIG5dtDnFytmugw-riVCra5wvl4-F5yGAp3F-h5FZcjMCyI9a7JUCOJ0_YdIxS1Gn5w3gcJj6nw0qRyNM20FjsnNVNbhhHOU5YL1kbYctAqZzs2HfpEhjMYzSqimrLLwkpg6llGmdq0IqkHoFyBXxJPRgVexpsyM1CDz2CvPxtDP3-B6plmgiLov1rWtIfUykGk0B1PCsagqlm3csvqCdzCRvnRxgEFibwwsUKFFM3smOoj829g5KVezaZIc5YURHxRvRvKhW-h1GevhhdvJKi5Qyebst0MbG-Fwh07nB7g
```

## Manual Deployment

`make install` will create CustomResourceDefinitions and do other setup
required to prepare the cluster for deploying the operator, using `oc` or
`kubectl` on whichever OpenShift cluster is configured with the local client.
`make uninstall` destroys the CRDs and undoes the setup.

`make deploy` will deploy the operator in the default namespace
(`cryostat-operator-system`). `make DEPLOY_NAMESPACE=foo-namespace deploy`
can be used to deploy to an arbitrary namespace named `foo-namespace`. For
a convenient shorthand, use
`make DEPLOY_NAMESPACE=$(kubectl config view --minify -o 'jsonpath={.contexts[0].context.namespace}') deploy`
to deploy to the currently active OpenShift project namespace.
`make undeploy` will likewise remove the operator, and also uses the
`DEPLOY_NAMESPACE` variable.
This also respects the `IMAGE_TAG` environment variable, so that different
versions of the operator can be easily deployed.

`make run` can be used to run the operator controller manager as a process on
your local development machine and observing/interacting with your cluster.
This may be useful in some development scenarios, however in this case the
operator process will not have access to certain in-cluster resources such as
environment variables or service account token files.

# Development
An invocation like
`export IMAGE_NAMESPACE=quay.io/some-user` `export IMAGE_VERSION=test-version`
`make generate manifests manager oci-build bundle bundle-build`
`podman image prune -f && podman push $IMAGE_NAMESPACE/cryostat-operator:$IMAGE_VERSION && podman push $IMAGE_NAMESPACE/cryostat-operator-bundle:$IMAGE_VERSION`
`make deploy_bundle`
is handy for local development testing using ex. CodeReady Containers. This
exercises a similar build and deployment path as what end users using OLM and
OperatorHub will eventually receive.

# Testing
## Requirements
- (optional) [oc](https://www.okd.io/download.html)
- (optional) [crc](https://github.com/code-ready/crc)

## Instructions
`make test-envtest` will run controller tests using ginkgo if installed, or go test if
not, requiring no cluster connection.

`make test-scorecard` will run the Operator SDK's scorecard test suite. This requires a
Kubernetes or OpenShift cluster to be available and logged in with your `kubectl` or `oc`
client. The recommended setup for development testing is CodeReady Containers (`crc`).

Before the scorecard tests are run, all cryostat and cryostat-operator
resources will be deleted to ensure a clean slate.

`make test` will run all tests.
