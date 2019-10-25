# container-jfr-operator

A Kubernetes Operator to automate deployment of [Container-JFR](https://github.com/rh-jmc-team/container-jfr) and provide an API to manage [JDK Flight Recordings](https://openjdk.java.net/jeps/328).

# Using
Once deployed, the `containerjfr` instance can be accessed via web browser
at the URL provided by
`oc get pod -l kind=containerjfr -o jsonpath='http://{$.items[0].metadata.annotations.redhat\.com/containerJfrUrl} '`

# Building
`make image` will create an OCI image to the local registry, tagged as
`quay.io/rh-jmc-team/container-jfr-operator`. This tag can be overridden by
setting the environment variable `IMAGE_TAG`.

To (re)build the Operator bundle for packaging in catalogs (ex. OperatorHub),
use [operator-courier](https://github.com/operator-framework/operator-courier).
eg. `operator-courer push bundle rh-jmc-team container-jfr-operator-bundle $VERSION $TOKEN`
, where `$VERSION` is the version string matching that within the bundle
definition YAMLs, and `$TOKEN` has been replaced by your personal quay.io
access token.

To make this bundle available within the OpenShift Console's Operator Catalog,
add the custom Operator Source via `oc apply -f bundle/openshift/operator-source.yaml`

# Setup / Deployment
`make deploy` will deploy the operator using `oc` to whichever OpenShift
cluster is configured with the local client. This also respects the
`IMAGE_TAG` environment variable, so that different versions of the operator
can be easily deployed.

# Development
An invocation like
`export IMAGE_TAG=quay.io/some-user/container-jfr-operator`
`make clean image && docker image push $IMAGE_TAG && make deploy`
is handy for local development testing using ex. Minishift.
