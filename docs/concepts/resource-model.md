# Resource Model

The API group is `omni.texashpc.com/v1alpha1`.

## Resources

| Kind | Purpose |
| --- | --- |
| `OmniConnection` | Defines one Omni endpoint and the Secret key that contains an Omni service account key. |
| `OmniCluster` | Owns the remote Omni cluster lifecycle and cluster-level template settings. |
| `OmniControlPlane` | Defines the Omni `ControlPlane` template document for one cluster. Exactly one should reference each `OmniCluster`. |
| `OmniWorkers` | Defines an Omni `Workers` template document. Multiple worker sets may reference the same cluster. |
| `OmniMachine` | Defines optional per-machine template settings for static machines. |
| `OmniCilium` | Defines an optional Cilium install rendered from Helm and injected into one `OmniCluster` template. |

## Namespace ownership

The default chart runs the operator in namespaced mode. The operator watches only the release namespace and has namespace-scoped RBAC for Omni custom resources and referenced Secrets.

Keep these objects in the release namespace:

- `OmniConnection`
- `OmniCluster`
- `OmniControlPlane`
- `OmniWorkers`
- `OmniMachine`
- `OmniCilium`
- The Secret referenced by `OmniConnection.spec.auth.serviceAccountSecretRef`

## References

`OmniCluster.spec.connectionRef.name` selects the Omni instance. Child template resources use `spec.clusterRef.name` to attach to the cluster.

Child resources do not select an `OmniConnection` directly. This keeps all template documents for a cluster bound to one Omni instance.

## Remote ownership

`OmniCluster` is the resource with remote side effects. It adds the Omni finalizer, syncs the rendered template to Omni, reads remote status, and deletes the remote Omni cluster on Kubernetes deletion unless orphan mode is enabled.

Set `spec.deletePolicy.orphan: true` when deleting the Kubernetes resources should leave the remote Omni cluster intact.
