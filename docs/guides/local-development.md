# Developer Local Development

This page is for contributors working from this repository. Operator users installing released artifacts should start with [Installation](../getting-started/installation.md).

Install pinned tools with mise:

```sh
mise trust
mise install
```

There is intentionally no Makefile. Use `mise run <task>`.

## Run the operator locally in kind

```sh
mise run kind-up
mise run cert-manager-up
mise run tilt
```

Tilt builds `controller:latest`, applies the default Kustomize deployment, and exposes the health endpoint on port `8081`.

`mise run cert-manager-up` is only for the local development cluster created by this repository. Do not use it as end-user installation guidance for production clusters.

## Render samples

```sh
mise run samples
```

Apply samples only after replacing placeholder values:

```sh
kubectl apply -k config/samples
```

## Run tests

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

## Work on documentation

Build the documentation site:

```sh
mise run docs-build
```

Serve it locally:

```sh
mise run docs-serve
```

The local server listens on `127.0.0.1:8000` unless `MKDOCS_DEV_ADDR` is set.
