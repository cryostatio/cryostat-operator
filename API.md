# `FlightRecorder` API overview

This operator provides a Kubernetes API to interact with [Container JFR](https://github.com/rh-jmc-team/container-jfr).
This API comes in the form of the `FlightRecorders` Custom Resource Definition, and allows you to create, list, and download recordings from a Kubernetes cluster.

## Retrieving `FlightRecorder` objects
You can use `FlightRecorders` like any other built-in resource on the command line with oc/kubectl.

```shell
$ oc get flightrecorders
NAME           AGE
containerjfr   7d16h
```

`FlightRecorder` objects are created by the operator whenever a new Container JFR-compatible service is detected.
Services that expose a port named `jfr-jmx` are considered compatible. The number of this port is stored in the `spec.port` property for use by the operator. Each `FlightRecorder` object maps one-to-one with a Kubernetes service. This service is stored in the `status.target` property of the `FlightRecorder` object.

```shell
$ oc get -o json flightrecorders/containerjfr
```
```json
{
    "apiVersion": "rhjmc.redhat.com/v1alpha1",
    "kind": "FlightRecorder",
    "metadata": {
        "creationTimestamp": "2020-01-15T22:54:22Z",
        "generation": 1,
        "labels": {
            "app": "containerjfr"
        },
        "name": "containerjfr",
        "namespace": "default",
        "ownerReferences": [
            {
                "apiVersion": "v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "Service",
                "name": "containerjfr",
                "uid": "f895e203-37e9-11ea-8866-52fdfc072182"
            }
        ],
        "resourceVersion": "298903",
        "selfLink": "/apis/rhjmc.redhat.com/v1alpha1/namespaces/default/flightrecorders/containerjfr",
        "uid": "f896b6e1-37e9-11ea-8866-52fdfc072182"
    },
    "spec": {
        "port": 9091,
        "recordingRequests": []
    },
    "status": {
        "recordings": [],
        "target": {
            "apiVersion": "v1",
            "kind": "Service",
            "name": "containerjfr",
            "namespace": "default",
            "resourceVersion": "298899",
            "uid": "f895e203-37e9-11ea-8866-52fdfc072182"
        }
    }
}
```

## Creating a new Flight Recording

To start a new recording, you will need to add a new recording request to the `FlightRecorder` in the `spec.recordingRequests` array. You can do this nicely on the command line with `oc edit`. The recording request must include the following:

1. `name`: a string uniquely identifying the recording within that service
2. `eventOptions`: an array of string options passed to Container JFR. The `"ALL"` special string can be used to enable all available events.
3. `duration`: length of the requested recording as a [duration string](https://golang.org/pkg/time/#ParseDuration).

```shell
oc edit -o json flightrecorders/containerjfr
```
```json
    "spec": {
        "port": 9091,
        "recordingRequests": [
                {
                        "name": "my-recording",
                        "eventOptions": [ "jdk.SocketRead:enabled=true", "jdk.SocketWrite:enabled=true" ],
                        "duration": "30s"
                }
        ]
    },
    "status": {
        "recordings": [],
        "target": {
            "apiVersion": "v1",
            "kind": "Service",
            "name": "containerjfr",
            "namespace": "default",
            "resourceVersion": "298899",
            "uid": "f895e203-37e9-11ea-8866-52fdfc072182"
        }
    }
```

Once the operator has processed the request, it will be removed from the `spec.recordingRequests` array and a corresponding entry will appear in the `status.recordings` array. A recording is considered `active` if it is currently running.

```shell
$ oc get -o json flightrecorders/containerjfr
```
```json
    "spec": {
        "port": 9091,
        "recordingRequests": []
    },
    "status": {
        "recordings": [
            {
                "active": true,
                "duration": "30s",
                "name": "my-recording",
                "startTime": "2020-01-23T15:55:04Z"
            }
        ],
        "target": {
            "apiVersion": "v1",
            "kind": "Service",
            "name": "containerjfr",
            "namespace": "default",
            "resourceVersion": "298899",
            "uid": "f895e203-37e9-11ea-8866-52fdfc072182"
        }
    }
```

## Downloading a Flight Recording

Once the recording completes, the operator archives the recording and places a download link in the recording's status that you can then download with curl.

```shell
$ oc get -o json flightrecorders/containerjfr
```
```json
    "spec": {
        "port": 9091,
        "recordingRequests": []
    },
    "status": {
        "recordings": [
            {
                "active": false,
                "downloadUrl": "https://containerjfr-default.apps-crc.testing:443/recordings/172-30-157-32_my-recording_20200123T155535Z.jfr",
                "duration": "30s",
                "name": "my-recording",
                "startTime": "2020-01-23T15:55:04Z"
            }
        ],
        "target": {
            "apiVersion": "v1",
            "kind": "Service",
            "name": "containerjfr",
            "namespace": "default",
            "resourceVersion": "298899",
            "uid": "f895e203-37e9-11ea-8866-52fdfc072182"
        }
    }
```

You'll need to pass your bearer token with the curl request. (You may also need -k if your test cluster uses a self-signed certificate)
```shell
$ curl -k -H "Authorization: Bearer $(oc whoami -t)" https://containerjfr-default.apps-crc.testing:443/recordings/172-30-157-32_my-recording_20200123T155535Z.jfr my-recording.jfr
```

You can then open and analyze the recording with [JDK Mission Control](https://github.com/openjdk/jmc/) on your local machine.
