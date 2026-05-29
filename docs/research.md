# Research Notes

## Operator Stack

The project uses Go, Kubebuilder, controller-runtime, and controller-gen.

The practical reason is that this operator needs to consume Sidero's published Go
client and reconcile Kubernetes CRDs. Operator SDK remains viable, but for Go it
uses Kubebuilder under the hood and adds OLM, OperatorHub, scorecard, and other
packaging-oriented layers. Those are useful when publishing an operator bundle,
but they are not required for the first GitOps/control-plane implementation.

The local stack is pinned in `mise.toml`:

- Go `1.26.3`
- Kubebuilder `4.14.0`
- kind `0.31.0`
- ctlptl `0.9.3`
- Tilt `0.37.3`
- kubectl `1.36.1`
- kustomize `5.8.1`
- golangci-lint `2.11.4`
- omnictl `1.8.0`

## Omni Client Surface

Sidero publishes `github.com/siderolabs/omni/client`. That module contains:

- `pkg/client`: Omni API client creation and service account auth.
- `api/...`: generated gRPC/API packages.
- `pkg/omni/resources/...`: public Omni/COSI resource wrappers.
- `pkg/template/operations`: public cluster-template operations used by `omnictl`.

The cluster-template document structs themselves are in
`pkg/template/internal/models`. They are visible in source and pkg.go.dev, but not
importable by this operator because Go's `internal` package rule intentionally
limits them to Omni's module.

## Chosen Integration Boundary

The operator should not directly duplicate Omni resource translation. Instead it
uses a narrow render-and-delegate boundary:

1. Kubernetes CRDs represent the public template document fields.
2. The controller renders deterministic Omni cluster-template YAML.
3. Upstream Omni code validates, translates, diffs, syncs, deletes, and reads
   status through public template operations.

That means if Omni changes internal resource translation, this operator inherits
the behavior by updating the Omni client dependency. The remaining local drift
risk is limited to the CRD field surface and YAML rendering.

## CRD Model

`OmniCluster` owns the remote lifecycle and chooses the `OmniConnection`.
Template child resources reference `OmniCluster` only:

- `OmniControlPlane.spec.clusterRef`
- `OmniWorkers.spec.clusterRef`
- `OmniMachine.spec.clusterRef`

This avoids a split-brain shape where child documents could point at a different
Omni instance than the cluster they are intended to modify.

## Validation Strategy

The first pass uses CRD schema validation plus CEL validations where the API
server can enforce invariants cheaply:

- `OmniControlPlane` and `OmniWorkers` require exactly one of `machines` or
  `machineClass`.
- Version strings are constrained to `vX.Y.Z`.
- worker set names and cluster names use Omni-compatible name patterns.

The controller also runs `operations.ValidateTemplate` before any remote sync, so
Omni remains the source of truth for full template validity. A webhook server is
scaffolded by Kubebuilder and can be added later if we need cross-object admission
checks before persistence. Today those checks are safer in reconciliation because
they depend on the assembled template and, potentially, the mounted template file
root.

## Local Test Harness

The repo includes two local loops:

- fast Go unit tests against a fake controller-runtime client and fake Omni client
- kind/ctlptl/Tilt for running the manager in a real local Kubernetes API server

The e2e scaffold is in place but still generic. A meaningful next step is a
self-hosted Omni fixture or protocol-compatible fake server that can exercise the
real `github.com/siderolabs/omni/client` transport without requiring a developer's
personal Omni SaaS account.
