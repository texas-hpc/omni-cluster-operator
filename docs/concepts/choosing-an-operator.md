# Choosing an Operator

`omni-cluster-operator` and [`talos-operator`](https://alperencelik.github.io/talos-operator/) both expose Kubernetes custom resources for Talos-related operations, but they solve different problems.

Use this project when Omni is already part of your management plane. Use `talos-operator` when you want Kubernetes-native Talos cluster lifecycle management without Omni.

## Quick decision

| If you want to... | Start with... |
| --- | --- |
| Manage Sidero Omni cluster templates from Kubernetes | `omni-cluster-operator` |
| Keep Omni as the lifecycle authority for cluster template validation, sync, status, and deletion | `omni-cluster-operator` |
| Store an Omni service account key in Kubernetes and sync rendered templates to Omni | `omni-cluster-operator` |
| Manage Talos clusters directly without an Omni instance | [`talos-operator`](https://alperencelik.github.io/talos-operator/) |
| Generate Talos configs, Talos secrets, and kubeconfigs directly from Kubernetes resources | [`talos-operator`](https://alperencelik.github.io/talos-operator/) |
| Use a Talos operator with direct metal and container modes | [`talos-operator`](https://alperencelik.github.io/talos-operator/operator_manual/modes/) |

## How this project fits

`omni-cluster-operator` is an Omni integration layer. It renders Omni cluster-template YAML from Kubernetes resources, validates that template with Omni's public Go client, syncs it to Omni, reads Omni status, and deletes the remote Omni cluster unless orphan mode is enabled.

It intentionally does not replace Omni's cluster-template behavior. The operator's Kubernetes API is a way to manage Omni desired state through GitOps and normal Kubernetes reconciliation.

This is a good fit when:

- Omni owns machine inventory, assignment, SideroLink connectivity, and cluster lifecycle.
- You want cluster settings, control planes, workers, static machines, workload kubeconfig exports, and direct Helm releases expressed as Kubernetes resources.
- Your platform workflow already treats Omni as the source of truth for Talos cluster creation.

## How talos-operator fits

`talos-operator` is a direct Talos lifecycle operator. Its documentation describes Kubernetes resources for Talos clusters, control planes, workers, machines, etcd backups, backup schedules, and Kubernetes manifest installation. It also documents metal and container modes for running Talos on machines in maintenance mode or as containerized Talos instances.

That is a good fit when:

- You do not run Omni.
- You want the operator to generate and manage Talos configuration and access secrets directly.
- You want Kubernetes resources to own Talos lifecycle actions such as bootstrap, upgrades, backups, and addon installation.

## Can they be used together?

Usually, pick one lifecycle authority for a cluster.

For an Omni-managed cluster, use `omni-cluster-operator` to manage the Omni template and let Omni apply the Talos lifecycle. For a direct Talos-managed cluster, use `talos-operator` and let that operator own the generated Talos state.

Running both against the same cluster would create overlapping ownership over Talos configuration and lifecycle actions. If you use both projects in the same environment, keep them pointed at different clusters with clear ownership boundaries.
