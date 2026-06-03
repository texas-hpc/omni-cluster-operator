# Contributing

Thanks for improving `omni-cluster-operator`. This project is a
Kubebuilder/controller-runtime operator for managing Sidero Omni cluster
templates from Kubernetes custom resources.

## Development Setup

This project uses [`mise`](https://mise.jdx.dev/) for toolchain management and
[Task](https://taskfile.dev/) for project automation. Install `mise` before
running repository tasks. Common installation options include:

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

There is intentionally no Makefile. Use `task <task>` for project automation.
Task definitions live in `Taskfile.yml`.

For local Kubernetes development:

```sh
task kind-up
task cert-manager-up
task tilt
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
- Keep the live Omni fixture opt-in. `task omni-up` installs a real Omni
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
task manifests
task generate
```

If chart CRD packaging is affected, also sync chart CRDs:

```sh
task chart-sync-crds
```

## Testing

Use the fast loop while iterating:

```sh
task test-unit
```

Before opening or updating a pull request, run the full local verification set
that matches CI coverage:

```sh
task test
task lint
task build
task samples
task render-default
task chart-lint
task chart-template
task docs-build
```

Optional real-Omni transport smoke tests stay explicit:

```sh
task omni-template
task test-live-omni
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
task docs-build
```

Serve it locally when reviewing rendered pages:

```sh
task docs-serve
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

This project follows [Semantic Versioning](https://semver.org/), the Kubernetes
[API versioning](https://kubernetes.io/docs/reference/using-api/) and
[deprecation](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
model, and Kubernetes
[CRD versioning](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/)
rules. In practical operator terms: CRDs are user-facing APIs, deprecations are
release-line changes, and stored custom resources need explicit migration
planning.

This matches established operator guidance:
[Operator SDK](https://sdk.operatorframework.io/docs/best-practices/best-practices/)
recommends SemVer for operators and Kubernetes API versioning for CRDs,
[cert-manager](https://cert-manager.io/docs/contributing/api-compatibility/)
follows upstream Kubernetes API compatibility,
[Argo CD](https://argo-cd.readthedocs.io/en/stable/operator-manual/upgrading/overview/)
keeps patch releases non-breaking, and
[ECK](https://www.elastic.co/docs/deploy-manage/upgrade/orchestrator/upgrade-cloud-on-k8s)
documents CRD/operator upgrade ordering explicitly.

#### Public release surface

For versioning decisions, treat all of the following as public release surface:

- CRD group, version, kind, plural, scope, categories, short names, subresources,
  schemas, defaults, validation, enum values, `spec`, `status`, status
  conditions, and additional printer columns.
- Go API types under `api/`, because they generate the CRDs and may be imported
  by downstream tooling.
- Validating/defaulting webhook behavior and any compatibility assumptions it
  creates for existing custom resources.
- Helm chart values, rendered install resources, RBAC, webhook configuration,
  cert-manager resources, container args, and environment variables.
- Documented examples, sample manifests, upgrade instructions, and user-facing
  CLI/task workflows.

Internal controller implementation, tests, generated output, and build plumbing
are not public API by themselves, but changes to them still require a version
bump when they alter the public release surface above.

#### Patch, minor, and major lines

Keep patch-level artifact versions on the current line for compatible fixes
only. Examples include reconcile bug fixes that preserve the API contract,
security fixes, dependency updates that do not change supported behavior,
compatible documentation fixes, and build/test maintenance. NBGV will produce
patch movement from filtered artifact commits.

Use a minor bump for additive or notice-giving changes:

- New CRDs or controllers.
- New optional `spec` fields with defaults that preserve old behavior.
- New status fields, status conditions, printer columns, or events.
- New chart values or install options that default to current behavior.
- New controller behavior users can opt into without migrating existing objects.
- Deprecating a CRD, API version, field, enum value, chart value, or documented
  workflow while keeping it functional.

Use a major bump for incompatible public-surface changes after `1.0.0`:

- Deleting or renaming a CRD.
- Changing CRD group, kind, plural, scope, or served API versions.
- Removing, renaming, retyping, or repurposing fields.
- Adding required fields without safe defaults or conversion.
- Narrowing schemas, enum values, validation, or admission behavior in a way
  that can reject existing objects or manifests.
- Changing defaults or reconciliation semantics in a way existing installations
  must migrate around.
- Removing chart values, changing install scope, or changing RBAC/webhook
  behavior in a disruptive way.

While the project is still on `0.y.z`, do not use patch releases for breaking
changes. Use a new minor line for changes that would be major after `1.0.0`,
mark the release notes clearly as breaking, and include migration guidance. Only
move to `1.0.0` when maintainers intentionally declare the public API stable.

When changing the base version, keep `versionHeightOffsetAppliesTo` aligned with
the new base version so the first clean `master` release on that line starts at
`.0`.

Update [`CHANGELOG.md`](CHANGELOG.md) only when the pull request bumps the base
minor or major version. Patch-level fixes do not need manual changelog entries;
the GitHub Release can use generated commit notes for those.

#### CRD change decisions

Use this table when a pull request touches `api/`, Kubebuilder markers,
generated CRDs, chart CRD packaging, admission behavior, or documented custom
resources.

| Change | Version action | Required work |
| --- | --- | --- |
| Add a new CRD/GVK | Minor | Add controller/RBAC/chart CRD/docs/tests, sync chart CRDs, update `CHANGELOG.md`. |
| Add an optional `spec` field | Minor | Preserve old behavior by default; document the field and add validation/tests. |
| Add a required `spec` field | Major after `1.0.0`; breaking minor on `0.y.z` | Prefer a new API version with conversion/defaulting; include migration notes. |
| Add status fields, status conditions, or printer columns | Minor | Keep existing status fields and condition semantics intact. |
| Fix controller behavior without changing schema or user-visible semantics | Patch | Add or update focused tests. |
| Loosen validation or accept a new enum value | Minor | Confirm existing objects remain valid. |
| Tighten validation, remove enum values, or reject objects that used to pass | Major after `1.0.0`; breaking minor on `0.y.z` | Add migration guidance and compatibility tests. |
| Rename, remove, retype, or repurpose a field | Major after `1.0.0`; breaking minor on `0.y.z` | Prefer a new API version and conversion path; never silently reuse old meaning. |
| Change defaults or reconciliation semantics for existing objects | Major after `1.0.0`; breaking minor on `0.y.z` | Document impact, migration, and rollback behavior. |
| Mark a CRD, API version, field, enum value, or chart value deprecated | Minor | Keep it working; add replacement guidance, warnings where Kubernetes supports them, and `CHANGELOG.md` notes. |
| Stop serving or remove an API version | Major after `1.0.0`; breaking minor on `0.y.z` | Require prior deprecation except for emergency fixes; verify stored-version migration before removal. |
| Delete a CRD/GVK | Major after `1.0.0`; breaking minor on `0.y.z` | Require prior deprecation except for emergency fixes; include cleanup, finalizer, and migration guidance. |
| Change CRD group, kind, plural, or scope | Treat as delete plus new CRD | Include migration tooling or explicit re-apply/delete guidance. |

#### Deprecation and removal process

Deprecation is a promise to keep the old surface working for the deprecation
release. Do not deprecate and remove the same public API in one compatible
release.

For CRD API versions, prefer the Kubernetes multi-version flow:

1. Add the replacement version with `served: true`.
2. Use `strategy: None` only when schemas are equivalent; otherwise add a
   conversion webhook before data can move safely between versions.
3. Mark the old version deprecated and provide a warning message where
   Kubernetes supports it.
4. Give users migration instructions and keep both versions served for the
   deprecation release.
5. Before removal, ensure stored objects have migrated off the old storage
   version and the old version is gone from CRD `status.storedVersions`.
6. Only then set `served: false`, remove the version from the CRD, and drop
   conversion support in the same breaking release.

For whole-CRD deletion, remember that Helm does not upgrade or delete CRDs as
ordinary release resources. The release notes must explain the manual cleanup or
migration path, including finalizers and existing custom resources.
