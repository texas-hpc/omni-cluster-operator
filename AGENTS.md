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
- Keep secrets out of samples except obvious placeholders.

## Generated Files

Do not hand-edit these unless changing generation itself:

- `api/*/zz_generated.deepcopy.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`

After changing API types or markers, run:

```sh
make manifests generate
```

## Verification

Use the pinned tooling from `mise.toml` when available.

Fast loop:

```sh
mise run test-unit
```

Full local verification:

```sh
make test
make lint
make build
bin/kustomize build config/samples
bin/kustomize build config/default
```

`bin/` and `cover.out` are generated local artifacts and are intentionally ignored.
