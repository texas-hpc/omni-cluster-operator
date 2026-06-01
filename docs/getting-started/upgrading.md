# Upgrading

Upgrade the CRD chart first, then upgrade the operator chart.

```sh
helm upgrade --install omni-cluster-operator-crds \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator-crds
```

```sh
helm upgrade --install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace
```

If you want a specific release, choose versions from the [CRD chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator-crds) and [operator chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator), then pass `--version <chart-version>` to both commands. When pinning versions, use matching versions for the two charts.

## Before upgrading

- Confirm cert-manager is installed and healthy.
- Read the release notes for API or webhook validation changes.
- Back up GitOps manifests for `omni.texashpc.com` resources.
- Check that all existing resources are reconciled before changing the controller.

```sh
kubectl get omniconnections,omniclusters,omnicontrolplanes,omniworkers,omnimachines \
  --namespace omni-cluster-operator-system
```

## Image overrides

For a branch build or unreleased test image, override the image tag:

```sh
helm upgrade --install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace \
  --set image.tag=dev
```

If the test image expects a specific chart release, add `--version <chart-version>`.

## Rollback

If the operator deployment fails after an upgrade, roll back the operator chart first:

```sh
helm rollback omni-cluster-operator \
  --namespace omni-cluster-operator-system
```

Only roll back CRDs when you know the previous CRD version is compatible with the objects already stored in the Kubernetes API server.
