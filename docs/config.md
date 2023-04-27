## Configuring Cryostat
The operator creates and manages a Deployment of [Cryostat](https://github.com/cryostatio/cryostat) when the user creates or updates a `Cryostat` object. Only one `Cryostat` object should exist in the operator's namespace at a time. There are a few options available in the `Cryostat` spec that control how Cryostat is deployed.

### Minimal Deployment
The `spec.minimal` property determines what is deployed alongside Cryostat. This value is set to `false` by default, which tells the operator to deploy Cryostat, with a [customized Grafana](https://github.com/cryostatio/cryostat-grafana-dashboard) and a [Grafana Data Source for JFR files](https://github.com/cryostatio/jfr-datasource) as 3 containers within a Pod. When `minimal` is set to `true`, the Deployment consists of only the Cryostat container.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  minimal: true
```

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
| Reports Container CPU | 128m |
| Reports Container Memory | 256Mi |


### Resource Requirements
By default, the operator deploys Cryostat with pre-configured resource requests:

| Request | Quantity |
|---------|----------|
| Cryostat container CPU | 100m |
| Cryostat container Memory | 384Mi |
| JFR Data Source container CPU | 100m |
| JFR Data Source container Memory | 512Mi |
| Grafana container CPU | 100m |
| Grafana container Memory | 256Mi |

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

### Cryostat Client Options
The `maxWsConnections` property optionally specifies the maximum number of WebSocket client connections allowed.
The default number of `maxWsConnections` is unlimited.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  maxWsConnections: 2
```

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

### JMX Credentials Database

The Cryostat application must be provided with a password to encrypt saved JMX credentials in database. The user can specify a secret containing the password entry with key `CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD`. The Cryostat application will use this password to encrypt saved JMX credentials in database.

For example:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: credentials-database-secret
type: Opaque
stringData:
  CRYOSTAT_JMX_CREDENTIALS_DB_PASSWORD: a-very-good-password
```

Then, the property `.spec.jmxCredentialsDatabaseOptions.databaseSecretName` must be set to use this secret for password.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  jmxCredentialsDatabaseOptions:
    databaseSecretName: credentials-database-secret
```

**Note**: If the secret is not provided, a default one is generated for this purpose. However, switching between using provided and generated secret is not allowed to avoid password mismatch that causes the Cryostat application's failure to access the credentials database.

### Authorization Properties

When running on OpenShift, the user is required to have sufficient permissions for certain Kubernetes resources that are mapped into Cryostat-managed resources for authorization.

The mappings can be specified using a ConfigMap that is compatible with [`OpenShiftAuthManager.properties`](https://github.com/cryostatio/cryostat/blob/6db048682b2b0048c1f6ea9215de626b5a5be284/src/main/resources/io/cryostat/net/openshift/OpenShiftAuthManager.properties). For example:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: auth-properties
data:  
  auth.properties: |
    TARGET=pods,deployments.apps
    RECORDING=pods,pods/exec
    CERTIFICATE=deployments.apps,pods,cryostats.operator.cryostat.io
    CREDENTIALS=cryostats.operator.cryostat.io
```

If custom mapping is specified, a ClusterRole must be defined and should contain permissions for all Kubernetes objects listed in custom permission mapping. This ClusterRole will give additional rules on top of [default rules](https://github.com/cryostatio/cryostat-operator/blob/1b5d1ab97fca925e14b6c2baf2585f5e04426440/config/rbac/oauth_client.yaml).


**Note**: Using [`Secret`](https://kubernetes.io/docs/concepts/configuration/secret/) in mapping can fail with access denied under [security protection](https://kubernetes.io/docs/concepts/configuration/secret/#information-security-for-secrets) against escalations. Find more details about this issue [here](https://docs.openshift.com/container-platform/4.11/authentication/tokens-scoping.html#scoping-tokens-role-scope_configuring-internal-oauth).

The property `spec.authProperties` can then be set to configure Cryostat to use this mapping instead of the default ones.
```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: Cryostat
metadata:
  name: cryostat-sample
spec:
  authProperties:
    configMapName: auth-properties
    filename: auth.properties
    clusterRoleName: oauth-cluster-role
```

Each `configMapName` must refer to the name of a Config Map in the same namespace as Cryostat. The corresponding `filename` must be a key within that Config Map containing resource mappings. The `clusterRoleName` must be a valid name of an existing Cluster Role.

**Note:** If the mapping is updated, Cryostat must be manually restarted.


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
