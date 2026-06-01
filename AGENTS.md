# Agent Notes

This is a Kubebuilder/controller-runtime Go operator for Sidero Omni cluster
templates.

## Project Rules

- Keep `github.com/siderolabs/omni/client` as the integration boundary.
- Do not import `github.com/siderolabs/omni/client/pkg/template/internal/models`;
  Go's `internal` package rule makes that unavailable to downstream modules.
- Prefer rendering Omni cluster-template YAML and delegating validation, sync,
  delete, and status behavior to `pkg/template/operations`.
- Keep child template resources bound to `OmniCluster.spec.clusterRef`; the
  `OmniCluster` owns the `OmniConnection` selection.
- The default deployment includes validating webhooks and cert-manager
  certificate resources; install cert-manager before `mise run deploy`, Tilt, or e2e.
- Use `mise run <task>` for project automation. There is intentionally no
  Makefile; executable task scripts live under `mise-tasks/`.
- Keep the live Omni fixture opt-in. `mise run omni-up` installs real Omni into
  the current Kubernetes context, accepts the local-test EULA values, and writes
  credentials under `.local/`.
- Version published artifacts from root `version.json` with NBGV. The publish
  workflow is master-only and pushes the operator image plus both OCI Helm
  charts to GHCR.
- NBGV `pathFilters` intentionally keep docs-only changes from advancing
  artifact versions. Keep publish workflow path filters aligned with
  `version.json` path filters when changing release inputs.
- For breaking changes, incompatible API/schema changes, new CRDs, meaningful
  chart behavior changes, or other minor/major release-line changes, bump the
  base `version` in `version.json` and keep `versionHeightOffsetAppliesTo`
  aligned with the new base version.
- `CHANGELOG.md` is for minor and major release notes only. When an agent makes
  or reviews a change that bumps the base minor/major version, update
  `CHANGELOG.md` under `Unreleased` in the same change. Do not require changelog
  entries for ordinary patch fixes; GitHub Releases can use generated notes for
  those.
- Keep secrets out of samples except obvious placeholders.

## Generated Files

Do not hand-edit these unless changing generation itself:

- `api/*/zz_generated.deepcopy.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`

After changing API types or markers, run:

```sh
mise run manifests
mise run generate
```

## Verification

Use the pinned tooling from `mise.toml`.

Fast loop:

```sh
mise run test-unit
```

Full local verification:

```sh
mise run test
mise run lint
mise run build
mise run samples
mise run render-default
mise run chart-lint
mise run chart-template
```

Optional real-Omni transport smoke:

```sh
mise run omni-template
mise run test-live-omni
```

`bin/`, `dist/`, `.local/`, and `cover.out` are generated local artifacts and are
intentionally ignored.
