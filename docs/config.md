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
Cryostat uses storage volumes to hold Flight Recording files and user-configured Recording Templates. In the interest of persisting these files across redeployments, Cryostat uses a Persistent Volume Claim. By default, the operator will create a Persistent Volume Claim with the default Storage Class and 500MiB of storage capacity. Through the `spec.storageOptions` property, users can provide a custom Persistent Volume Claim `spec`, which will override any defaults when the operator creates the Persistent Volume Claim. Additional labels and annotations for the Persistent Volume Claim may also be specified.
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

### Network Options
When running on Kubernetes, the operator requires Ingress configurations for each of its services to make them available outside of the cluster. For a `Cryostat` object named `x`, the following Ingress configurations must be specified within the `spec.networkOptions` property:
- `coreConfig` exposing the service `x` on port `8181`.
- `commandConfig` exposing the service `x-command` on port `9090`.
- `grafanaConfig` exposing the service `x-grafana` on port `3000`.

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
    commandConfig:
      annotations:
        nginx.ingress.kubernetes.io/backend-protocol : HTTPS
      ingressSpec:
        tls:
        - {}
        rules:
        - host: testing.cryostat-command
          http:
            paths:
            - path: /
              pathType: Prefix
              backend:
                service:
                  name: cryostat-sample-command
                  port:
                    number: 9090
    grafanaConfig:
      annotations:
        nginx.ingress.kubernetes.io/backend-protocol : HTTPS
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
