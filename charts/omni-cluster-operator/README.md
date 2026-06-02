# omni-cluster-operator

Kubernetes operator for managing Sidero Omni cluster lifecycles.

This chart installs the `omni-cluster-operator` controller manager, webhook
configuration, RBAC, services, and optional cert-manager certificate resources.

`omni-cluster-operator` is an independent community project for managing Sidero
Omni cluster templates from Kubernetes. It is not affiliated with, sponsored by,
endorsed by, or maintained by Sidero Labs.

## Installation

Prerequisites:

- Kubernetes cluster where the operator will run
- Helm 3
- cert-manager installed in the cluster when `webhook.enabled` and
  `certManager.enabled` are left at their defaults
- The `omni-cluster-operator-crds` chart installed first
- Access to an Omni instance and an Omni service account key

Install the CRDs first:

```sh
helm install omni-cluster-operator-crds \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator-crds
```

Then install the operator:

```sh
helm install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace
```

To pin a specific release, use matching versions for both charts:

```sh
helm install omni-cluster-operator \
  oci://ghcr.io/texas-hpc/charts/omni-cluster-operator \
  --namespace omni-cluster-operator-system \
  --create-namespace \
  --version <chart-version>
```

## Omni Credentials

Create the Omni service account Secret in the operator release namespace. The
service account key can come from the Omni UI or `omnictl serviceaccount
create`. Do not commit real service account keys to Git.

```sh
kubectl create secret generic omni-service-account \
  --namespace omni-cluster-operator-system \
  --from-literal=serviceAccountKey='<omni service account key>'
```

Reference that Secret from an `OmniConnection` resource in the same namespace as
the operator release.

## Scope

The operator runs in namespaced mode and watches the namespace where the chart is
installed. Create `OmniConnection`, `OmniCluster`, and child resources in the
operator release namespace.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| nameOverride | string | `""` | Overrides the chart name used in generated resource names. |
| fullnameOverride | string | `""` | Overrides the full generated release name. |
| namespace.create | bool | `false` | Creates the release namespace as a chart resource. |
| image.repository | string | `"ghcr.io/texas-hpc/omni-cluster-operator"` | Operator image repository. |
| image.tag | string | `""` | Operator image tag. Defaults to the chart app version when empty. |
| image.pullPolicy | string | `"IfNotPresent"` | Kubernetes image pull policy for the manager container. |
| imagePullSecrets | list | `[]` | Image pull secrets attached to the manager Pod. |
| replicaCount | int | `1` | Number of manager replicas. |
| leaderElection.enabled | bool | `true` | Enables controller-runtime leader election. |
| metrics.enabled | bool | `true` | Exposes the controller metrics service. |
| metrics.secure | bool | `true` | Serves metrics over HTTPS. |
| metrics.port | int | `8443` | Metrics container and service port. |
| metrics.service.annotations | object | `{}` | Extra annotations for the metrics Service. |
| metrics.service.labels | object | `{}` | Extra labels for the metrics Service. |
| metrics.rbac.enabled | bool | `true` | Creates metrics authentication and reader RBAC. |
| webhook.enabled | bool | `true` | Creates and serves validating webhooks. |
| webhook.port | int | `9443` | Webhook container port. |
| webhook.failurePolicy | string | `"Fail"` | Failure policy for validating webhook calls. |
| webhook.certDir | string | `"/tmp/k8s-webhook-server/serving-certs"` | Directory where webhook serving certificates are mounted. |
| certManager.enabled | bool | `true` | Creates cert-manager Issuer and Certificate resources and injects the webhook CA bundle. |
| rbac.create | bool | `true` | Creates the manager, leader election, and metrics RBAC resources. |
| rbac.helperRoles.create | bool | `true` | Creates admin, editor, and viewer helper ClusterRoles for the custom resources. |
| serviceAccount.create | bool | `true` | Creates the manager ServiceAccount. |
| serviceAccount.annotations | object | `{}` | Extra annotations for the manager ServiceAccount. |
| serviceAccount.name | string | `""` | Existing ServiceAccount name to use when serviceAccount.create is false. |
| podAnnotations | object | `{"kubectl.kubernetes.io/default-container":"manager"}` | Annotations applied to the manager Pod. |
| podLabels | object | `{}` | Extra labels applied to the manager Pod. |
| podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level security context. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container-level security context. |
| resources | object | `{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | Manager container resource requests and limits. |
| nodeSelector | object | `{}` | Node selector for the manager Pod. |
| tolerations | list | `[]` | Tolerations for the manager Pod. |
| affinity | object | `{}` | Affinity rules for the manager Pod. |
| healthProbe.bindAddress | string | `":8081"` | Manager health probe bind address. |
| healthProbe.port | int | `8081` | Health and readiness probe port. |
| healthProbe.liveness.initialDelaySeconds | int | `15` | Liveness probe initial delay. |
| healthProbe.liveness.periodSeconds | int | `20` | Liveness probe interval. |
| healthProbe.readiness.initialDelaySeconds | int | `5` | Readiness probe initial delay. |
| healthProbe.readiness.periodSeconds | int | `10` | Readiness probe interval. |
| extraArgs | list | `[]` | Extra command-line arguments passed to the manager. |
| extraEnv | list | `[]` | Extra environment variables added to the manager container. |
| extraVolumes | list | `[]` | Extra volumes added to the manager Pod. |
| extraVolumeMounts | list | `[]` | Extra volume mounts added to the manager container. |
| terminationGracePeriodSeconds | int | `10` | Manager Pod termination grace period. |

## Documentation

- [Project documentation](https://texas-hpc.github.io/omni-cluster-operator/)
- [Installation](https://texas-hpc.github.io/omni-cluster-operator/getting-started/installation/)
- [Workload access](https://texas-hpc.github.io/omni-cluster-operator/getting-started/workload-access/)
- [API reference](https://texas-hpc.github.io/omni-cluster-operator/reference/api/)
- [Source repository](https://github.com/texas-hpc/omni-cluster-operator)

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
