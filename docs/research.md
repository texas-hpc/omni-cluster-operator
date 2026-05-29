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
- Helm `3.21.0`
- .NET SDK `10.0.300`
- golangci-lint `2.11.4`
- omnictl `1.8.0`
- NBGV `3.9.50` through a repo-local dotnet tool helper
- controller-gen `0.20.1` through mise's Go backend
- setup-envtest `release-0.23` through mise's Go backend

Automation is intentionally mise-native. Build, generate, test, deploy, e2e,
kind, cert-manager, and Tilt commands live as executable file tasks under
`mise-tasks/`, which keeps local development, CI, and the devcontainer on the
same pinned toolchain.

The operator runtime image is not mise-based. It uses the pinned upstream Go
builder image and a distroless runtime image; mise stays on the local development,
devcontainer, and CI side of the boundary.

Release versions come from root `version.json` using Nerdbank.GitVersioning.
Publishing is intentionally restricted to the `master` branch. The publish
workflow pushes the operator image to GHCR and packages the Helm chart as an OCI
artifact in GHCR with the same NBGV SemVer value.

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

Validating webhooks enforce per-object invariants that are awkward to maintain as
schema-only markers:

- patches and Kubernetes manifests must set exactly one source (`file` or
  `inline`)
- `deletePolicy.orphan` cannot be combined with `destroyMachines`
- machine class sizes must be positive integers or Omni's supported string
  keywords
- `workerSetName: control-planes` is reserved
- static-only kernel args are rejected on machine-class-backed sets
- rendered static machine IDs must be UUIDs

The controller also runs `operations.ValidateTemplate` before any remote sync, so
Omni remains the source of truth for full assembled-template validity. Cross-object
checks stay in reconciliation because they depend on the assembled template and,
potentially, the mounted template file root.

## Local Test Harness

The repo includes two local loops:

- fast Go unit tests against a fake controller-runtime client and fake Omni client
- kind/ctlptl/Tilt for running the manager in a real local Kubernetes API server

The kind e2e suite deploys the manager, validates CRD/webhook admission, and
checks suspended reconciliation and child status without requiring live Omni
credentials.

The optional live fixture under `hack/omni/` installs real Omni into the kind
cluster through Sidero's Helm chart and uses Dex for local OIDC. The live suite is
tagged `live_omni`; its first test creates an `OmniConnection` and waits for the
deployed controller to prove service account auth and COSI cluster listing through
the real `github.com/siderolabs/omni/client` transport. Full create/delete cluster
lifecycle tests are deliberately left behind an explicit future gate because they
need disposable Omni machines or machine classes.
