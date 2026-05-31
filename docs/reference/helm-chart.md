# Helm Chart

Published artifacts use the same version:

- Operator image: `ghcr.io/texas-hpc/omni-cluster-operator:<version>`
- CRD chart: `oci://ghcr.io/texas-hpc/charts/omni-cluster-operator-crds`
- Operator chart: `oci://ghcr.io/texas-hpc/charts/omni-cluster-operator`

## CRD chart

The CRD chart installs only the custom resource definitions.

```sh
helm install omni-cluster-operator-crds \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator-crds
```

Pin a specific version with `--version <version>` after choosing a release from the [CRD chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator-crds).

## Operator chart

The operator chart installs:

- Namespace.
- ServiceAccount.
- Namespaced RBAC.
- Manager Deployment.
- Metrics and webhook Services.
- Validating webhooks.
- cert-manager Certificate resources.

```sh
helm install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace
```

Pin a specific version with `--version <version>` after choosing a release from the [operator chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator). Use matching chart versions when installing both charts.

## Values

Inspect the current chart defaults with:

```sh
helm show values \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator
```
