# Developer Local Omni Fixture

This page is for contributors who need a local real-Omni smoke test while developing the operator.

The operator has deterministic no-Omni tests today:

- template rendering and validation with upstream Omni template code
- controller reconciliation through a fake `omniapi.Client`
- kind e2e coverage for CRD admission, validating webhooks, suspended clusters,
  and child-resource status

The repo also includes an opt-in local Omni fixture for transport-level smoke
testing. It installs Omni into the same kind cluster through Sidero's Helm chart,
runs Dex as an in-cluster OIDC provider, asks Omni to create an initial service
account key, then creates an `OmniConnection` and waits for the deployed operator
to report `Ready=True`.

This is not part of the default e2e suite because it pulls the real Omni chart and
image, accepts the Omni EULA for a local test instance, and depends on container
startup timing. The default e2e suite stays fast and deterministic.

## Fixture Shape

Sidero's current self-hosted docs describe Omni as a containerized service with
TLS, identity provider configuration, SideroLink ports, persistent state, and EULA
requirements. The local fixture follows that deployment model instead of using a
dummy Docker Compose service, because the published Omni Go client talks to Omni's
authenticated gRPC API and COSI state.

The fixture files live under `hack/omni/`:

- `dex.yaml` runs a local Dex OIDC provider in the `omni-system` namespace.
- `values.yaml` configures the Omni Helm chart for in-cluster HTTP/gRPC access,
  Dex-backed OIDC, persistence, EULA acceptance, and an initial service account.
- `generate-etcd-key` creates the local GPG private key Omni requires for storage
  encryption.
- `wait-for-initial-service-account` extracts the generated service account key
  into `.local/omni/service-account.key`.

References:

- [Run Omni On-Prem](https://docs.siderolabs.com/omni/self-hosted/run-omni-on-prem)
- [Omni Configuration Examples](https://docs.siderolabs.com/omni/self-hosted/omni-configuration-example)
- [Options for Running Omni](https://docs.siderolabs.com/omni/self-hosted/options-for-running-omni)
- [Sidero Omni Helm chart README](https://github.com/siderolabs/omni/blob/main/deploy/helm/omni/README.md)

## Commands

Render the Omni chart without installing it:

```sh
task omni-template >/tmp/omni-render.yaml
```

Create the local Kubernetes cluster, install Omni, deploy the operator, and run
the live smoke test:

```sh
task kind-up
task omni-up
task cert-manager-up
task docker-build
kind load docker-image "${OMNI_OPERATOR_IMG:-omni-cluster-operator:dev}" --name "${KIND_CLUSTER:-omni-cluster-operator-dev}"
task deploy
task test-live-omni
```

Inspect or remove the fixture:

```sh
task omni-status
task omni-down
task undeploy
task kind-down
```

The default endpoint is the in-cluster Omni service:

```sh
OMNI_LIVE_ENDPOINT=http://omni.omni-system.svc.cluster.local:8080
```

Override the endpoint and key path to point the same test at an existing Omni
instance:

```sh
OMNI_E2E_ENDPOINT=https://omni.example.com \
OMNI_E2E_SERVICE_ACCOUNT_KEY_FILE=/path/to/service-account.key \
go test -tags=live_omni ./test/live
```

Set `OMNI_E2E_INSECURE_SKIP_TLS_VERIFY=true` only for HTTPS test endpoints with
self-signed certificates.

## Current Test Contract

`task test-live-omni` currently verifies:

1. The target namespace already exists, which means the operator deployment has
   been installed.
2. A Secret can be created from the Omni service account key.
3. An `OmniConnection` can be created against the target endpoint.
4. The real controller, running in-cluster, can use
   `github.com/siderolabs/omni/client` service account auth to list Omni cluster
   resources.
5. The `OmniConnection` reaches `Ready=True`.

The live test intentionally does not create or delete a real Omni cluster yet.
That destructive path should be gated behind explicit disposable machine-class or
static-machine configuration.

## Next Live Coverage

The next layer should add a second live suite that is disabled unless explicit
machine input is provided. That suite should create a suspended `OmniCluster`,
then optionally verify `SyncTemplate`, `StatusCluster`, and finalizer delete
paths with `spec.deletePolicy.orphan: true` by default.
