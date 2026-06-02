# omni-cluster-operator

<img src="docs/assets/patch-logo.png" alt="Patch, the omni-cluster-operator mascot" width="128" align="right">

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/omni-cluster-operator-crds)](https://artifacthub.io/packages/search?repo=omni-cluster-operator-crds)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/omni-cluster-operator)](https://artifacthub.io/packages/search?repo=omni-cluster-operator)

Manage Sidero Omni cluster templates from Kubernetes, without replacing Omni.

> [!IMPORTANT]
> `omni-cluster-operator` is an independent community project. It is not
> affiliated with, sponsored by, endorsed by, or maintained by Sidero Labs.
> Sidero, Omni, Talos, and related names are trademarks or projects of their
> respective owners.

`omni-cluster-operator` gives platform teams a GitOps-friendly way to define
Omni connections, cluster templates, machine groups, and deletion policy with
normal Kubernetes custom resources while keeping Omni as the lifecycle authority.

Patch is the `omni-cluster-operator` mascot: an armadillo with a keyboard and a
job to do. We picked an armadillo because it is small, armored, persistent, and
close to the ground, the same attitude this operator takes toward turning
Kubernetes resources into steady Omni cluster-template updates.

## Which Operator Should I Use?

Use `omni-cluster-operator` when Omni is part of your management plane and you
want Kubernetes resources to render, validate, sync, and delete Omni cluster
templates.

If you want to manage Talos Linux clusters directly without Omni, consider
[`talos-operator`](https://alperencelik.github.io/talos-operator/) instead. It
provides Kubernetes custom resources for Talos cluster lifecycle management,
including direct Talos configuration, upgrades, backups, and generated access
secrets. See the docs site's
[Choosing an Operator](https://texas-hpc.github.io/omni-cluster-operator/concepts/choosing-an-operator/)
page for a more detailed comparison.

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

kubectl get crd | grep omni.texashpc.com
```

The operator runs in namespaced mode. Create `OmniConnection`, `OmniCluster`,
child resources, explicit kubeconfig exports, and direct Helm releases in the
operator release namespace unless you customize the deployment.

## Documentation

- [Installation](https://texas-hpc.github.io/omni-cluster-operator/getting-started/installation/)
- [Cluster lifecycle](https://texas-hpc.github.io/omni-cluster-operator/getting-started/create-a-cluster/)
- [Workload access](https://texas-hpc.github.io/omni-cluster-operator/getting-started/workload-access/)
- [Direct Helm reconciliation](https://texas-hpc.github.io/omni-cluster-operator/concepts/direct-helm-reconciliation/)
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
