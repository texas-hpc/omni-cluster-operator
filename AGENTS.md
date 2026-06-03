# Agent Notes

This is a Kubebuilder/controller-runtime Go operator for Sidero Omni cluster
templates.

Before making changes, read [`CONTRIBUTING.md`](CONTRIBUTING.md) and follow its
contributor workflow, commit-message, verification, and release-versioning
guidance.

## Project Rules

- Keep `github.com/siderolabs/omni/client` as the integration boundary.
- Do not import `github.com/siderolabs/omni/client/pkg/template/internal/models`;
  Go's `internal` package rule makes that unavailable to downstream modules.
- Prefer rendering Omni cluster-template YAML and delegating validation, sync,
  delete, and status behavior to `pkg/template/operations`.
- Keep child template resources bound to `OmniCluster.spec.clusterRef`; the
  `OmniCluster` owns the `OmniConnection` selection.
- The default deployment includes validating webhooks and cert-manager
  certificate resources; install cert-manager before `task deploy`, `task dev`,
  or e2e.
- Use `task <task>` for project automation. There is intentionally no Makefile;
  task definitions live in `Taskfile.yml`.
- Keep the live Omni fixture opt-in. `task omni-up` installs real Omni into
  the current Kubernetes context, accepts the local-test EULA values, and writes
  credentials under `.local/`.
- Version published artifacts from root `version.json` with NBGV. The publish
  workflow is master-only and pushes the operator image plus both OCI Helm
  charts to GHCR.
- NBGV `pathFilters` intentionally keep docs-only changes from advancing
  artifact versions. Keep publish workflow path filters aligned with
  `version.json` path filters when changing release inputs.
- `CHANGELOG.md` is for minor and major release notes only. When an agent makes
  or reviews a change that bumps the base minor/major version, update
  `CHANGELOG.md` under `Unreleased` in the same change. Do not require changelog
  entries for ordinary patch fixes; GitHub Releases can use generated notes for
  those.
- Keep secrets out of samples except obvious placeholders.

## Release Versioning Policy

- Treat CRDs, Go API types under `api/`, admission/defaulting behavior, status
  conditions, chart values, installed RBAC/webhook resources, and documented
  samples as public release surface.
- Patch lines are only for backward-compatible fixes. Do not ship new CRDs, CRD
  schema/API changes, deprecations, migration requirements, new chart
  capabilities, or breaking behavior changes as patch-only changes.
- Minor lines are for additive or notice-giving changes: new CRDs, new optional
  fields with old-behavior defaults, new status fields or conditions, new chart
  values defaulted to existing behavior, new controller capabilities, or marking
  CRDs/fields/API versions as deprecated.
- Major lines are for incompatible public-surface changes after `1.0.0`:
  deleting or renaming CRDs, ceasing to serve API versions, changing CRD group,
  kind, plural, or scope, removing or retyping fields, adding required fields
  without defaults, narrowing enum/validation rules that can reject existing
  objects, or changing reconciliation/chart behavior in a way existing
  installations must migrate around.
- While the project is on `0.y.z`, use a new minor line for changes that would
  be major after `1.0.0`, mark them clearly as breaking in `CHANGELOG.md`, and
  never hide them in a patch release.
- When bumping the base `version`, keep `versionHeightOffsetAppliesTo` aligned
  with the new base version so the first clean `master` release on that line
  starts at `.0`.
- Deprecated CRDs, fields, and API versions must keep working in the release
  that marks them deprecated. Removing them requires a later breaking release,
  migration notes, and a stored-version/finalizer cleanup plan when Kubernetes
  may already have persisted custom resources.

## Generated Files

Do not hand-edit these unless changing generation itself:

- `api/*/zz_generated.deepcopy.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`

After changing API types or markers, run:

```sh
task manifests
task generate
```

## Verification

Use the pinned tooling from `mise.toml`.

Fast loop:

```sh
task test-unit
```

Full local verification:

```sh
task test
task lint
task build
task samples
task render-default
task chart-lint
task chart-template
```

Optional real-Omni transport smoke:

```sh
task omni-template
task test-live-omni
```

`bin/`, `dist/`, `.local/`, and `cover.out` are generated local artifacts and are
intentionally ignored.
