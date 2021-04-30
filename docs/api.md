# Kubernetes API Overview

This operator provides a Kubernetes API to interact with [Cryostat](https://github.com/cryostatio/cryostat).
This API comes in the form of the `FlightRecorders` and `Recordings` Custom Resource Definitions, and allows you to create, list, delete, and download recordings from a Kubernetes cluster.

## Retrieving `FlightRecorder` objects
You can use `FlightRecorders` like any other built-in resource on the command line with kubectl/oc.

```shell
$ kubectl get flightrecorders
NAME                               AGE
cryostat-sample-6d8dcf5c9f-w5lgq   2m
jmx-listener-55d48f7cfc-8nkln      110s
```

`FlightRecorder` objects are created by the operator whenever a new Cryostat-compatible service is detected.
Services that expose a port named `jfr-jmx` are considered compatible. The number of this port is stored in the `status.port` property for use by the operator. Each `FlightRecorder` object maps one-to-one with a Kubernetes service. This service is stored in the `status.target` property of the `FlightRecorder` object. When the operator learns of a new `FlightRecorder` object, it queries Cryostat for a list of all available JFR events for the JVM behind the `FlightRecorder's` service. The details of these event types are stored in the `status.events` property of the `FlightRecorder`. The `spec.recordingSelector` property provides an association of `Recordings` (outlined below) with this `FlightRecorder` object. The operator also queries Cryostat for a list of known Recording Templates provided by the JVM, and any built-in or user-specified templates registered with Cryostat. These are listed in `status.templates` property.

```shell
$ kubectl get flightrecorder -o yaml jmx-listener-55d48f7cfc-8nkln
```
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: FlightRecorder
metadata:
  labels:
    app: jmx-listener-55d48f7cfc-8nkln
  name: jmx-listener-55d48f7cfc-8nkln
  namespace: cryostat-operator-system
  ownerReferences:
  - apiVersion: v1
    kind: Pod
    name: jmx-listener-55d48f7cfc-8nkln
spec:
  recordingSelector:
    matchLabels:
      operator.cryostat.io/flightrecorder: jmx-listener-55d48f7cfc-8nkln
status:
  events:
  - category:
    - Java Application
    description: Writing data to a socket
    name: Socket Write
    options:
      enabled:
        defaultValue: "false"
        description: Record event
        name: Enabled
      stackTrace:
        defaultValue: "false"
        description: Record stack traces
        name: Stack Trace
      threshold:
        defaultValue: 0ns[ns]
        description: Record event with duration above or equal to threshold
        name: Threshold
    typeId: jdk.SocketWrite
  - category:
    - Java Application
    description: Reading data from a socket
    name: Socket Read
    options:
      enabled:
        defaultValue: "false"
        description: Record event
        name: Enabled
      stackTrace:
        defaultValue: "false"
        description: Record stack traces
        name: Stack Trace
      threshold:
        defaultValue: 0ns[ns]
        description: Record event with duration above or equal to threshold
        name: Threshold
    typeId: jdk.SocketRead
  port: 9093
  target:
    kind: Pod
    name: jmx-listener-55d48f7cfc-8nkln
    namespace: cryostat-operator-system
  templates:
  - description: Low overhead configuration safe for continuous use in production environments, typically less than 1 % overhead.
    name: Continuous
    provider: Oracle
    type: TARGET
  - description: Low overhead configuration for profiling, typically around 2 % overhead.
    name: Profiling
    provider: Oracle
    type: TARGET
  - description: Enable all available events in the target JVM, with default option values. This will be very expensive and is intended primarily for testing Cryostat's own capabilities.
    name: ALL
    provider: Cryostat
    type: TARGET
```
(Some fields are removed or abbreviated for readability)

### Configuring JMX Authentication

If the target Pod for a `FlightRecorder` object is using password JMX authentication, the `FlightRecorder` must be configured with these credentials in order for Cryostat to connect to the Pod. The `spec.jmxCredentials` property tells the operator where to find the JMX authentication credentials for the target of the `FlightRecorder`. The `secretName` property must refer to the name of a Secret within the same namespace as the `FlightRecorder`. The `usernameKey` and `passwordKey` are the names of the keys used to index the username and password within the named Secret. If the `usernameKey` or `passwordKey` properties are omitted, the operator will use the default key names of `username` and `password`.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: FlightRecorder
metadata:
  labels:
    app: jmx-listener-55d48f7cfc-8nkln
  name: jmx-listener-55d48f7cfc-8nkln
  namespace: cryostat-operator-system
  ownerReferences:
  - apiVersion: v1
    kind: Pod
    name: jmx-listener-55d48f7cfc-8nkln
spec:
  jmxCredentials:
    secretName: my-jmx-auth-secret
    usernameKey: my-user-key
    passwordKey: my-pass-key
```

## Creating a new Flight Recording

To start a new recording, you will need to create a new `Recording` custom resource. The `Recording` must include the following:

1. `name`: a string uniquely identifying the recording within that service.
2. `eventOptions`: an array of string options passed to Cryostat. Templates can be specified with the option `"template=<template_name>"`, such as `"template=Profiling"` for the Profiling template.
3. `duration`: length of the requested recording as a [duration string](https://golang.org/pkg/time/#ParseDuration).
4. `archive`: whether to save the completed recording to persistent storage.
5. `flightRecorder`: a [`LocalObjectReference`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#localobjectreference-v1-core) pointing to the `FlightRecorder` that should perform the recording.

The following example can serve as a template when creating your own `Recording` object:
```shell
$ cat my-recording.yaml
```
```yaml
apiVersion: rhjmc.redhat.com/v1beta1
kind: Recording
metadata:
  name: my-recording
spec:
  name: my-recording
  eventOptions:
  - "jdk.SocketRead:enabled=true"
  - "jdk.SocketWrite:enabled=true"
  duration: 30s
  archive: true
  flightRecorder:
    name: jmx-listener-55d48f7cfc-8nkln
```
```shell
$ kubectl create -f my-recording.yaml
```

Once the operator has processed the new `Recording`, it will communicate with Cryostat via the referenced `FlightRecorder` to remotely create the JFR recording. Once this occurs, details of the recording are populated in the `status` of the `Recording` object. The `status.duration` property corresponds to the duration the recording was created with, `status.startTime` is when the recording actually started in the target JVM, and `status.state` is the current state of the recording from the following:
* `CREATED`: the recording has been accepted, but has not started yet.
* `RUNNING`: the recording has started and is currently running.
* `STOPPING`: the recording is in the process of finishing.
* `STOPPED`: the recording has completed and the JFR file is fully written.

```shell
$ kubectl get -o yaml recording/my-recording
```
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Recording
metadata:
  finalizers:
  - operator.cryostat.io/recording.finalizer
  labels:
    operator.cryostat.io/flightrecorder: jmx-listener-55d48f7cfc-8nkln
  name: my-recording
  namespace: cryostat-operator-system
spec:
  archive: true
  duration: 30s
  eventOptions:
  - jdk.SocketRead:enabled=true
  - jdk.SocketWrite:enabled=true
  flightRecorder:
    name: jmx-listener-55d48f7cfc-8nkln
  name: my-recording
status:
  downloadURL: https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/targets/service:jmx:rmi:%2F%2F%2Fjndi%2Frmi:%2F%2F10.217.0.29:9093%2Fjmxrmi/recordings/my-recording
  duration: 30s
  reportURL: https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/targets/service:jmx:rmi:%2F%2F%2Fjndi%2Frmi:%2F%2F10.217.0.29:9093%2Fjmxrmi/reports/my-recording
  startTime: "2021-04-29T22:03:28Z"
  state: RUNNING
```

### Creating a continuous Flight Recording

You may not necessarily want your recording to be a fixed duration, in this case you can specify that you want your `Recording` to be continuous. This is done by setting the `spec.duration` to a zero-value.

```shell
$ cat my-cont-recording.yaml
```
```yaml
apiVersion: rhjmc.redhat.com/v1beta1
kind: Recording
metadata:
  name: cont-recording
spec:
  name: cont-recording
  eventOptions:
  - "jdk.SocketRead:enabled=true"
  - "jdk.SocketWrite:enabled=true"
  duration: 0s
  archive: true
  flightRecorder:
    name: jmx-listener-55d48f7cfc-8nkln
```
```shell
$ kubectl create -f my-cont-recording.yaml
```

In order to stop this recording, you'll need to set `spec.state` to `"STOPPED"`, like the following:
```shell
$ kubectl edit recording/cont-recording
```
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Recording
metadata:
  finalizers:
  - operator.cryostat.io/recording.finalizer
  labels:
    operator.cryostat.io/flightrecorder: jmx-listener-55d48f7cfc-8nkln
  name: cont-recording
  namespace: cryostat-operator-system
spec:
  archive: true
  duration: 0s
  eventOptions:
  - jdk.SocketRead:enabled=true
  - jdk.SocketWrite:enabled=true
  flightRecorder:
    name: jmx-listener-55d48f7cfc-8nkln
  name: cont-recording
  state: STOPPED
status:
  downloadURL: https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/targets/service:jmx:rmi:%2F%2F%2Fjndi%2Frmi:%2F%2F10.217.0.29:9093%2Fjmxrmi/recordings/cont-recording
  duration: 0s
  reportURL: https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/targets/service:jmx:rmi:%2F%2F%2Fjndi%2Frmi:%2F%2F10.217.0.29:9093%2Fjmxrmi/reports/cont-recording
  startTime: "2021-04-29T22:12:59Z"
  state: RUNNING
```

## Downloading a Flight Recording

When Cryostat starts the recording, URLs to the JFR file and automated analysis HTML report are added to `status.downloadURL` and `status.reportURL`, respectively. If `spec.archive` is `true`, the operator archives the recording once completed. The operator then replaces the download and report URLs with persisted versions that do not depend on the lifecycle of the target JVM.

The JFR file and HTML report can be downloaded from the URLs contained in `downloadURL` and `reportURL` using cURL, or similar tools.

```shell
$ kubectl get -o yaml recording/my-recording
```
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Recording
metadata:
  finalizers:
  - operator.cryostat.io/recording.finalizer
  labels:
    operator.cryostat.io/flightrecorder: jmx-listener-55d48f7cfc-8nkln
  name: my-recording
  namespace: cryostat-operator-system
spec:
  archive: true
  duration: 30s
  eventOptions:
  - jdk.SocketRead:enabled=true
  - jdk.SocketWrite:enabled=true
  flightRecorder:
    name: jmx-listener-55d48f7cfc-8nkln
  name: my-recording
status:
  downloadURL: https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/recordings/10-217-0-29_my-recording_20210429T220400Z.jfr
  duration: 30s
  reportURL: https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/reports/10-217-0-29_my-recording_20210429T220400Z.jfr
  startTime: "2021-04-29T22:03:28Z"
  state: STOPPED
```

If running on OpenShift, you will need to pass your bearer token with the `curl` request. (You may also need -k if your test cluster uses a self-signed certificate)
```shell
$ curl -k -H "Authorization: Bearer $(oc whoami -t)" \
https://cryostat-sample-cryostat-operator-system.apps-crc.testing:443/api/v1/recordings/10-217-0-29_my-recording_20210429T220400Z.jfr \
my-recording.jfr
```

You can then open and analyze the recording with [JDK Mission Control](https://github.com/openjdk/jmc/) on your local machine.
