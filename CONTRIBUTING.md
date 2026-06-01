# Contributing

Thanks for improving `omni-cluster-operator`. This project is a
Kubebuilder/controller-runtime operator for managing Sidero Omni cluster
templates from Kubernetes custom resources.

## Development Setup

This project requires [`mise`](https://mise.jdx.dev/) for toolchain management
and project automation. Install it before running repository tasks. Common
installation options include:

```sh
# macOS, via Homebrew
brew install mise

# Linux/macOS, via the mise installer
curl https://mise.run | sh
```

See the official
[`mise` installation guide](https://mise.jdx.dev/installing-mise.html) for
other package managers and shell activation steps.

After installing `mise`, install the pinned project toolchain:

```sh
mise trust
mise install
```

There is intentionally no Makefile. Use `mise run <task>` for project
automation. Executable task scripts live in `mise-tasks/`.

For local Kubernetes development:

```sh
mise run kind-up
mise run cert-manager-up
mise run tilt
```

The default deployment includes validating webhooks and cert-manager
`Certificate` resources, so cert-manager must be installed before deploying the
operator, running Tilt, or running e2e tests.

## Project Boundaries

- Keep `github.com/siderolabs/omni/client` as the Omni integration boundary.
- Do not import `github.com/siderolabs/omni/client/pkg/template/internal/models`;
  Go's `internal` package rule makes that unavailable to downstream modules.
- Prefer rendering Omni cluster-template YAML and delegating validation, sync,
  delete, and status behavior to `pkg/template/operations`.
- Keep child template resources bound to `OmniCluster.spec.clusterRef`.
  `OmniCluster` owns the `OmniConnection` selection.
- Keep the live Omni fixture opt-in. `mise run omni-up` installs a real Omni
  fixture into the current Kubernetes context, accepts the local-test EULA
  values, and writes credentials under `.local/`.
- Keep secrets out of samples except obvious placeholders.

## Generated Files

Do not hand-edit generated files unless you are changing generation itself:

- `api/*/zz_generated.deepcopy.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`

After changing API types or Kubebuilder markers, regenerate manifests and code:

```sh
mise run manifests
mise run generate
```

If chart CRD packaging is affected, also sync chart CRDs:

```sh
mise run chart-sync-crds
```

## Testing

Use the fast loop while iterating:

```sh
mise run test-unit
```

Before opening or updating a pull request, run the full local verification set
that matches CI coverage:

```sh
mise run test
mise run lint
mise run build
mise run samples
mise run render-default
mise run chart-lint
mise run chart-template
mise run docs-build
```

Optional real-Omni transport smoke tests stay explicit:

```sh
mise run omni-template
mise run test-live-omni
```

## GitHub Collaborator Access

This repository expects contributors who raise pull requests or make sustained
project changes to be GitHub collaborators on the repository. Collaborator
access keeps pull request authorship, CI access, review requests, branch
permissions, and follow-up maintenance in the same GitHub security boundary.

If you are not already a collaborator, request access before starting a change
that you expect to submit upstream:

1. Open a GitHub issue describing the change you want to make, why it belongs in
   this repository, and any operator, Omni, Kubernetes, or documentation context
   you already have.
2. Ask a current maintainer to sponsor collaborator access in that issue.
3. Wait for a maintainer to confirm the contribution scope and send a GitHub
   repository invitation.
4. Accept the invitation, then create your branch and pull request from the
   repository using the normal verification steps above.

Small reports, questions, and issue comments do not require collaborator access,
but pull requests and ongoing contribution work do.

## Documentation

Contributor workflow details live in
[`docs/guides/local-development.md`](docs/guides/local-development.md). The
real-Omni fixture is documented in
[`docs/local-omni-fixture.md`](docs/local-omni-fixture.md).

Build the documentation site before changing docs:

```sh
mise run docs-build
```

Serve it locally when reviewing rendered pages:

```sh
mise run docs-serve
```

## Releases

Published artifact versions come from root `version.json` through NBGV. The
publish workflow runs from `master` and pushes both the operator image and OCI
Helm charts to GHCR.

Use the pinned NBGV tool directly through the existing tasks and workflows.
Do not add a repo-local wrapper around `nbgv`.

### Version bumps are contributor responsibility

If your change is user-facing, compatibility-affecting, or release-shaping, you
are responsible for deciding whether `version.json` must move to a new minor or
major version before the pull request merges.

This is not optional for big changes. A pull request with breaking behavior,
incompatible API/schema changes, or a release-line-worthy feature is not ready
to merge until the version bump is included or the maintainer explicitly decides
that the current release line is still correct.

Do not leave this for the publish workflow to infer. NBGV can count artifact
commits, but it cannot know whether an API change is breaking, whether a chart
behavior change deserves a minor release, or whether stored Kubernetes resources
need a major-version warning. That judgment belongs in the pull request.

Bump the base `version` in `version.json` when the current release line is no
longer appropriate:

- Use a minor bump for new CRDs, new fields, new chart capabilities, meaningful
  behavior changes, or non-breaking operator features users should notice.
- Use a major bump for breaking API changes, incompatible CRD/schema changes,
  disruptive chart/RBAC changes, migration requirements, or behavior changes
  that can surprise existing installations.
- Keep patch-level artifact versions on the current line for small compatible
  fixes. NBGV will produce patch movement from filtered artifact commits.

When changing the base version, keep `versionHeightOffsetAppliesTo` aligned with
the new base version so the first clean `master` release on that line starts at
`.0`.

Update [`CHANGELOG.md`](CHANGELOG.md) only when the pull request bumps the base
minor or major version. Patch-level fixes do not need manual changelog entries;
the GitHub Release can use generated commit notes for those.
