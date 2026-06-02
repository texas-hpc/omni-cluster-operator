# Workload Cluster Access

Omni remains the source of workload-cluster credentials. Use Omni directly for human access, and create `OmniKubeconfigExport` only when an automation in the management cluster needs a scoped kubeconfig Secret.

## Human access

For interactive `kubectl` access, download the kubeconfig from the Omni UI or use `omnictl`:

```sh
omnictl kubeconfig --cluster <omni-cluster-name> --merge
```

For Talos access, download the talosconfig from the Omni UI or use:

```sh
omnictl talosconfig --cluster <omni-cluster-name>
```

The operator does not automatically write workload-cluster kubeconfigs or talosconfigs into the management cluster. That keeps access credentials out of Kubernetes unless you explicitly ask for a Secret export.

## Automation access

Use `OmniKubeconfigExport` when a controller, job, or GitOps workflow in the management cluster needs a workload-cluster kubeconfig Secret.

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniKubeconfigExport
metadata:
  name: cluster-01-automation-kubeconfig
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  targetSecretRef:
    name: cluster-01-automation-kubeconfig
  serviceAccount:
    user: cluster-01-automation
    groups:
      - cluster-automation
  ttl: 24h
  renewBefore: 4h
  deletionPolicy: Delete
```

The operator asks Omni for a service-account kubeconfig for the referenced cluster, stores it in `data.kubeconfig`, and rotates it before expiration when `renewBefore` is set.

`spec.clusterRef.name` points at the local `OmniCluster` resource. If `OmniCluster.spec.clusterName` is set, the operator uses that remote Omni cluster name when requesting the kubeconfig.

## Secret shape

The target Secret is created in the same namespace as the `OmniKubeconfigExport`.

```sh
kubectl get secret cluster-01-automation-kubeconfig \
  --namespace omni-cluster-operator-system \
  --output jsonpath='{.data.kubeconfig}' | base64 --decode
```

Use `spec.targetSecretRef.key` when the consumer expects a different Secret key:

```yaml
targetSecretRef:
  name: cluster-01-automation-kubeconfig
  key: workload.kubeconfig
```

The Secret is labeled and annotated with the owner export, cluster name, kubeconfig hash, spec hash, expiration time, and last rotation time. The resource status mirrors the target Secret, hash, expiration, next rotation, and last rotation fields for quick inspection.

## Rotation

Set `ttl` to the shortest lifetime that works for the consumer. Set `renewBefore` to give dependent workloads time to observe the updated Secret before the current kubeconfig expires.

```yaml
ttl: 24h
renewBefore: 4h
```

With those values, the operator rotates approximately 20 hours after the last export. If `renewBefore` is omitted, the next rotation is due at expiration.

Changing any field that affects generated kubeconfig contents, such as service-account user, groups, TTL, target key, or remote cluster name, causes a new export.

## Deletion behavior

Choose what happens to the target Secret when the `OmniKubeconfigExport` is deleted:

| Policy | Behavior |
| --- | --- |
| `Delete` | Delete the target Secret if this export owns it. |
| `Orphan` | Leave the target Secret behind. |

Use `Delete` for most generated credentials. Use `Orphan` only when another cleanup workflow intentionally owns Secret removal.

## Permissions

The generated kubeconfig uses the Kubernetes username and groups you request. It does not create Kubernetes RBAC in the workload cluster. Bind the requested group or user in the workload cluster with normal Kubernetes `RoleBinding` or `ClusterRoleBinding` objects.

`system:masters` is rejected unless `serviceAccount.allowClusterAdmin: true` is set:

```yaml
serviceAccount:
  user: cluster-01-breakglass
  groups:
    - system:masters
  allowClusterAdmin: true
```

Use that only for deliberate cluster-admin exports. Prefer a scoped group such as `cluster-automation`, with workload-cluster RBAC granting only the permissions the consumer needs.
