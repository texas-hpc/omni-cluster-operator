# omni-cluster-operator

`omni-cluster-operator` lets platform teams manage Sidero Omni cluster templates with normal Kubernetes custom resources.

!!! warning "Independent Community Project"
    `omni-cluster-operator` is not affiliated with, sponsored by, endorsed by, or maintained by Sidero Labs. Sidero, Omni, Talos, and related names are trademarks or projects of their respective owners.

The operator runs in a Kubernetes namespace, reads an `OmniConnection`, assembles one `OmniCluster` plus its child template documents, validates the rendered Omni cluster-template YAML with Omni's public Go client, syncs it to Omni, and reports status back through Kubernetes conditions.

Use it when you want:

- GitOps-friendly Omni cluster lifecycle configuration.
- Omni service account keys stored in Kubernetes Secrets.
- Separate Kubernetes resources for cluster settings, control plane, workers, and static machines.
- Optional Cilium rendering through the operator while Omni applies raw manifests.
- Finalizer-based remote cleanup, with an orphan mode when you want to keep the Omni cluster after deleting Kubernetes resources.

## Start here

1. [Install the operator](getting-started/installation.md).
2. [Manage a cluster lifecycle](getting-started/create-a-cluster.md).
3. [Manage Cilium](getting-started/install-cilium.md), if the cluster should receive Cilium through Omni manifest sync.
4. [Configure NVIDIA GPU workers](getting-started/nvidia-gpu.md), if the cluster should run GPU workloads.
5. [Plan GitOps ordering and deletion behavior](getting-started/gitops.md).
6. [Check status and debug reconciliation](guides/debugging.md).
7. Use the [API reference](reference/api.md) when writing manifests.

## Important model

`OmniCluster` is the resource with remote side effects. It references an `OmniConnection`; child resources reference the cluster with `spec.clusterRef.name`.

```mermaid
flowchart LR
  Secret["Secret: serviceAccountKey"] --> Connection["OmniConnection"]
  Connection --> Cluster["OmniCluster"]
  ControlPlane["OmniControlPlane"] --> Cluster
  Workers["OmniWorkers"] --> Cluster
  Machine["OmniMachine"] --> Cluster
  Cluster --> Omni["Sidero Omni"]
```

All of these objects must live in the operator release namespace because the default deployment runs in namespaced mode.
