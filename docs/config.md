## Configuring Cryostat
The operator creates and manages a Deployment of [Cryostat](https://github.com/cryostatio/cryostat) when the user creates or updates a `Cryostat` object. Only one `Cryostat` object should exist in the operator's namespace at a time. There are a few options available in the `Cryostat` spec that control how Cryostat is deployed.

### Target Namespaces
Specify the list of namespaces containing your workloads that you want your multi-namespace Cryostat installation to work with under the `spec.targetNamespaces` property. The resulting Cryostat will have permissions to access workloads only within these specified namespaces. If not specified, `spec.targetNamespaces` will default to the namespace of the `Cryostat` object.

```yaml
apiVersion: operator.cryostat.io/v1beta2
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  targetNamespaces:
    - my-app-namespace
    - my-other-app-namespace
```

#### Data Isolation
When installed in a multi-namespace manner, all users with access to a Cryostat instance have the same visibility and privileges to all data available to that Cryostat instance. Administrators deploying Cryostat instances must ensure that the users who have access to a Cryostat instance also have equivalent access to all the applications that can be monitored by that Cryostat instance. Otherwise, underprivileged users may use Cryostat to escalate permissions to start recordings and collect JFR data from applications that they do not otherwise have access to.

For now, all authorization checks are done against the namespace where Cryostat is installed. For a user to use Cryostat with workloads in a target namespace, that user must have the necessary Kubernetes permissions in the namespace where Cryostat is installed.

### Disabling cert-manager Integration
By default, the operator expects [cert-manager](https://cert-manager.io/) to be available in the cluster. The operator uses cert-manager to generate a self-signed CA to allow traffic between Cryostat components within the cluster to use HTTPS. If cert-manager is not available in the cluster, this integration can be disabled with the `spec.enableCertManager` property.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  enableCertManager: false
```

### Custom Event Templates
All JDK Flight Recordings created by Cryostat are configured using an event template. These templates specify which events to record, and Cryostat includes some templates automatically, including those provided by the target's JVM. Cryostat also provides the ability to [upload customized templates](https://cryostat.io/getting-started/#download-edit-and-upload-a-customized-event-template), which can then be used to create recordings.

The Cryostat Operator provides an additional feature to pre-configure Cryostat with custom templates that are stored in Config Maps. When Cryostat is deployed from this Cryostat object, it will have the listed templates already available for use.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  eventTemplates:
  - configMapName: custom-template
    filename: my-template.jfc
```
Multiple templates can be specified in the `eventTemplates` array. Each `configMapName` must refer to the name of a Config Map in the same namespace as Cryostat. The corresponding `filename` must be a key within that Config Map containting the template file.

### Trusted TLS Certificates
By default, Cryostat uses TLS when connecting to the user's applications over JMX. In order to verify the identity of the applications Cryostat connects to, it should be configured to trust the TLS certificates presented by those applications. One way to do that is to specify certificates that Cryostat should trust in the `spec.trustedCertSecrets` property.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  trustedCertSecrets:
  - secretName: my-tls-secret
    certificateKey: ca.crt
```
Multiple TLS secrets may be specified in the `trustedCertSecrets` array. The `secretName` property is mandatory, and must refer to the name of a Secret within the same namespace as the `Cryostat` object. The `certificateKey` must point to the X.509 certificate file to be trusted. If `certificateKey` is omitted, the default key name of `tls.crt` will be used.

### Storage Options
Cryostat uses storage volumes to hold Flight Recording files and user-configured Recording Templates. In the interest of persisting these files across redeployments, Cryostat uses a Persistent Volume Claim by default. Unless overidden, the operator will create a Persistent Volume Claim with the default Storage Class and 500MiB of storage capacity. 

Through the `spec.storageOptions` property, users can choose to provide either a custom Persistent Volume Claim `pvc.spec` or an `emptyDir` configuration. Either of these configurations will override any defaults when the operator creates the storage volume. If an `emptyDir` configuration is enabled, Cryostat will use an EmptyDir volume instead of a Persistent Volume Claim. Additional labels and annotations for the Persistent Volume Claim may also be specified.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  storageOptions:
    pvc:
      labels:
        my-custom-label: some-value
      annotations:
        my-custom-annotation: some-value
      spec:
        storageClassName: faster
        resources:
          requests:
            storage: 1Gi
```
The `emptyDir.medium` and `emptyDir.sizeLimit` fields are optional. If an `emptyDir` is
specified without additional configurations, Cryostat will mount an EmptyDir volume with the same default values as Kubernetes.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  storageOptions:
    emptyDir:
      enabled: true
      medium: "Memory"
      sizeLimit: 1Gi
```

### Service Options
The Cryostat operator creates three services: one for the core Cryostat application, one for Grafana, and one for the cryostat-reports sidecars. These services are created by default as Cluster IP services. The core service exposes two ports: `8181` for HTTP and `9091` for JMX. The Grafana service exposes port `3000` for HTTP traffic. The Reports service exposts port `10000` for HTTP traffic. The service type, port numbers, labels and annotations can all be customized using the `spec.serviceOptions` property.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  serviceOptions:
    coreConfig:
      labels:
        my-custom-label: some-value
      annotations:
        my-custom-annotation: some-value
      serviceType: NodePort
      httpPort: 8080
      jmxPort: 9095
    grafanaConfig:
      labels:
        my-custom-label: some-value
      annotations:
        my-custom-annotation: some-value
      serviceType: NodePort
      httpPort: 8080
    reportsConfig:
      labels:
        my-custom-label: some-value
      annotations:
        my-custom-annotation: some-value
      serviceType: NodePort
      httpPort: 13161
```

### Reports Options
The Cryostat operator can optionally configure Cryostat to use `cryostat-reports` as a sidecar microservice for generating Automated Rules Analysis Reports. If this is not configured then the main Cryostat container will perform this task itself, however, this is a relatively heavyweight and resource-intensive task. It is recommended to configure `cryostat-reports` sidecars if the Automated Analysis feature will be used or relied upon. The number of sidecar containers to deploy and the amount of CPU and memory resources to allocate for each container can be customized using the `spec.reportOptions` property.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  reportOptions:
    replicas: 1
    resources:
      requests:
        cpu: 1000m
        memory: 512Mi
```
If zero sidecar replicas are configured, SubProcessMaxHeapSize configures
the maximum heap size of the main Cryostat container's subprocess report generator in MiB.
The default heap size is `200` MiB.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  reportOptions:
    replicas: 0
    subProcessMaxHeapSize: 200
```

If the sidecar's resource requests are not specified, they are set with the following defaults:

| Request | Quantity |
|---------|----------|
| Reports Container CPU | 200m |
| Reports Container Memory | 384Mi |


### Resource Requirements
By default, the operator deploys Cryostat with pre-configured resource requests:

| Request | Quantity |
|---------|----------|
| Cryostat container CPU | 500m |
| Cryostat container Memory | 256Mi |
| JFR Data Source container CPU | 200m |
| JFR Data Source container Memory | 384Mi |
| Grafana container CPU | 100m |
| Grafana container Memory | 120Mi |

Using the Cryostat custom resource, you can define resources requests and/or limits for each of the three containers in Cryostat's main pod:
- the `core` container running the Cryostat backend and web application. If setting a memory limit for this container, we recommend at least 768MiB.
- the `datasource` container running JFR Data Source, which converts recordings into a Grafana-compatible format.
- the `grafana` container running the Grafana instance customized for Cryostat.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  resources:
    coreResources:
      requests:
        cpu: 1200m
        memory: 768Mi
      limits:
        cpu: 2000m
        memory: 2Gi
    dataSourceResources:
      requests:
        cpu: 500m
        memory: 256Mi
      limits:
        cpu: 800m
        memory: 512Mi
    grafanaResources:
      requests:
        cpu: 800m
        memory: 256Mi
      limits:
        cpu: 1000m
        memory: 512Mi
```
This example sets CPU and memory requests and limits for each container, but you may choose to define any combination of requests and limits that suits your use case.

Note that if you define limits lower than the default requests, the resource requests will be set to the value of your provided limits.

### Network Options
When running on Kubernetes, the operator requires Ingress configurations for each of its services to make them available outside of the cluster. For a `Cryostat` object named `x`, the following Ingress configurations must be specified within the `spec.networkOptions` property:
- `coreConfig` exposing the service `x` on port `8181` (or alternate specified in [Service Options](#service-options)).
- `grafanaConfig` exposing the service `x-grafana` on port `3000` (or alternate specified in [Service Options](#service-options)).

The user is responsible for providing the hostnames for each Ingress. In Minikube, this can be done by adding entries to the host machine's `/etc/hosts` for each hostname, pointing to Minikube's IP address. See: https://kubernetes.io/docs/tasks/access-application-cluster/ingress-minikube/

Since Cryostat only accept HTTPS traffic by default, the Ingresses should be configured to forward traffic to the backend services over HTTPS. For the NGINX Ingress Controller, this can be done with the `nginx.ingress.kubernetes.io/backend-protocol` annotation. The operator considers TLS to be enabled for the Ingress if the Ingress's `spec.tls` array is non-empty. The example below uses the cluster's default wildcard certificate.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  networkOptions:
    coreConfig:
      annotations:
        nginx.ingress.kubernetes.io/backend-protocol : HTTPS
      ingressSpec:
        tls:
        - {}
        rules:
        - host: testing.cryostat
          http:
            paths:
            - path: /
              pathType: Prefix
              backend:
                service:
                  name: cryostat-sample
                  port:
                    number: 8181
    grafanaConfig:
      annotations:
        nginx.ingress.kubernetes.io/backend-protocol: HTTPS
      ingressSpec:
        tls:
        - {}
        rules:
        - host: testing.cryostat-grafana
          http:
            paths:
            - path: /
              pathType: Prefix
              backend:
                service:
                  name: cryostat-sample-grafana
                  port:
                    number: 3000
```

When running on OpenShift, labels and annotations specified in `coreConfig` and `grafanaConfig` will be applied to the coresponding Routes created by the operator.

### JMX Cache Configuration Options
Cryostat's target JMX connection cache can be optionally configured with `targetCacheSize` and `targetCacheTTL`.
`targetCacheSize` sets the maximum number of JMX connections cached by Cryostat.
Use `-1` for an unlimited cache size. The default cache size is unlimited (`-1`).
`targetCacheTTL` sets the time to live (in seconds) for cached JMX connections. The default TTL is `10` seconds.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  jmxCacheOptions:
    targetCacheSize: -1
    targetCacheTTL: 10
```

### Application Database

Cryostat stores various pieces of information in a database. This can also include target application connection credentials, such as target applications' JMX credentials, which are stored in an encrypted database table. By default, the Operator will generate both a random database connection key and a random table encryption key and configure Cryostat and the database to use these. You may also specify these keys yourself by creating a Secret containing the keys `CONNECTION_KEY` and `ENCRYPTION_KEY`.

For example:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: credentials-database-secret
type: Opaque
stringData:
  CONNECTION_KEY: a-very-good-password
  ENCRYPTION_KEY: a-second-good-password
```

Then, the property `.spec.databaseOptions.secretName` must be set to use this Secret for the two keys.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  jmxCredentialsDatabaseOptions:
    databaseSecretName: credentials-database-secret
```

**Note**: If the secret is not provided, one is generated for this purpose containing two randomly generated keys. However, switching between using provided and generated secret is not allowed to avoid password mismatch that causes the Cryostat application's failure to access the database or failure to decrypt the credentials keyring.

### Authorization Options

On OpenShift, the authentication/authorization proxy deployed in front of the Cryostat application requires all users to pass a `create pods/exec` access review in the Cryostat installation namespace.
This means that access to the Cryostat application is granted to exactly the set of OpenShift cluster user accounts and service accounts which have this Role.

The auth proxy may also be configured to allow Basic authentication by creating a Secret containing an `htpasswd` user file. Any user accounts defined in this file will also be granted access to
the Cryostat application, and when this configuration is enabled you will see an additional Basic login option when visiting the Cryostat application UI. If deployed on a non-OpenShift Kubernetes
then this is the only supported authentication mechanism.

If not deployed on OpenShift, or if OpenShift SSO integration is disabled, then no authentication is performed by default - the Cryostat application UI is openly accessible. You should configure
`htpasswd` Basic authentication or install some other access control mechanism.

```yaml
apiVersion: operator.cryostat.io/v1beta2
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  authorizationOptions:
    openShiftSSO: # only effective when running on OpenShift
      disable: false # set this to `true` to disable OpenShift SSO integration
      accessReview: # override this to change the required Role for users and service accounts to access the application
        verb: create
        resource: pods
        subresource: exec
        namespace: cryostat-install-namespace
    basicAuth:
      secretName: my-secret # a Secret with this name must exist in the Cryostat installation namespace
      filename: htpasswd.conf # the name of the htpasswd user file within the Secret
```


### Security Context

With [Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/), pods must be properly configured under the enforced security standards defined globally or on namespace level to be admitted to launch.

The user is responsible for ensuring the security contexts of their workloads to meet these standards. The property `spec.securityOptions` can be set to define security contexts for Cryostat application and `spec.reportOptions.securityOptions` is for its report sidecar.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  securityOptions:
    podSecurityContext:
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    coreSecurityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
      runAsUser: 1001
    dataSourceSecurityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
    grafanaSecurityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
  reportOptions:
    replicas: 1
    podSecurityContext:
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    reportsSecurityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
      runAsUser: 1001
```

If not specified, the security contexts are defaulted to conform to the [restricted](https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted) Pod Security Standard. 
For the Cryostat application pod, the operator selects an fsGroup to ensure that Cryostat can read and write files in its Persistent Volume.

On OpenShift, Cryostat application pod's `spec.securityContext.seccompProfile` is left unset for backward compatibility. On versions of OpenShift supporting Pod Security Admission, the `restricted-v2` Security Context Constraint sets `seccompProfile` to `runtime/default` as required for the restricted Pod Security Standard. For more details, see [Security Context Constraints](https://docs.openshift.com/container-platform/4.11/authentication/managing-security-context-constraints.html#default-sccs_configuring-internal-oauth).

### Scheduling Options

If you wish to control which nodes Cryostat and its reports microservice are scheduled on, you may do so when configuring your Cryostat instance. You can specify a [Node Selector](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector), [Affinities](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity) and [Tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/). For the main Cryostat application, use the `spec.SchedulingOptions` property. For the report generator, use `spec.ReportOptions.SchedulingOptions`.

```yaml
kind: Cryostat
apiVersion: operator.cryostat.io/v1beta1
metadata:
  name: cryostat
spec:
  schedulingOptions:
    nodeSelector:
      node: good
    affinity:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: node
              operator: In
              values:
              - good
              - better
      podAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              pod: good
          topologyKey: topology.kubernetes.io/zone
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              pod: bad
          topologyKey: topology.kubernetes.io/zone
    tolerations:
    - key: node
      operator: Equal
      value: ok
      effect: NoExecute
  reportOptions:
    replicas: 1
    schedulingOptions:
      nodeSelector:
        node: good
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: node
                operator: In
                values:
                - good
                - better
        podAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                pod: good
            topologyKey: topology.kubernetes.io/zone
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                pod: bad
            topologyKey: topology.kubernetes.io/zone
      tolerations:
      - key: node
        operator: Equal
        value: ok
        effect: NoExecute
```

### Target Discovery Options

If you wish to use only Cryostat's [Discovery Plugin API](https://github.com/cryostatio/cryostat/blob/801779d5ddf7fa30f7b230f649220a852b06f27d/docs/DISCOVERY_PLUGINS.md), set the property `spec.targetDiscoveryOptions.builtInDiscoveryDisabled` to `true` to disable Cryostat's built-in discovery mechanisms.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  targetDiscoveryOptions:
    builtInDiscoveryDisabled: true
```
