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
| `OmniClusterAddon` | Defines an optional Helm-rendered manifest injected into one `OmniCluster` template. |
| `OmniKubeconfigExport` | Explicitly exports and rotates a workload-cluster service-account kubeconfig Secret. |
| `OmniCilium` | Legacy compatibility resource for Cilium rendering. New manifests should prefer `OmniClusterAddon` plus explicit Talos patches. |

## Namespace ownership

The default chart runs the operator in namespaced mode. The operator watches only the release namespace and has namespace-scoped RBAC for Omni custom resources and referenced Secrets.

Keep these objects in the release namespace:

- `OmniConnection`
- `OmniCluster`
- `OmniControlPlane`
- `OmniWorkers`
- `OmniMachine`
- `OmniClusterAddon`
- `OmniKubeconfigExport`
- `OmniCilium`
- The Secret referenced by `OmniConnection.spec.auth.serviceAccountSecretRef`

## References

`OmniCluster.spec.connectionRef.name` selects the Omni instance. Child template resources use `spec.clusterRef.name` to attach to the cluster.

Child resources do not select an `OmniConnection` directly. This keeps all template documents for a cluster bound to one Omni instance.

`OmniKubeconfigExport.spec.clusterRef.name` also points at an `OmniCluster`, but it is not part of the rendered Omni cluster template. It reads the parent cluster's `OmniConnection`, requests an explicit service-account kubeconfig from Omni, and writes only the requested target Secret. Use it for management-cluster automation that needs workload-cluster access; use Omni UI or `omnictl` for human kubeconfig and talosconfig downloads.

## Remote ownership

`OmniCluster` is the resource with remote side effects. It adds the Omni finalizer, syncs the rendered template to Omni, reads remote status, and deletes the remote Omni cluster on Kubernetes deletion unless orphan mode is enabled.

Set `spec.deletePolicy.orphan: true` when deleting the Kubernetes resources should leave the remote Omni cluster intact.
