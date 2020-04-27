# `FlightRecorder` API overview

This operator provides a Kubernetes API to interact with [Container JFR](https://github.com/rh-jmc-team/container-jfr).
This API comes in the form of the `FlightRecorders` and `Recordings` Custom Resource Definitions, and allows you to create, list, delete, and download recordings from a Kubernetes cluster.

## Retrieving `FlightRecorder` objects
You can use `FlightRecorders` like any other built-in resource on the command line with oc/kubectl.

```shell
$ oc get flightrecorders
NAME           AGE
containerjfr   75s
jmx-listener   77s
```

`FlightRecorder` objects are created by the operator whenever a new Container JFR-compatible service is detected.
Services that expose a port named `jfr-jmx` are considered compatible. The number of this port is stored in the `status.port` property for use by the operator. Each `FlightRecorder` object maps one-to-one with a Kubernetes service. This service is stored in the `status.target` property of the `FlightRecorder` object. When the operator learns of a new `FlightRecorder` object, it queries Container JFR for a list of all available JFR events for the JVM behind the `FlightRecorder's` service. The details of these event types are stored in the `status.events` property of the `FlightRecorder`. The `spec.recordingSelector` property provides an association of `Recordings` (outlined below) with this `FlightRecorder` object.

```shell
$ oc get -o json flightrecorders/jmx-listener
```
```json
{
    "apiVersion": "rhjmc.redhat.com/v1alpha2",
    "kind": "FlightRecorder",
    "metadata": {
        "creationTimestamp": "2020-03-26T21:54:29Z",
        "generation": 1,
        "labels": {
            "app": "jmx-listener"
        },
        "name": "jmx-listener",
        "namespace": "default",
        "ownerReferences": [
            {
                "apiVersion": "v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "Service",
                "name": "jmx-listener",
                "uid": "4f5b0695-6fac-11ea-ae0c-52fdfc072182"
            }
        ],
        "resourceVersion": "393024",
        "selfLink": "/apis/rhjmc.redhat.com/v1alpha2/namespaces/default/flightrecorders/jmx-listener",
        "uid": "5e53b4ee-6fac-11ea-ae0c-52fdfc072182"
    },
    "spec": {
        "recordingSelector": {
            "matchLabels": {
                "rhjmc.redhat.com/flightrecorder": "jmx-listener"
            }
        }
    },
    "status": {
        "events": [
            {
                "category": [
                    "Java Application"
                ],
                "description": "Writing data to a socket",
                "name": "Socket Write",
                "options": {
                    "enabled": {
                        "defaultValue": "false",
                        "description": "Record event",
                        "name": "Enabled"
                    },
                    "stackTrace": {
                        "defaultValue": "false",
                        "description": "Record stack traces",
                        "name": "Stack Trace"
                    },
                    "threshold": {
                        "defaultValue": "0ns[ns]",
                        "description": "Record event with duration above or equal to threshold",
                        "name": "Threshold"
                    }
                },
                "typeId": "jdk.SocketWrite"
            },
            {
                "category": [
                    "Java Application"
                ],
                "description": "Reading data from a socket",
                "name": "Socket Read",
                "options": {
                    "enabled": {
                        "defaultValue": "false",
                        "description": "Record event",
                        "name": "Enabled"
                    },
                    "stackTrace": {
                        "defaultValue": "false",
                        "description": "Record stack traces",
                        "name": "Stack Trace"
                    },
                    "threshold": {
                        "defaultValue": "0ns[ns]",
                        "description": "Record event with duration above or equal to threshold",
                        "name": "Threshold"
                    }
                },
                "typeId": "jdk.SocketRead"
            }
        ],
        "port": 9093,
        "target": {
            "apiVersion": "v1",
            "kind": "Service",
            "name": "jmx-listener",
            "namespace": "default",
            "resourceVersion": "392780",
            "uid": "4f5b0695-6fac-11ea-ae0c-52fdfc072182"
        }
    }
}
```
(Event listing abbreviated for readability)

## Creating a new Flight Recording

To start a new recording, you will need to create a new `Recording` custom resource. The `Recording` must include the following:

1. `name`: a string uniquely identifying the recording within that service.
2. `eventOptions`: an array of string options passed to Container JFR. The `"ALL"` special string can be used to enable all available events.
3. `duration`: length of the requested recording as a [duration string](https://golang.org/pkg/time/#ParseDuration).
4. `archive`: whether to save the completed recording to persistent storage.
5. `flightRecorder`: a [`LocalObjectReference`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#localobjectreference-v1-core) pointing to the `FlightRecorder` that should perform the recording.

The following example can serve as a template when creating your own `Recording` object:
```shell
$ cat my-recording.json
```
```json
{
    "apiVersion": "rhjmc.redhat.com/v1alpha2",
    "kind": "Recording",
    "metadata": {
        "labels": {
            "app": "jmx-listener",
            "rhjmc.redhat.com/flightrecorder": "jmx-listener"
        },
        "name": "my-recording",
        "namespace": "default"
    },
    "spec": {
        "name": "my-recording",
        "eventOptions": [
            "jdk.SocketRead:enabled=true",
            "jdk.SocketWrite:enabled=true"
        ],
        "duration": "30s",
        "archive": true,
        "flightRecorder": {
            "name": "jmx-listener"
        }
    },
    "status": {}
}
```
```shell
$ oc create -f my-recording.json
```

Once the operator has processed the new `Recording`, it will communicate with Container JFR via the referenced `FlightRecorder` to remotely create the JFR recording. Once this occurs, details of the recording are populated in the `status` of the `Recording` object.

```shell
$ oc get -o json recording/my-recording
```
```json
{
    "apiVersion": "rhjmc.redhat.com/v1alpha2",
    "kind": "Recording",
    "metadata": {
        "creationTimestamp": "2020-03-26T22:11:04Z",
        "generation": 1,
        "labels": {
            "app": "jmx-listener",
            "rhjmc.redhat.com/flightrecorder": "jmx-listener"
        },
        "name": "my-recording",
        "namespace": "default",
        "resourceVersion": "395738",
        "selfLink": "/apis/rhjmc.redhat.com/v1alpha2/namespaces/default/recordings/my-recording",
        "uid": "af1631e2-6fae-11ea-ae0c-52fdfc072182"
    },
    "spec": {
        "archive": true,
        "duration": "30s",
        "eventOptions": [
            "jdk.SocketRead:enabled=true",
            "jdk.SocketWrite:enabled=true"
        ],
        "flightRecorder": {
            "name": "jmx-listener"
        },
        "name": "my-recording"
    },
    "status": {
        "duration": "30s",
        "startTime": "2020-03-26T22:11:04Z",
        "state": "RUNNING"
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
