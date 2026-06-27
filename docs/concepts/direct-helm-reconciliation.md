# Direct Helm Reconciliation

`OmniHelmRelease` is the opt-in path for reconciling a Helm release directly in an Omni-created workload cluster.

Use it when a chart should have normal Helm release lifecycle semantics: release history, revision status, hooks, wait behavior, upgrades, rollback-on-failure, and uninstall on deletion.

Use `OmniCluster.spec.kubernetes.manifests` when you want Omni manifest sync to apply raw Kubernetes objects through the cluster template.

## Design decision

The direct Helm path is a separate CRD rather than a field on `OmniCluster` because it is not part of the Omni template model.

`OmniHelmRelease` is not part of the Omni template. It reads an explicit workload-cluster kubeconfig Secret, connects to the workload cluster, and runs Helm install, upgrade, status, and uninstall actions there.

Keeping raw manifest sync and direct Helm reconciliation separate avoids mixing two different sources of lifecycle truth in one API.

| Resource path | Workload-cluster credentials | Source of lifecycle truth | Delete behavior |
| --- | --- | --- | --- |
| `OmniCluster.spec.kubernetes.manifests` | Not required | Omni manifest sync | Removes the manifest entry from the Omni template; workload-cluster cleanup depends on Omni manifest sync behavior. |
| `OmniHelmRelease` | Required | Helm release state in the workload cluster | Runs `helm uninstall` by default, or orphans the release when requested. |

## Credential boundary

`OmniHelmRelease` never exports credentials itself. It only reads a Secret you explicitly name:

```yaml
spec:
  kubeconfigSecretRef:
    name: edge-sample-automation-kubeconfig
    key: kubeconfig
```

Create that Secret with `OmniKubeconfigExport`, the Omni UI, `omnictl`, or another controlled process. The recommended operator-native path is `OmniKubeconfigExport` because it makes the export explicit, scoped, rotating, and visible in Kubernetes status.

The kubeconfig's user and groups determine what Helm can do in the workload cluster. `OmniHelmRelease` does not create workload-cluster RBAC. Bind the exported user or group to the minimum permissions the chart needs.

## Helm implementation

The operator uses the existing `helm.sh/helm/v4` dependency directly.

That keeps the integration boundary small:

- Helm release storage remains in the workload cluster.
- Install and upgrade use Helm's action APIs.
- Missing release history causes install; existing history causes upgrade.
- Delete with `deletionPolicy: Uninstall` runs Helm uninstall.
- Delete with `deletionPolicy: Orphan` removes only the management-cluster CR.

The controller reports the last action, release revision, release status, chart, namespace, timestamps, last error, and `Ready`/`Released` conditions.

## Create a direct Helm release

First create or reference a workload-cluster kubeconfig Secret. With `OmniKubeconfigExport`, that can look like this:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniKubeconfigExport
metadata:
  name: edge-sample-automation-kubeconfig
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: edge-sample
  targetSecretRef:
    name: edge-sample-automation-kubeconfig
  serviceAccount:
    user: edge-sample-automation
    groups:
      - cluster-automation
  ttl: 24h
  renewBefore: 4h
  deletionPolicy: Delete
```

Then reference that Secret from `OmniHelmRelease`:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniHelmRelease
metadata:
  name: edge-sample-metrics-server
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: edge-sample
  kubeconfigSecretRef:
    name: edge-sample-automation-kubeconfig
  releaseName: metrics-server
  namespace: kube-system
  wait: true
  timeout: 5m
  deletionPolicy: Uninstall
  chart:
    repository: https://kubernetes-sigs.github.io/metrics-server/
    chart: metrics-server
    version: 3.13.0
    values:
      replicas: 2
```

For OCI charts, set `spec.chart.chart` to the full `oci://` chart reference.
The `spec.chart.repository` field is still required by the v1alpha1 schema, so
set it to the same OCI reference; the controller ignores it for Helm lookup when
`spec.chart.chart` starts with `oci://`.

## Migration from raw manifest sync

Do not manage the same application through raw Omni manifest sync and `OmniHelmRelease` at the same time unless you have a deliberate handoff plan.

A typical migration is:

1. Create a scoped kubeconfig export and workload-cluster RBAC for Helm.
2. Remove or disable the raw manifest-sync entry in `OmniCluster.spec.kubernetes.manifests`.
3. Verify Omni has stopped managing those rendered objects.
4. Create `OmniHelmRelease` for the same chart, release name, namespace, and values.
5. Confirm the `Released` and `Ready` conditions and inspect Helm release history in the workload cluster.

For charts with non-idempotent generated output, direct Helm reconciliation avoids the render-cache workaround because Helm owns the release state.
