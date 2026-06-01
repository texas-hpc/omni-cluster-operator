# Installation

## Prerequisites

You need:

- A Kubernetes cluster where the operator will run.
- Helm 3 or newer.
- cert-manager installed in the target cluster.
- Access to a Sidero Omni instance.
- An Omni service account key created in the Omni UI or with `omnictl serviceaccount create`.

The default release namespace is `omni-cluster-operator-system`. The operator watches only its own namespace, so create `OmniConnection`, `OmniCluster`, and all child template resources in that namespace.

## Install cert-manager

The operator chart installs validating webhooks and cert-manager `Certificate` resources. Install cert-manager before installing the operator chart.

Use the cert-manager installation method approved for your cluster. For example, install cert-manager with your platform's add-on manager, GitOps stack, or the upstream cert-manager Helm chart. The operator only requires that cert-manager is already reconciling `Certificate` resources before the operator chart is installed.

## Install CRDs

Install the CRD chart before the operator chart:

```sh
helm install omni-cluster-operator-crds \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator-crds
```

By default, Helm installs the latest chart version it can resolve from the OCI registry. To pin a specific release, choose a version from the [CRD chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator-crds) and pass `--version <chart-version>`.

## Install the operator

```sh
helm install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace
```

To pin the operator chart, choose a version from the [operator chart package](https://github.com/texas-hpc/omni-cluster-operator/pkgs/container/charts%2Fomni-cluster-operator) and pass `--version <chart-version>`. When you pin versions, use matching versions for the CRD chart and operator chart.

Inspect chart defaults before installing:

```sh
helm show values \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator
```

## Add Omni credentials

Create the Secret in the operator release namespace. Do not commit the real service account key to Git.

The key value can come from the Omni UI or from `omnictl serviceaccount create`.

```sh
kubectl create secret generic omni-service-account \
  --namespace omni-cluster-operator-system \
  --from-literal=serviceAccountKey='<omni service account key>'
```

## Verify the install

```sh
kubectl get pods,svc,certificate \
  --namespace omni-cluster-operator-system

kubectl get crd | grep omni.texashpc.com
```

The manager pod should be running, the webhook certificate should become ready, and the Omni CRDs should be present.
