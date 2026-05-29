# omni-cluster-operator 🚀

Manage Sidero Omni cluster templates from Kubernetes.

`omni-cluster-operator` gives platform teams a GitOps-friendly way to define Omni
connections, cluster templates, machine groups, and deletion policy with normal
Kubernetes custom resources. The controller renders those resources into an Omni
cluster template, asks Omni to validate it, syncs it, and reports status back on
the Kubernetes objects you already know how to inspect.

Use this when you want:

- 🧭 One Kubernetes-native API for Omni cluster lifecycle configuration.
- 🔐 Omni credentials stored in Kubernetes Secrets, not committed to Git.
- 🧩 Separate resources for the cluster, control plane, workers, and static machines.
- 🧹 Finalizer-based cleanup, with an orphan option when you want to keep the Omni
  cluster after deleting Kubernetes resources.

## ⚡ Quick Start

You need:

- a Kubernetes cluster where the operator will run
- Helm 3
- access to an Omni instance
- an Omni service account key from `omnictl serviceaccount create`

### 1. Install the Operator

Install the operator with Helm from the GHCR OCI chart registry:

```sh
helm install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --version <chart-version> \
  --namespace omni-cluster-operator-system \
  --create-namespace
```

Choose a chart version from the
[GitHub Packages page](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator).
The chart installs the operator deployment, RBAC, services, and CRDs. By default,
webhooks are disabled and the manager image tag follows the chart app version.

Inspect chart defaults before installing:

```sh
helm show values \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --version <chart-version>
```

If GHCR prompts for credentials, log in with a GitHub token that can read the
package:

```sh
echo "$GITHUB_TOKEN" | helm registry login ghcr.io \
  --username <github-user> \
  --password-stdin
```

For testing an unreleased branch build, override the image tag explicitly:

```sh
helm upgrade --install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
omnictl serviceaccount create > omni-service-account.key
  --version <chart-version> \
  --namespace omni-cluster-operator-system \
  --from-file=serviceAccountKey=./omni-service-account.key
rm -f omni-service-account.key
  --set image.tag=dev
```

### 2. Add Omni Credentials

Create the Omni service account key in the namespace where your `OmniConnection`
and cluster template resources will live. The Secret must not be committed to Git.

```sh
kubectl create namespace clusters

kubectl create secret generic omni-service-account \
  --namespace clusters \
  --from-literal=serviceAccountKey='<output from omnictl serviceaccount create>'
```

### 3. Create Your First Cluster Template

Then apply an `OmniConnection`, one `OmniCluster`, exactly one
`OmniControlPlane`, and any `OmniWorkers` or `OmniMachine` documents for that
cluster:

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniConnection
metadata:
  name: omni
  namespace: clusters
spec:
  endpoint: https://omni.example.com
  auth:
    serviceAccountSecretRef:
      name: omni-service-account
      key: serviceAccountKey
---
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniCluster
metadata:
  name: edge
  namespace: clusters
spec:
  connectionRef:
    name: omni
  kubernetes:
    version: v1.35.0
  talos:
    version: v1.13.2
  syncInterval: 5m
---
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniControlPlane
metadata:
  name: edge-control-plane
  namespace: clusters
spec:
  clusterRef:
    name: edge
  machines:
    - 11111111-1111-4111-8111-111111111111
---
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniWorkers
metadata:
  name: edge-workers
  namespace: clusters
spec:
  clusterRef:
    name: edge
  machines:
    - 22222222-2222-4222-8222-222222222222
```

Check reconciliation status with:

```sh
kubectl get omniconnections,omniclusters,omnicontrolplanes,omniworkers,omnimachines \
  --namespace clusters

