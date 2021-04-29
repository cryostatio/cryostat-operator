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
