# Cryostat Operator Helm Chart

A Helm chart to deploy the Cryostat Operator on Kubernetes to automate the deployment of [Cryostat](https://cryostat.io/). Cryostat is a tool for monitoring and profiling Java applications using Java Flight Recorder (JFR).

This chart deploys Cryostat Operator version 3.0.0

## Features

- Includes CustomResourceDefinitions (CRDs) for Cryostat resources
- Manages TLS certificates for webhook server via cert-manager
- Provides Mutating and Validating Admission Webhook configurations for Cryostat CRD operations
- Defines appropriate RBAC roles and bindings for namespace and cluster-level permissions

## Installation

### Deploy with Helm
```bash
helm repo add cryostat-operator https://cryostatio.io/cryostat-operator
helm repo update
helm upgrade --install cryostat-operator cryostat-operator \
  --create-namespace -n cryostat-operator
```
### Install from local Helm chart folder

Clone the project repository and install the chart locally:

```bash
git clone https://github.com/cryostatio/cryostat-operator.git
cd cryostat-operator/helm/charts/cryostat-operator
helm install cryostat-operator . --create-namespace -n cryostat-operator
```

## Usage

After installation, manage Cryostat instances with the `Cryostat` Custom Resource. Refer to Cryostat Operator upstream documentation for CR usage.

## References

- [Cert Manager Certificate](https://cert-manager.io/docs/usage/certificate/)