kubectl describe omnicluster edge --namespace clusters
```

## 🧱 API Shape

The API group is `omni.texas-hpc.org/v1alpha1`.

- `OmniConnection` defines one Omni endpoint and a Secret reference containing an
  Omni service account key.
- `OmniCluster` owns the remote Omni cluster lifecycle, references an
  `OmniConnection`, and contains cluster-level template fields such as Kubernetes
  version, Talos version, features, patches, system extensions, kernel args,
  sync interval, and delete policy.
- `OmniControlPlane` defines the Omni `ControlPlane` template document for one
  `OmniCluster`. Exactly one control plane must reference a cluster.
- `OmniWorkers` defines one Omni `Workers` template document. Multiple worker
  sets may reference the same `OmniCluster`.
- `OmniMachine` defines an optional Omni `Machine` template document for static
  machines, including install disk and per-machine patches.

Child resources use `spec.clusterRef.name` instead of duplicating
`OmniConnectionRef`; the owning `OmniCluster` selects the Omni instance. This
keeps all template documents for one cluster bound to one connection and avoids
ambiguous cross-Omni machine ownership.

## 🤝 Upstream Omni Boundary

The operator intentionally keeps Omni as the source of truth for template behavior.
It does not try to reimplement Omni's reconciliation rules locally.

Sidero publishes `github.com/siderolabs/omni/client`, including the gRPC/API
packages, COSI resource types, and public template operation functions. The actual
cluster-template document model structs are present under
`client/pkg/template/internal/models`, which Go intentionally prevents downstream
modules from importing.

This repo therefore keeps only the small CRD-to-YAML rendering layer locally and
delegates the drift-prone behavior to upstream Omni:

- template validation: `operations.ValidateTemplate`
- create/update reconciliation: `operations.SyncTemplate`
- delete reconciliation: `operations.DeleteCluster`
- status readback: `operations.StatusCluster`

Omni's cluster template docs also state that templates are backward compatible, so
template YAML is the most stable public contract for this operator to target.

References:

- [Omni cluster template reference](https://docs.siderolabs.com/omni/reference/cluster-templates)
- [Omni Go client module](https://pkg.go.dev/github.com/siderolabs/omni/client)
- [Omni template operations package](https://pkg.go.dev/github.com/siderolabs/omni/client/pkg/template/operations)

## 🛠️ Tooling

Install pinned tools through `mise`:

```sh
mise trust
mise install
```

Useful tasks:

```sh
mise run generate
mise run manifests
mise run test-unit
mise run test
mise run build
mise run kind-up
mise run tilt
mise run kind-down
```

`mise run test-unit` is the fast loop for API/template/controller unit tests.
`mise run test` runs the generated manifests/code, formatting, vet, and all
non-e2e tests.

## 💻 Local Development

Create a local kind cluster through `ctlptl`, then run Tilt:

```sh
mise run kind-up
mise run tilt
```

The Tilt setup builds `controller:latest`, applies the default Kustomize
deployment, and exposes the health endpoint on port `8081`.

Apply the sample CRs only after replacing the placeholder service account key:

```sh
kubectl apply -k config/samples
```

The Secret key defaults to `serviceAccountKey`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: omni-service-account
type: Opaque
stringData:
  serviceAccountKey: "<output from omnictl serviceaccount create>"
```

## 🔄 Operator Behavior

`OmniCluster` is the resource with remote side effects. On reconcile it:

1. Adds the Omni finalizer.
2. Reads the referenced `OmniConnection`.
3. Selects child `OmniControlPlane`, `OmniWorkers`, and `OmniMachine` resources in
   the same namespace by `clusterRef`.
4. Renders deterministic Omni template YAML and stores its SHA-256 hash in status.
5. Validates the template with upstream Omni code.
6. Syncs the template to the selected Omni instance.
7. Reads Omni cluster status and updates Kubernetes conditions.
8. On deletion, calls Omni template deletion unless `spec.deletePolicy.orphan` is
   true, then removes the finalizer.

`OmniConnection` reconciles independently and reports whether the endpoint and
service account key can list Omni cluster resources. Child document controllers
report whether their referenced `OmniCluster` exists.

## ✅ Testing

Current coverage includes:

- deterministic template rendering
- upstream Omni template validation
- machine class and static machine template shapes
- cluster sync, missing-template, and delete/finalizer behavior through a fake
  Omni client
- child resource accepted/missing-cluster status handling
- manifest generation and controller build through standard Kubebuilder targets

The e2e scaffold is present under `test/e2e` and is tagged `e2e`. The next useful
expansion is a local Omni-compatible test double or self-hosted Omni fixture that
can exercise the real `github.com/siderolabs/omni/client` transport.

## 📝 Notes

This project uses Kubebuilder/controller-runtime directly. Operator SDK's own FAQ
describes Go Operator SDK projects as Kubebuilder-based and sharing the same
controller-runtime layout, with extra OLM/OperatorHub/scorecard features on top.
For this repo, those extras are not currently part of the delivery target, so the
lower-level Kubebuilder stack keeps the project simpler while preserving the path
to add OLM packaging later.
