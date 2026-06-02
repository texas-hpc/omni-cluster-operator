# API Reference

This page summarizes the user-facing fields. The generated CRDs under `config/crd/bases` are the source of truth for Kubernetes schema validation.

## OmniConnection

Defines one Omni endpoint and the service account key used by the operator.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.endpoint` | Yes | Omni API URL. Must start with `http://`, `https://`, or `grpc://`. |
| `spec.auth.serviceAccountSecretRef.name` | Yes | Secret name in the same namespace. |
| `spec.auth.serviceAccountSecretRef.key` | Yes | Secret key. Defaults to `serviceAccountKey`. |
| `spec.insecureSkipTLSVerify` | No | Disables TLS verification. Use only for local development. |

Status includes `Ready`, `Reachable`, `endpoint`, `connectionRef`, `observedGeneration`, and `lastCheckTime`.

## OmniCluster

Owns remote Omni cluster lifecycle and cluster-level template settings.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.connectionRef.name` | Yes | `OmniConnection` in the same namespace. |
| `spec.clusterName` | No | Remote Omni cluster name. Defaults to `metadata.name`. |
| `spec.kubernetes.version` | Yes | Kubernetes version such as `v1.35.0`. |
| `spec.kubernetes.manifests` | No | Omni-managed Kubernetes manifests, either inline or file-backed. |
| `spec.talos.version` | Yes | Talos version such as `v1.13.2`. |
| `spec.features` | No | Optional workload proxy, embedded discovery, disk encryption, and backup settings. |
| `spec.patches` | No | Cluster-scope Talos machine configuration patches. |
| `spec.systemExtensions` | No | System extensions installed on every machine. |
| `spec.kernelArgs` | No | Kernel args for static machines. |
| `spec.templateRoot` | No | Directory inside the operator container for file-backed patches and manifests. |
| `spec.deletePolicy.orphan` | No | Leave the remote Omni cluster intact on Kubernetes deletion. |
| `spec.deletePolicy.destroyMachines` | No | Forcefully remove disconnected nodes while deleting template resources. |
| `spec.syncInterval` | No | Periodic reconciliation interval. Defaults to `5m`. |
| `spec.suspend` | No | Stop remote Omni sync while keeping resources and finalizers. |

Status includes `Ready`, `Validated`, `Synced`, selected child references, rendered template hash, remote cluster phase, and observed generation.

## OmniControlPlane

Defines the Omni `ControlPlane` template document. Exactly one `OmniControlPlane` should reference each `OmniCluster`.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.clusterRef.name` | Yes | `OmniCluster` in the same namespace. |
| `spec.machines` | Conditional | Explicit Omni machine IDs. Mutually exclusive with `machineClass`. |
| `spec.machineClass.name` | Conditional | Omni MachineClass name. Mutually exclusive with `machines`. |
| `spec.machineClass.size` | Conditional | Number of machines or an Omni size keyword such as `unlimited`. |
| `spec.patches` | No | Machine-set patches. |
| `spec.systemExtensions` | No | Extensions for every machine in the set. |
| `spec.kernelArgs` | No | Kernel args for static machines in the set. |
| `spec.bootstrapSpec` | No | Restore bootstrap settings. |

## OmniWorkers

Defines one Omni `Workers` template document.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.clusterRef.name` | Yes | `OmniCluster` in the same namespace. |
| `spec.workerSetName` | No | Remote worker set name. Defaults to `metadata.name`; cannot be `control-planes`. |
| `spec.machines` | Conditional | Explicit Omni machine IDs. Mutually exclusive with `machineClass`. |
| `spec.machineClass.name` | Conditional | Omni MachineClass name. Mutually exclusive with `machines`. |
| `spec.machineClass.size` | Conditional | Number of machines or an Omni size keyword such as `unlimited`. |
| `spec.patches` | No | Machine-set patches. |
| `spec.systemExtensions` | No | Extensions for every machine in the set. |
| `spec.kernelArgs` | No | Kernel args for static machines in the set. |
| `spec.updateStrategy` | No | Config update behavior. |
| `spec.upgradeStrategy` | No | Version, extension, and kernel arg upgrade behavior. |
| `spec.deleteStrategy` | No | Machine removal behavior. |

## OmniMachine

Defines optional per-machine settings for a static machine.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.clusterRef.name` | Yes | `OmniCluster` in the same namespace. |
| `spec.machineID` | No | Omni machine ID. Defaults to `metadata.name`. |
| `spec.locked` | No | Prevents config updates, upgrades, and downgrades. Omni allows locked machines only as workers. |
| `spec.install.disk` | No | Talos install disk path. |
| `spec.patches` | No | Machine-specific patches. |
| `spec.systemExtensions` | No | Machine-specific system extensions. |
| `spec.kernelArgs` | No | Machine-specific kernel args. |

