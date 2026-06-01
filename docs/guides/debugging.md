# Debugging

Start with Kubernetes status, then inspect operator logs, then compare with Omni.

## Check resources

```sh
kubectl get omniconnections,omniclusters,omnicontrolplanes,omniworkers,omnimachines,omniciliums \
  --namespace omni-cluster-operator-system
```

Use `describe` for condition messages and recent events:

```sh
kubectl describe omniconnection omni \
  --namespace omni-cluster-operator-system

kubectl describe omnicluster edge \
  --namespace omni-cluster-operator-system

kubectl describe omnicilium edge-cilium \
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
| `Accepted` | A child document references an existing `OmniCluster`. |
| `Validated` | The assembled Omni template passed upstream Omni validation. |
| `Synced` | The desired template was synced to Omni. |
| `Ready` | The resource is ready for normal use. |

| Reason | Likely cause |
| --- | --- |
| `MissingSecret` | The referenced Secret or key does not exist in the operator namespace. |
| `ConnectionFailed` | Omni endpoint, TLS, network, or credential failure. |
| `MissingConnection` | `OmniCluster.spec.connectionRef.name` points at a missing `OmniConnection`. |
| `MissingCluster` | A child resource points at a missing `OmniCluster`. |
| `ValidationFailed` | The rendered Omni cluster template is not accepted by Omni validation. |
| `SyncFailed` | Omni rejected or failed the create/update operation. |
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
- Invalid addon Helm repository, chart, version, or values shape.
- Duplicate manifest names between `OmniCluster.spec.kubernetes.manifests[]`, `OmniClusterAddon.spec.manifestName`, and legacy `OmniCilium.spec.manifestName`.

## Addon render issues

`OmniClusterAddon` renders a Helm chart and caches the YAML in a Secret before `OmniCluster` syncs it to Omni. Legacy `OmniCilium` uses the same pattern.

If the parent `OmniCluster` is waiting on an addon, check:

```sh
kubectl get omniclusteraddons,omniciliums,secrets \
  --namespace omni-cluster-operator-system

kubectl describe omniclusteraddon cluster-01-cilium \
  --namespace omni-cluster-operator-system
```

Generic addon rendered manifest Secrets are named `<omniclusteraddon-name>-addon-manifest`. Legacy Cilium Secrets are named `<omnicilium-name>-cilium-manifest`. Render failures usually point to an invalid chart version, unreachable chart repository, or invalid values.

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
