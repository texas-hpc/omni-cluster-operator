# Reconciliation

On each `OmniCluster` reconcile, the operator:

1. Adds `omni.texashpc.com/finalizer`.
2. Reads the referenced `OmniConnection`.
3. Selects child `OmniControlPlane`, `OmniWorkers`, and `OmniMachine` resources in the same namespace by `clusterRef`.
4. Renders deterministic Omni cluster-template YAML.
5. Stores the rendered template hash in status.
6. Validates the template with upstream Omni code.
7. Syncs the template to the selected Omni instance.
8. Reads Omni cluster status and updates Kubernetes conditions.
9. On deletion, calls Omni template deletion unless `spec.deletePolicy.orphan` is true.

On each `OmniConnection` reconcile, the operator adds `omni.texashpc.com/finalizer`. On deletion, the finalizer is kept while any `OmniCluster` in the same namespace still references the connection through `spec.connectionRef.name`.

## Upstream Omni boundary

The operator intentionally does not reimplement Omni's cluster-template behavior. It renders YAML and delegates drift-prone behavior to `github.com/siderolabs/omni/client/pkg/template/operations`:

- `ValidateTemplate`
- `SyncTemplate`
- `DeleteCluster`
- `StatusCluster`

This keeps the Kubernetes API focused on expressing desired state while Omni remains the source of truth for template semantics.

## Periodic sync

`OmniCluster.spec.syncInterval` controls periodic reconciliation even when no Kubernetes object changed. The default is `5m`.

## Suspension

`OmniCluster.spec.suspend: true` stops remote Omni sync. This is useful while preparing a multi-object GitOps change or investigating remote Omni errors.