## OmniClusterAddon

Defines a generic Helm-rendered manifest for one `OmniCluster`.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.clusterRef.name` | Yes | `OmniCluster` in the same namespace. |
| `spec.manifestName` | No | Omni manifest sync entry name. Defaults to `metadata.name`. |
| `spec.mode` | No | Omni manifest apply mode, `full` or `one-time`. Defaults to `full`. |
| `spec.helm.repository` | Yes | Helm repository URL. |
| `spec.helm.chart` | Yes | Helm chart name to render. |
| `spec.helm.version` | Yes | Helm chart version to render. |
| `spec.helm.releaseName` | No | Helm release name used while rendering. Defaults to `metadata.name`. |
| `spec.helm.namespace` | No | Namespace for rendered objects. Defaults to `default`. |
| `spec.helm.values` | No | Helm values object passed to the renderer. |

Status includes rendered manifest Secret name, rendered manifest hash, manifest name, chart, chart version, and last render time.

## OmniKubeconfigExport

Exports a scoped workload-cluster service-account kubeconfig into a Secret only when explicitly requested.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.clusterRef.name` | Yes | `OmniCluster` in the same namespace. |
| `spec.targetSecretRef.name` | Yes | Target Secret in the same namespace. |
| `spec.targetSecretRef.key` | No | Secret data key. Defaults to `kubeconfig`. |
| `spec.serviceAccount.user` | Yes | Kubernetes username for the generated kubeconfig. |
| `spec.serviceAccount.groups` | Yes | Kubernetes groups for the generated kubeconfig. `system:masters` requires `allowClusterAdmin: true`. |
| `spec.serviceAccount.allowClusterAdmin` | No | Allows `system:masters`. Leave false for scoped automation credentials. |
| `spec.ttl` | Yes | Requested service-account kubeconfig lifetime, such as `24h`. |
| `spec.renewBefore` | No | Rotate before expiration, such as `4h`. Must be less than `ttl`. |
| `spec.deletionPolicy` | Yes | `Delete` removes the target Secret on deletion; `Orphan` leaves it behind. |

Status includes `Ready`, `Accepted`, `Exported`, target Secret name/key, kubeconfig hash, expiration time, next rotation time, and last rotation time.

The target Secret is created in the same namespace as the export. The default data key is `kubeconfig`. Secret labels and annotations include the owning export UID/name, remote cluster name, generated kubeconfig hash, export spec hash, expiration time, and last rotation time.

Changing the service-account user, groups, TTL, target key, or remote cluster name changes the export spec hash and causes a new kubeconfig request. `renewBefore`, target Secret name, and deletion policy affect rotation or cleanup behavior but do not change the generated kubeconfig identity.

## OmniCilium

Deprecated. Defines a legacy Cilium install for one `OmniCluster`. New Cilium manifests should prefer `OmniClusterAddon` plus explicit Talos patches on `OmniCluster`.

| Field | Required | Notes |
| --- | --- | --- |
| `spec.clusterRef.name` | Yes | `OmniCluster` in the same namespace. |
| `spec.chartVersion` | Yes | Cilium Helm chart version to render, such as `1.19.3`. |
| `spec.chartRepository` | No | Helm repository URL. Defaults to `https://helm.cilium.io/`. |
| `spec.releaseName` | No | Helm release name used while rendering. Defaults to `cilium`. |
| `spec.namespace` | No | Namespace for rendered Cilium objects. Defaults to `kube-system`. |
| `spec.manifestName` | No | Omni manifest sync entry name. Defaults to `cilium`. |
| `spec.mode` | No | Omni manifest apply mode, `full` or `one-time`. Defaults to `full`. |
| `spec.values` | No | Cilium Helm values merged over the operator's Talos-compatible defaults. |

Status includes rendered manifest Secret name, rendered manifest hash, manifest name, chart version, kube-proxy replacement state, and last render time.

## Common nested fields

### Patches

Each patch may use `inline` or `file`.

| Field | Notes |
| --- | --- |
| `file` | Path relative to `OmniCluster.spec.templateRoot` in the operator container. |
| `name` | Human-readable patch name. |
| `idOverride` | Overrides Omni's generated config patch ID. |
| `labels` | Labels applied to the generated config patch. |
| `annotations` | Annotations applied to the generated config patch. |
| `inline` | Talos strategic machine configuration patch. |

### Update strategies

`updateStrategy`, `upgradeStrategy`, and `deleteStrategy` use the same shape:

```yaml
type: Rolling
rolling:
  maxParallelism: 1
```

`type` may be `Rolling` or `Unset`. When unset, Omni applies the operation at once.
