# omni-cluster-operator

`omni-cluster-operator` is a Kubernetes operator for managing Sidero Omni cluster
lifecycles from Kubernetes-native custom resources. It lets GitOps define Omni
connections, cluster template documents, and cluster deletion policy while the
controller renders and syncs an Omni cluster template through Sidero's Go client.

The operator is intentionally downstream of Omni's published client module instead
of reimplementing Omni reconciliation. It renders a deterministic template from
CRDs, validates it with `github.com/siderolabs/omni/client/pkg/template/operations`,
syncs it with `SyncTemplate`, reads status with `StatusCluster`, and removes remote
template resources with `DeleteCluster` from a Kubernetes finalizer.

## API Shape

The API group is `omni.texas-hpc.org/v1alpha1`.

- `OmniConnection` defines one Omni endpoint and a Secret reference containing an
  Omni service account key.
- `OmniCluster` owns the remote Omni cluster lifecycle, references an
  `OmniConnection`, and contains cluster-level template fields such as Kubernetes
  version, Talos version, features, patches, system extensions, kernel args,
  sync interval, and delete policy.
- `OmniControlPlane` defines the Omni `ControlPlane` template document for one
  `OmniCluster`. Exactly one control plane must reference a cluster.
- `OmniWorkers` defines one Omni `Workers` template document. Multiple worker
  sets may reference the same `OmniCluster`.
- `OmniMachine` defines an optional Omni `Machine` template document for static
  machines, including install disk and per-machine patches.

Child resources use `spec.clusterRef.name` instead of duplicating
`OmniConnectionRef`; the owning `OmniCluster` selects the Omni instance. This
keeps all template documents for one cluster bound to one connection and avoids
ambiguous cross-Omni machine ownership.

## Upstream Omni Boundary

Sidero publishes `github.com/siderolabs/omni/client`, including the gRPC/API
packages, COSI resource types, and public template operation functions. The actual
cluster-template document model structs are present under
`client/pkg/template/internal/models`, which Go intentionally prevents downstream
modules from importing.

This repo therefore keeps only the small CRD-to-YAML rendering layer locally and
delegates the drift-prone behavior to upstream Omni:

- template validation: `operations.ValidateTemplate`
- create/update reconciliation: `operations.SyncTemplate`
- delete reconciliation: `operations.DeleteCluster`
- status readback: `operations.StatusCluster`

Omni's cluster template docs also state that templates are backward compatible, so
template YAML is the most stable public contract for this operator to target.

References:

- [Omni cluster template reference](https://docs.siderolabs.com/omni/reference/cluster-templates)
- [Omni Go client module](https://pkg.go.dev/github.com/siderolabs/omni/client)
- [Omni template operations package](https://pkg.go.dev/github.com/siderolabs/omni/client/pkg/template/operations)

## Tooling

Install pinned tools through `mise`:

```sh
mise trust
mise install
```

Useful tasks:

```sh
mise run generate
mise run manifests
mise run test-unit
mise run test
mise run build
mise run kind-up
mise run tilt
mise run kind-down
```

`mise run test-unit` is the fast loop for API/template/controller unit tests.
`mise run test` runs the generated manifests/code, formatting, vet, and all
non-e2e tests.

## Local Development

Create a local kind cluster through `ctlptl`, then run Tilt:

```sh
mise run kind-up
mise run tilt
```

The Tilt setup builds `controller:latest`, applies the default Kustomize
deployment, and exposes the health endpoint on port `8081`.

Apply the sample CRs only after replacing the placeholder service account key:

```sh
kubectl apply -k config/samples
```

The Secret key defaults to `serviceAccountKey`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: omni-service-account
type: Opaque
stringData:
  serviceAccountKey: "<output from omnictl serviceaccount create>"
```

## Operator Behavior

`OmniCluster` is the resource with remote side effects. On reconcile it:

1. Adds the Omni finalizer.
2. Reads the referenced `OmniConnection`.
3. Selects child `OmniControlPlane`, `OmniWorkers`, and `OmniMachine` resources in
   the same namespace by `clusterRef`.
4. Renders deterministic Omni template YAML and stores its SHA-256 hash in status.
5. Validates the template with upstream Omni code.
6. Syncs the template to the selected Omni instance.
7. Reads Omni cluster status and updates Kubernetes conditions.
8. On deletion, calls Omni template deletion unless `spec.deletePolicy.orphan` is
   true, then removes the finalizer.

`OmniConnection` reconciles independently and reports whether the endpoint and
service account key can list Omni cluster resources. Child document controllers
report whether their referenced `OmniCluster` exists.

## Testing

Current coverage includes:

- deterministic template rendering
- upstream Omni template validation
- machine class and static machine template shapes
- cluster sync, missing-template, and delete/finalizer behavior through a fake
  Omni client
- child resource accepted/missing-cluster status handling
- manifest generation and controller build through standard Kubebuilder targets

The e2e scaffold is present under `test/e2e` and is tagged `e2e`. The next useful
expansion is a local Omni-compatible test double or self-hosted Omni fixture that
can exercise the real `github.com/siderolabs/omni/client` transport.

## Notes

This project uses Kubebuilder/controller-runtime directly. Operator SDK's own FAQ
describes Go Operator SDK projects as Kubebuilder-based and sharing the same
controller-runtime layout, with extra OLM/OperatorHub/scorecard features on top.
For this repo, those extras are not currently part of the delivery target, so the
lower-level Kubebuilder stack keeps the project simpler while preserving the path
to add OLM packaging later.
