## Configuring a multi-namespace Cryostat
In addition to installing [Cryostat](https://github.com/cryostatio/cryostat) into a single namespace, the Cryostat Operator also allows you to create a Cryostat installation that can work across multiple namespaces. This can be done using the `ClusterCryostat` API. The `ClusterCryostat` API contains all the same [configuration properties](docs/config.md) as the `Cryostat` API does, but has some key differences to enable multi-namespace support that we'll outline below.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: ClusterCryostat
metadata:
  name: clustercryostat-sample
spec:
  installNamespace: my-cryostat-namespace
  targetNamespaces:
    - my-app-namespace
    - my-other-app-namespace
  minimal: false
  enableCertManager: true
```
### Cluster Scoped
In contrast to the namespaced `Cryostat` API, the `ClusterCryostat` API is [cluster-scoped](https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-uris). This distinction was made to prevent privilege escalation situation where a user authorized to install Cryostat into a single namespace would be able to configure that Cryostat to connect to other namespaces where the user does not have access. In order to create a `ClusterCryostat`, a user must be authorized to do so for the entire cluster using a [Cluster Role Binding](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#rolebinding-and-clusterrolebinding).

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: ClusterCryostat
metadata:
  name: clustercryostat-sample
  namespace: "" # No namespace
```

### Installation Namespace
Since the `ClusterCryostat` API is cluster-scoped, we cannot use the `metadata.namespace` property to determine where Cryostat should be installed. You must instead provide the namespace where Cryostat should be installed into using the `spec.installNamespace` property. For optimal security, we suggest using a different installation namespace than the namespace where the operator is installed, and a different namespace from where your target workloads will be. This is because the operator uses a larger set of permissions compared to Cryostat itself, and Cryostat may have more permissions than your target workloads. It is considered good practice to isolate more privileged Service Accounts from less privileged users, since a user authorized to create workloads can [access any Service Account](https://kubernetes.io/docs/concepts/security/rbac-good-practices/#workload-creation) in that namespace.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: ClusterCryostat
metadata:
  name: clustercryostat-sample
spec:
  installNamespace: my-cryostat-namespace
```

### Target Namespaces
Specify the list of namespaces containing your workloads that you want your multi-namespace Cryostat installation to work with under the `spec.targetNamespaces` property. The resulting Cryostat will have permissions to access workloads only within these specified namespaces. Cryostat's own namespace (`spec.installNamespace`) is not implicitly included.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: ClusterCryostat
metadata:
  name: clustercryostat-sample
spec:
  targetNamespaces:
    - my-app-namespace
    - my-other-app-namespace
```

### Other Configuration
All the [configuration options](/docs/config.md) available to the `Cryostat` API are also applicable to the `ClusterCryostat` API.

```yaml
apiVersion: operator.cryostat.io/v1beta1
kind: ClusterCryostat
metadata:
  name: clustercryostat-sample
spec:
  minimal: false
  enableCertManager: true
```
