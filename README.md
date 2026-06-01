# omni-cluster-operator

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/omni-cluster-operator-crds)](https://artifacthub.io/packages/search?repo=omni-cluster-operator-crds)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/omni-cluster-operator)](https://artifacthub.io/packages/search?repo=omni-cluster-operator)

Manage Sidero Omni cluster templates from Kubernetes.

> [!IMPORTANT]
> `omni-cluster-operator` is an independent community project. It is not
> affiliated with, sponsored by, endorsed by, or maintained by Sidero Labs.
> Sidero, Omni, Talos, and related names are trademarks or projects of their
> respective owners.

`omni-cluster-operator` gives platform teams a GitOps-friendly way to define
Omni connections, cluster templates, machine groups, and deletion policy with
normal Kubernetes custom resources.

Full user documentation is available at
[texas-hpc.github.io/omni-cluster-operator](https://texas-hpc.github.io/omni-cluster-operator/).

## Installation

You need:

- a Kubernetes cluster where the operator will run
- Helm 3 or newer
- cert-manager installed in the target cluster
- access to an Omni instance
- an Omni service account key from the Omni UI or `omnictl serviceaccount create`

Install the CRDs first:

```sh
helm install omni-cluster-operator-crds \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator-crds
```

Then install the operator:

```sh
helm install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace
```

By default, Helm installs the latest chart version it can resolve from the OCI
registry. To pin a specific release, choose matching versions from the
[operator chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator)
and
[CRD chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator-crds),
then add `--version <chart-version>` to both commands.

## Omni Credentials

Create the Omni service account Secret in the operator release namespace. Do not
commit the real service account key to Git.

```sh
kubectl create secret generic omni-service-account \
  --namespace omni-cluster-operator-system \
  --from-literal=serviceAccountKey='<omni service account key>'
```

## Verify

```sh
kubectl get pods,svc,certificate \
  --namespace omni-cluster-operator-system

kubectl get crd | grep omni.texas-hpc.org
```

The operator runs in namespaced mode. Create `OmniConnection`, `OmniCluster`,
and child resources in the operator release namespace unless you customize the
deployment.

## Documentation

- [Installation](https://texas-hpc.github.io/omni-cluster-operator/getting-started/installation/)
- [Cluster lifecycle](https://texas-hpc.github.io/omni-cluster-operator/getting-started/create-a-cluster/)
- [Manage Cilium](https://texas-hpc.github.io/omni-cluster-operator/getting-started/install-cilium/)
- [NVIDIA GPU workloads](https://texas-hpc.github.io/omni-cluster-operator/getting-started/nvidia-gpu/)
- [GitOps](https://texas-hpc.github.io/omni-cluster-operator/getting-started/gitops/)
- [API reference](https://texas-hpc.github.io/omni-cluster-operator/reference/api/)
- [Debugging](https://texas-hpc.github.io/omni-cluster-operator/guides/debugging/)
- [Contributing](CONTRIBUTING.md)

## License

The source code in this repository is licensed under the Apache License,
Version 2.0. See [LICENSE](LICENSE).

Third-party dependencies, including Sidero Labs libraries used by the operator,
remain under their own licenses.

## Conduct

Participation in this project is covered by the
[Code of Conduct](CODE_OF_CONDUCT.md).
