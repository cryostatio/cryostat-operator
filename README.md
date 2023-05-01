# cryostat-operator

[![CI build](https://github.com/cryostatio/cryostat-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/cryostatio/cryostat-operator/actions/workflows/ci.yaml)
[![Google Group : Cryostat Development](https://img.shields.io/badge/Google%20Group-Cryostat%20Development-blue.svg)](https://groups.google.com/g/cryostat-development)

A Kubernetes Operator to automate deployment of
[Cryostat](https://github.com/cryostatio/cryostat).
## SEE ALSO

* [cryostat-core](https://github.com/cryostatio/cryostat-core) for
the core library providing a convenience wrapper and headless stubs for use of
JFR using JDK Mission Control internals.

* [cryostat](https://github.com/cryostatio/cryostat) for the main API
backend to detect JVMs and manage JFR.

* [cryostat-web](https://github.com/cryostatio/cryostat-web) for the React
graphical frontend included as a submodule in Cryostat and built into
Cryostat's OCI images.

* [jfr-datasource](https://github.com/cryostatio/jfr-datasource) for
the JFR datasource for Grafana.

* [cryostat-grafana-dashboard](https://github.com/cryostatio/cryostat-grafana-dashboard)
for the Grafana dashboard.

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

# Building
## Requirements
- `go` v1.20
- [`operator-sdk`](https://github.com/operator-framework/operator-sdk) v1.28.0
- [`cert-manager`](https://github.com/jetstack/cert-manager) v1.7.1+ (Recommended)
- `podman` or `docker`
- [`jq`](https://stedolan.github.io/jq/) v1.6+
- `ginkgo` (Optional)

## Instructions
`make generate manifests manager` will trigger code/YAML generation and compile
the operator controller manager, along with running some code quality checks.

`make oci-build` will build an OCI image from the generated YAML and compiled
binary to the local registry, tagged as
`quay.io/crystatio/cryostat-operator`. This tag can be overridden by
setting the environment variables `IMAGE_NAMESPACE` and `OPERATOR_NAME`.
`IMAGE_VERSION` can also be set to override the tagged version.

`make bundle` will create an OLM bundle. This will generate a CSV, CRDs and
other manifests, and other required configurations for an OLM bundle versioned
with version `$IMAGE_VERSION` in the `bundle/` directory. `make bundle-build`
will create an OCI image of this bundle, which can then be pushed to an image
repository such as `quay.io`.

`make catalog-build` will build an OCI image of the operator catalog (i.e. index)
with version `$IMAGE_VERSION` that includes the bundle image of the same version.

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

Users can use `oc whoami --show-token | base64` to retrieve their encoded OpenShift OAuth token
for the currently logged in user account. This encoded token can be used when directly
interacting with the deployed Cryostat instance(s).

When using the web-client, users can login with their username and password associated with their OpenShift account. User credentials will be remembered for the duration of the session.

If the current user account does not have sufficient permissions to list
routes, list endpoints, or perform other actions that Cryostat requires,
then the user may also try to authenticate using the Operator's service
account. This, of course, assumes that the user has permission to view this
service account's secrets.

`oc get secrets | grep cryostat-operator-service-account-token` will provide at least one
such operator service account token secret name which can be used - for
example, `cryostat-operator-service-account-token-7tt7l`. The token can then be retrieved
for use in authenticating as the operator service account:

```
$ oc describe secret cryostat-operator-service-account-token-7tt7l
Name:         cryostat-operator-service-account-token-7tt7l
Namespace:    default
Labels:       <none>
Annotations:  kubernetes.io/created-by: openshift.io/create-dockercfg-secrets
              kubernetes.io/service-account.name: cryostat-operator-service-account
              kubernetes.io/service-account.uid: 692aa8c7-081e-4a51-9355-be3eaa8f9fa6

Type:  kubernetes.io/service-account-token

Data
====
ca.crt:          7209 bytes
namespace:       7 bytes
service-ca.crt:  8422 bytes
token:           eyJhbGciOiJSUzI1NiIsImtpZCI6IkhYZC13eDdGVGwyQzdGNVpZVndScEZ2VmRxWTlzbnBUUG9HRkJpejJkV3cifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImNyeW9zdGF0LW9wZXJhdG9yLXNlcnZpY2UtYWNjb3VudC10b2tlbi03dHQ3bCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJjcnlvc3RhdC1vcGVyYXRvci1zZXJ2aWNlLWFjY291bnQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI2OTJhYThjNy0wODFlLTRhNTEtOTM1NS1iZTNlYWE4ZjlmYTYiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpjcnlvc3RhdC1vcGVyYXRvci1zZXJ2aWNlLWFjY291bnQifQ.M7C1V0bN3aILBflO7TTOTikw7wLGRJ79-OkCDQIZbu71QLdX05jyCxxtlH32lr8jz6HwxfXXweh3ifG_2lbe7_TbM8jxmBoMdLuc4Q_akpmA-GQuDPrRxfHGJApYGQ6CVug3KHSrQwj2M4QrSUz7FoeQGaOH9BnWj1TrHGmOZUPJ6u7JSu2OwoLBda6rF-M4Bl72DmkyMAzikreRgPEk4D7gTCY0yNvsQDuUAwpFwmEukRC2WyTAVTpKPgThZUk-UJ-dXufbhAcqIRt6jeCQ19_Bo0zXc_ELgQydxuTack1ndT3HwRmwwNuZDFv-G3Y0YdjfRh00DqEvSn9ynZzwueDCJUxlHdznytfUWk9PA712JENpFC7b-zSHnjymIcFeUd8s_Zq_-JKrDIPnH0oZDRO_MUpKEC7Jz_8SeFJHLLGfBZt_aP4VwQHEUThiFQPwrfbd8tppUG2TKcekPScKcauy-BCI52odBzapP6meilMQVrmRtu7i30L05vgQiST_OsmSP8CuKW13a-leCCtN_aNQGqlWvLhP81H95ui-PvMzwMIDlfDZ03ycuYg4R4eUG3nUq7-42wrSdFLo8gm9wsl7y1ZRMQwHR1DCVBbHYS0iFOcmwto2Ejlrgvn3Cs0pDS7pDVoFkH2FsTopEw3jXtnkMs15mSmBnHz-UjF-l08
```

or more briefly:

```
$ oc get -o jsonpath='{.data.token}' secret cryostat-operator-service-account-token-7tt7l | base64 -d
eyJhbGciOiJSUzI1NiIsImtpZCI6IkhYZC13eDdGVGwyQzdGNVpZVndScEZ2VmRxWTlzbnBUUG9HRkJpejJkV3cifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJkZWZhdWx0Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZWNyZXQubmFtZSI6ImNyeW9zdGF0LW9wZXJhdG9yLXNlcnZpY2UtYWNjb3VudC10b2tlbi03dHQ3bCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJjcnlvc3RhdC1vcGVyYXRvci1zZXJ2aWNlLWFjY291bnQiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlcnZpY2UtYWNjb3VudC51aWQiOiI2OTJhYThjNy0wODFlLTRhNTEtOTM1NS1iZTNlYWE4ZjlmYTYiLCJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDpjcnlvc3RhdC1vcGVyYXRvci1zZXJ2aWNlLWFjY291bnQifQ.M7C1V0bN3aILBflO7TTOTikw7wLGRJ79-OkCDQIZbu71QLdX05jyCxxtlH32lr8jz6HwxfXXweh3ifG_2lbe7_TbM8jxmBoMdLuc4Q_akpmA-GQuDPrRxfHGJApYGQ6CVug3KHSrQwj2M4QrSUz7FoeQGaOH9BnWj1TrHGmOZUPJ6u7JSu2OwoLBda6rF-M4Bl72DmkyMAzikreRgPEk4D7gTCY0yNvsQDuUAwpFwmEukRC2WyTAVTpKPgThZUk-UJ-dXufbhAcqIRt6jeCQ19_Bo0zXc_ELgQydxuTack1ndT3HwRmwwNuZDFv-G3Y0YdjfRh00DqEvSn9ynZzwueDCJUxlHdznytfUWk9PA712JENpFC7b-zSHnjymIcFeUd8s_Zq_-JKrDIPnH0oZDRO_MUpKEC7Jz_8SeFJHLLGfBZt_aP4VwQHEUThiFQPwrfbd8tppUG2TKcekPScKcauy-BCI52odBzapP6meilMQVrmRtu7i30L05vgQiST_OsmSP8CuKW13a-leCCtN_aNQGqlWvLhP81H95ui-PvMzwMIDlfDZ03ycuYg4R4eUG3nUq7-42wrSdFLo8gm9wsl7y1ZRMQwHR1DCVBbHYS0iFOcmwto2Ejlrgvn3Cs0pDS7pDVoFkH2FsTopEw3jXtnkMs15mSmBnHz-UjF-l08
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
