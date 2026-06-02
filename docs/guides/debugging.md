# Debugging

Start with Kubernetes status, then inspect operator logs, then compare with Omni.

## Check resources

```sh
kubectl get omniconnections,omniclusters,omnicontrolplanes,omniworkers,omnimachines,omnikubeconfigexports,omnihelmreleases \
  --namespace omni-cluster-operator-system
```

Use `describe` for condition messages and recent events:

```sh
kubectl describe omniconnection omni \
  --namespace omni-cluster-operator-system

kubectl describe omnicluster edge \
  --namespace omni-cluster-operator-system

kubectl describe omnikubeconfigexport cluster-01-automation-kubeconfig \
  --namespace omni-cluster-operator-system

kubectl describe omnihelmrelease cluster-01-metrics-server \
  --namespace omni-cluster-operator-system
```

## Check logs

```sh
kubectl logs deployment/omni-cluster-operator-controller-manager \
  --namespace omni-cluster-operator-system \
  --all-containers
```

## Common conditions and reasons

| Condition | What it means |
| --- | --- |
| `Reachable` | The operator can reach the configured Omni endpoint with the service account key. |
| `Stalled` | Reconciliation has hit an error that GitOps health checks should treat as failed rather than still progressing. |
| `Accepted` | A child document references an existing `OmniCluster`. |
| `Validated` | The assembled Omni template passed upstream Omni validation. |
| `Synced` | The desired template was synced to Omni. |
| `Exported` | A requested workload-cluster kubeconfig Secret was written. |
| `Ready` | The resource is ready for normal use. |

| Reason | Likely cause |
| --- | --- |
| `MissingSecret` | The referenced Secret or key does not exist in the operator namespace. |
| `ConnectionFailed` | Omni endpoint, TLS, network, or credential failure. If the message mentions Omni environment assignments, store only the `OMNI_SERVICE_ACCOUNT_KEY` value in the referenced Secret key, not the whole copied env block. |
| `MissingConnection` | `OmniCluster.spec.connectionRef.name` points at a missing `OmniConnection`. |
| `MissingCluster` | A child resource points at a missing `OmniCluster`. |
| `ValidationFailed` | The rendered Omni cluster template is not accepted by Omni validation. |
| `SyncFailed` | Omni rejected or failed the create/update operation. |
| `ExportFailed` | Omni could not issue a kubeconfig, or the returned kubeconfig could not be parsed. |
| `Suspended` | `OmniCluster.spec.suspend` is true. |
| `Deleting` | The resource is waiting for remote cleanup before finalizer removal. |
| `DeleteFailed` | Omni deletion failed. |

## Admission failures

The chart installs validating webhooks. If `kubectl apply` fails before an object is stored, read the webhook error message first. Common invalid shapes include:

- Both `machines` and `machineClass` set on a control plane or worker set.
- Neither `machines` nor `machineClass` set.
- Reserved worker set name `control-planes`.
- Invalid version strings.
- Ambiguous inline and file-backed patch or manifest sources.
- Duplicate names in `OmniCluster.spec.kubernetes.manifests[]`.
- Invalid `OmniKubeconfigExport` fields, such as blank service-account groups, `renewBefore` greater than or equal to `ttl`, or `system:masters` without `serviceAccount.allowClusterAdmin: true`.
- Invalid `OmniHelmRelease` fields, such as malformed chart values, missing kubeconfig Secret references, or direct Helm credentials that do not have workload-cluster RBAC.

## Kubeconfig export issues

`OmniKubeconfigExport` creates and rotates a target Secret only after the parent cluster and connection are available.

Check the export status:

```sh
kubectl get omnikubeconfigexport cluster-01-automation-kubeconfig \
  --namespace omni-cluster-operator-system \
  --output yaml
```

Check the target Secret metadata and key:

```sh
kubectl get secret cluster-01-automation-kubeconfig \
  --namespace omni-cluster-operator-system \
  --output jsonpath='{.metadata.annotations}{"\n"}{.data.kubeconfig}' | head
```

Common causes:

- `MissingCluster`: `spec.clusterRef.name` does not match an `OmniCluster` in the same namespace.
- `MissingConnection`: the referenced cluster points at an unavailable `OmniConnection`.
- `ExportFailed`: Omni rejected the service-account kubeconfig request, credentials are invalid, or Omni returned data that is not a kubeconfig.
- Secret consumers are reading the wrong key. The default key is `kubeconfig`; custom keys come from `spec.targetSecretRef.key`.

## Direct Helm issues

`OmniHelmRelease` reads a workload-cluster kubeconfig Secret and runs Helm actions directly in that cluster.

If a release is not ready, check the release and the referenced Secret:

```sh
kubectl get omnihelmreleases,secrets \
  --namespace omni-cluster-operator-system

kubectl describe omnihelmrelease cluster-01-metrics-server \
  --namespace omni-cluster-operator-system
```

Common causes are missing kubeconfig Secret data, insufficient workload-cluster RBAC for the exported user or group, unreachable chart repositories, invalid chart versions, invalid values, or Helm wait timeouts.

## Stuck deletion

`OmniCluster` uses a finalizer because it owns remote Omni lifecycle. If deletion is stuck:

1. Check the operator pod is running.
2. Describe the `OmniCluster` for `DeleteFailed` details.
3. Check operator logs for Omni delete errors.
4. Decide whether the remote Omni cluster should be deleted or orphaned.

To keep the remote Omni cluster and let Kubernetes deletion proceed, set orphan mode before deleting:

```sh
kubectl patch omnicluster edge \
  --namespace omni-cluster-operator-system \
  --type merge \
  --patch '{"spec":{"deletePolicy":{"orphan":true}}}'
```
