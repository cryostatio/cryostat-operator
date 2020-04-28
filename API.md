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
2. `eventOptions`: an array of string options passed to Container JFR. The `"template=ALL"` special string can be used to enable all available events.
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

Once the operator has processed the new `Recording`, it will communicate with Container JFR via the referenced `FlightRecorder` to remotely create the JFR recording. Once this occurs, details of the recording are populated in the `status` of the `Recording` object. The `status.duration` property corresponds to the duration the recording was created with, `status.startTime` is when the recording actually started in the target JVM, and `status.state` is the current state of the recording from the following:
* `CREATED`: the recording has been accepted, but has not started yet.
* `RUNNING`: the recording has started and is currently running.
* `STOPPING`: the recording is in the process of finishing.
* `STOPPED`: the recording has completed and the JFR file is fully written.

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

### Creating a continuous Flight Recording

You may not necessarily want your recording to be a fixed duration, in this case you can specify that you want your `Recording` to be continuous. This is done by setting the `spec.duration` to a zero-value.

```shell
$ cat my-cont-recording.json
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
        "name": "cont-recording",
        "namespace": "default"
    },
    "spec": {
        "name": "cont-recording",
        "eventOptions": [
            "jdk.SocketRead:enabled=true",
            "jdk.SocketWrite:enabled=true"
        ],
        "duration": "0s",
        "archive": true,
        "flightRecorder": {
            "name": "jmx-listener"
        }
    },
    "status": {}
}
```
```shell
$ oc create -f my-cont-recording.json
```

In order to stop this recording, you'll need to set `spec.state` to `"STOPPED"`, like the following:
```shell
$ oc edit -o json recording/my-cont-recording
```
```json
{
    "apiVersion": "rhjmc.redhat.com/v1alpha2",
    "kind": "Recording",
    "metadata": {
        "creationTimestamp": "2020-03-26T22:12:30Z",
        "generation": 1,
        "labels": {
            "app": "jmx-listener",
            "rhjmc.redhat.com/flightrecorder": "jmx-listener"
        },
        "name": "cont-recording",
        "namespace": "default",
        "resourceVersion": "395986",
        "selfLink": "/apis/rhjmc.redhat.com/v1alpha2/namespaces/default/recordings/cont-recording",
        "uid": "e2b7f375-6fae-11ea-ae0c-52fdfc072182"
    },
    "spec": {
        "archive": true,
        "duration": "0s",
        "eventOptions": [
            "jdk.SocketRead:enabled=true",
            "jdk.SocketWrite:enabled=true"
        ],
        "flightRecorder": {
            "name": "jmx-listener"
        },
        "name": "cont-recording",
        "state": "STOPPED"
    },
    "status": {
        "duration": "0s",
        "startTime": "2020-03-26T22:12:31Z",
        "state": "RUNNING"
    }
}
```

## Downloading a Flight Recording

Once the recording completes and `spec.archive` is `true`, the operator archives the recording and places a download link in the `status.downloadUrl` of the `Recording` that you can then download with curl.

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
        "resourceVersion": "395834",
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
        "downloadURL": "https://containerjfr-default.apps-crc.testing:443/recordings/172-30-177-37_my-recording_20200326T221136Z.jfr",
        "duration": "30s",
        "startTime": "2020-03-26T22:11:04Z",
        "state": "STOPPED"
    }
}
```

You'll need to pass your bearer token with the curl request. (You may also need -k if your test cluster uses a self-signed certificate)
```shell
$ curl -k -H "Authorization: Bearer $(oc whoami -t)" \
https://containerjfr-default.apps-crc.testing:443/recordings/172-30-177-37_my-recording_20200326T221136Z.jfr \
my-recording.jfr
```

You can then open and analyze the recording with [JDK Mission Control](https://github.com/openjdk/jmc/) on your local machine.
