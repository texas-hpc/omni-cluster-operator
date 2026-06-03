# Developer Local Development

This page is for contributors working from this repository. Operator users installing released artifacts should start with [Installation](../getting-started/installation.md).

Install pinned tools with mise:

```sh
mise trust
mise install
```

There is intentionally no Makefile. Use `task <task>`; task definitions live in
`Taskfile.yml`.

## Run the operator locally in kind

```sh
task kind-up
task cert-manager-up
task tilt
```

Tilt builds `controller:latest`, applies the default Kustomize deployment, and exposes the health endpoint on port `8081`.

`task cert-manager-up` is only for the local development cluster created by this repository. Do not use it as end-user installation guidance for production clusters.

## Render samples

```sh
task samples
```

Apply samples only after replacing placeholder values:

```sh
kubectl apply -k config/samples
```

## Run tests

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

## Work on documentation

Build the documentation site:

```sh
task docs-build
```

Serve it locally:

```sh
task docs-serve
```

The local server listens on `127.0.0.1:8000` unless `MKDOCS_DEV_ADDR` is set.
