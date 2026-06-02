# GitOps with FluxCD or Argo CD

The Omni custom resources are good GitOps objects, but they are not passive configuration. `OmniCluster` has a finalizer and performs remote Omni operations. Treat changes to these resources the same way you would treat cluster lifecycle changes in Omni itself.

## Ordering

Install dependencies before applying cluster resources:

1. cert-manager
2. `omni-cluster-operator-crds` chart
3. `omni-cluster-operator` chart
4. Omni service account Secret
5. `OmniConnection`
6. `OmniCluster` and child resources

The default operator watches only its release namespace. Put the `OmniConnection`, `OmniCluster`, `OmniControlPlane`, `OmniWorkers`, `OmniMachine`, `OmniHelmRelease`, `OmniKubeconfigExport`, and referenced Secret in that namespace unless you have changed the deployment model.

## Secrets

Do not commit real Omni service account keys. Use your normal secret workflow, such as SOPS, External Secrets Operator, Sealed Secrets, or a manually created Secret.

The referenced Secret key must contain only the base64 Omni service account key value, such as the value of `OMNI_SERVICE_ACCOUNT_KEY`. Do not store the whole copied `OMNI_ENDPOINT=...` / `OMNI_SERVICE_ACCOUNT_KEY=...` environment block in `serviceAccountKey`.

The Secret must exist before the `OmniConnection` can become ready:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniConnection
metadata:
  name: omni
  namespace: omni-cluster-operator-system
spec:
  endpoint: https://omni.example.com
  auth:
    serviceAccountSecretRef:
      name: omni-service-account
      key: serviceAccountKey
```

## Stage large changes

Use `OmniCluster.spec.suspend: true` to stage multi-object changes without syncing each intermediate state to Omni.

For example, suspend before changing the control plane, workers, machine-specific patches, managed manifests, and workload-cluster Helm releases in one pull request:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniCluster
metadata:
  name: cluster-01
  namespace: omni-cluster-operator-system
spec:
  connectionRef:
    name: omni
  kubernetes:
    version: v1.35.1
  talos:
    version: v1.13.3
  suspend: true
```

After all related resources are merged and applied, remove `suspend` or set it to `false`.

## Deletion and pruning

GitOps pruning can delete an `OmniCluster`. That is a remote Omni lifecycle operation, not just local cleanup.

Before removing an `OmniCluster` from Git, choose the intended deletion behavior:

| Intent | Configuration before deletion |
| --- | --- |
| Delete the remote Omni cluster through Omni template deletion. | Leave `spec.deletePolicy.orphan` unset or `false`. |
| Remove Kubernetes ownership but keep the remote Omni cluster. | Set `spec.deletePolicy.orphan: true`, wait for the change to apply, then remove the resource from Git. |
| Force disconnected machines to be removed during remote deletion. | Set `spec.deletePolicy.destroyMachines: true` only when that behavior is intentional. |

`orphan` and `destroyMachines` are mutually exclusive.

When deleting a full cluster from Git, delete child resources and the parent in a controlled change. If your GitOps tool prunes everything at once, the parent `OmniCluster` finalizer still controls remote deletion, but the remaining child objects may disappear from Kubernetes before the final remote operation finishes.

## Managed manifests and Helm in GitOps

`OmniCluster.spec.kubernetes.manifests` is part of the Omni cluster template. `OmniHelmRelease` is separate from that template and reconciles a Helm release directly in the workload cluster using an explicit kubeconfig Secret.

For GitOps:

- Keep `OmniCluster.spec.kubernetes.manifests[].name` values unique for each cluster.
- Put Talos settings required by workload components in `OmniCluster.spec.patches`.
- Use `OmniHelmRelease` when Helm should own install, upgrade, status, and uninstall behavior.
- Order `OmniHelmRelease` after the workload-cluster kubeconfig Secret and any workload-cluster RBAC it needs.
- Treat changes to networking, storage, or other bootstrap-critical components as staged migrations. Use `spec.suspend` while changing ownership or prerequisites.

## Kubeconfig exports in GitOps

`OmniKubeconfigExport` writes a workload-cluster kubeconfig Secret into the operator namespace only when the export resource exists. Keep these exports close to the automation that consumes them, and make the deletion policy explicit.

For GitOps:

- Do not commit exported kubeconfig Secret data.
- Use scoped service-account users and groups instead of `system:masters`.
- Set `ttl` and `renewBefore` so automation has time to reload the Secret before expiration.
- Use `deletionPolicy: Delete` when the export owns the credential lifecycle.
- Use `deletionPolicy: Orphan` only when another workflow intentionally cleans up the Secret.
- Bind the requested user or group in the workload cluster with normal Kubernetes RBAC.

If a Flux or Argo application consumes the exported Secret, order that consumer after the `OmniKubeconfigExport` resource. The export can only become ready after the referenced `OmniCluster` and its `OmniConnection` are available and Omni can issue the service-account kubeconfig.

`OmniHelmRelease` is one consumer of an exported kubeconfig Secret. Order it after the matching `OmniKubeconfigExport` and any workload-cluster RBAC that grants the exported user or group the permissions Helm needs.

## FluxCD notes

Use separate Flux `Kustomization` objects or explicit `dependsOn` ordering when the operator, secret source, and cluster resources live in different paths.

A common layout is:

```text
clusters/management/
  cert-manager/
  omni-cluster-operator/
  omni-secrets/
  omni-clusters/
```

The cluster resources should depend on the operator and secret path:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: omni-clusters
  namespace: flux-system
spec:
  interval: 10m
  path: ./clusters/management/omni-clusters
  prune: true
  wait: true
  dependsOn:
    - name: omni-cluster-operator
    - name: omni-secrets
  sourceRef:
    kind: GitRepository
    name: platform
```

Flux applies status updates from controllers outside Git. Do not commit `status` fields. If a deletion is stuck, inspect the `OmniCluster` finalizer and the operator logs before forcing removal.

## Argo CD notes

Use sync waves or separate Applications to order CRDs, the operator, secrets, and cluster resources.

Example sync-wave annotations:

```yaml
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "30"
```

Suggested waves:

| Wave | Contents |
| --- | --- |
| `0` | cert-manager |
| `10` | Omni CRD chart |
| `20` | Omni operator chart and Omni service account Secret |
| `30` | `OmniConnection`, `OmniCluster`, and child resources |

Keep pruning deliberate on Applications that contain `OmniCluster` resources. If an Argo CD Application is deleted with cascading deletion enabled, Argo can remove the Kubernetes custom resources and trigger the operator's remote Omni deletion path.

Argo CD may show generated fields, defaults, and status as live-only state. Keep manifests focused on `metadata` and `spec`; do not add controller-owned `status` fields to Git.

## Review checklist

Before merging a GitOps change that touches Omni resources, check:

- Does it create, update, pause, or delete a remote Omni cluster?
- Are CRDs, webhooks, cert-manager, and the service account Secret already present?
- Are all resources in the operator release namespace?
- Is `spec.suspend` needed while multiple resources change together?
- If anything is removed from Git, is the deletion policy intentional?
- If networking ownership changes, is the old networking cleanup, replacement install, and kube-proxy behavior accounted for?
