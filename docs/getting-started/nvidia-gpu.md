# NVIDIA GPU Workloads

Use this guide when an Omni-managed Talos cluster should run NVIDIA GPU workloads.

This page maps the current Talos guidance into `omni-cluster-operator` resources. Talos still provides the NVIDIA driver, container runtime integration, and workload-level runtime behavior. The operator only renders the Omni cluster template that tells Omni which machines need those settings.

## What Talos requires

Talos GPU enablement has three layers:

1. Talos system extensions for the NVIDIA kernel modules and NVIDIA container runtime support.
2. A Talos machine configuration patch that loads NVIDIA kernel modules.
3. Kubernetes GPU management resources, currently the NVIDIA GPU Operator for Talos 1.13 and newer.

Talos system extensions are installed through boot assets, disk images, or installer images and are activated during Talos install or upgrade. For Omni clusters, use Omni media, Image Factory, or Omni cluster-template `systemExtensions` so the GPU worker machines receive the same extension set during lifecycle operations.

!!! warning "GB10 and arm64 NVIDIA systems"
    Talos 1.13 documentation calls out Grace Blackwell GB10 devices with arm64 CPUs, such as NVIDIA DGX Spark, as requiring the `arm64.nobti` kernel argument. Without it, the system may crash or CUDA libraries may fail to load. Add `arm64.nobti` to the affected arm64 GPU machines, and keep those machines on the proprietary NVIDIA driver family unless current Talos guidance for your hardware says otherwise.

## Choose GPU nodes

Prefer a dedicated `OmniWorkers` set for GPU nodes. Do not put NVIDIA extensions on every machine unless every machine has compatible NVIDIA hardware.

Use `spec.machineClass` when Omni should select GPU nodes dynamically. A MachineClass is an Omni resource outside this operator that selects registered machines by Omni labels. Create it in Omni first, and label the GPU machines so they match that class. The operator only references the MachineClass by name; it does not create or label it.

Use `machineClass.size` to tell Omni how many matching machines to place in the worker set. Use an integer for a fixed count, or an Omni-supported keyword such as `unlimited` when that is what you want. Do not set both `machineClass` and `machines` on the same `OmniWorkers`.

Use explicit `machines` instead when you want direct control over the exact GPU machine IDs, or when a static machine needs per-node settings such as `kernelArgs`.

## Configure a GPU worker set

Configure extensions, the Talos module-loading patch, and rolling strategies on the GPU `OmniWorkers` resource. Worker-set scope is usually easiest when all machines in that set have the same GPU role.

Choose one NVIDIA driver family:

- OSS kernel modules: `siderolabs/nvidia-open-gpu-kernel-modules-production` or `siderolabs/nvidia-open-gpu-kernel-modules-lts`
- Proprietary kernel modules: `siderolabs/nonfree-kmod-nvidia-production` or `siderolabs/nonfree-kmod-nvidia-lts`

Use the matching `siderolabs/nvidia-container-toolkit-production` or `siderolabs/nvidia-container-toolkit-lts` extension with either driver family. The suffix is Sidero's extension track: `-production` is intended to track NVIDIA's production driver branch, and `-lts` is intended to track NVIDIA's LTS driver branch. The actual versions are whatever Sidero has published for that Talos release, so use the [Sidero extensions catalog](https://github.com/siderolabs/extensions#nvidia-gpu) as the source of truth rather than assuming the latest upstream NVIDIA release is already available.

Pick one suffix per GPU node and use it consistently for the driver, container toolkit, and Fabric Manager if needed. Do not install both OSS and proprietary driver extensions on the same machine.

Talos also needs the NVIDIA kernel modules loaded on GPU nodes. Include the module patch in the same worker set so extension changes and machine config changes roll together:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniWorkers
metadata:
  name: cluster-01-gpu-workers
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  workerSetName: gpu-workers
  machineClass:
    name: gpu
    size: 2
  systemExtensions:
    - siderolabs/nvidia-open-gpu-kernel-modules-production
    - siderolabs/nvidia-container-toolkit-production
  patches:
    - name: nvidia-kernel-modules
      inline:
        machine:
          kernel:
            modules:
              - name: nvidia
              - name: nvidia_uvm
              - name: nvidia_drm
              - name: nvidia_modeset
  updateStrategy:
    type: Rolling
    rolling:
      maxParallelism: 1
  upgradeStrategy:
    type: Rolling
    rolling:
      maxParallelism: 1
```

The extension names to select are:

| Purpose | Production branch | LTS branch |
| --- | --- | --- |
| OSS kernel modules | `siderolabs/nvidia-open-gpu-kernel-modules-production` | `siderolabs/nvidia-open-gpu-kernel-modules-lts` |
| Proprietary kernel modules | `siderolabs/nonfree-kmod-nvidia-production` | `siderolabs/nonfree-kmod-nvidia-lts` |
| Container toolkit | `siderolabs/nvidia-container-toolkit-production` | `siderolabs/nvidia-container-toolkit-lts` |
| Fabric Manager | `siderolabs/nvidia-fabricmanager-production` | `siderolabs/nvidia-fabricmanager-lts` |

Add the optional extensions only when the hardware or workload requires them:

- Fabric Manager: add `siderolabs/nvidia-fabricmanager-production` or `siderolabs/nvidia-fabricmanager-lts` for hardware that requires it, such as NVLink/NVSwitch systems. Its suffix must match the NVIDIA driver family.
- GPUDirect RDMA/GDRCopy: add `siderolabs/nvidia-gdrdrv-device` for workloads that need that device path. This is additive; it does not replace the NVIDIA driver or container toolkit extensions.

## Hybrid driver families

A single Omni cluster can have multiple worker sets. Omni cluster templates support any number of `kind: Workers` documents with different names, and this operator maps each `OmniWorkers` resource that references the same `OmniCluster` into one of those documents. Use that shape when different hardware needs different NVIDIA driver families.

For example, an x86 GPU worker set might use OSS kernel modules while an arm64 Grace Blackwell worker set uses the proprietary driver. Talos 1.13 documentation calls out DGX systems as proprietary-driver systems, and Grace Blackwell GB10 devices with arm64 CPUs, such as NVIDIA DGX Spark, need the `arm64.nobti` kernel argument.

Keep the split explicit:

- Put x86 OSS GPU nodes in one `OmniWorkers` resource.
- Put arm64 proprietary GPU nodes in another `OmniWorkers` resource.
- Give every `OmniWorkers` a distinct `spec.workerSetName`, or omit it and use distinct Kubernetes object names.
- Use one driver extension family per worker set.
- Keep the NVIDIA container toolkit family aligned with the selected driver family.
- Use rolling update and upgrade strategies on both sets.
- Use explicit machine IDs for the arm64 GB10 set if you need `kernelArgs`, because this operator validates `kernelArgs` only for static machine sets.

Minimal x86 OSS worker set shape:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniWorkers
metadata:
  name: cluster-01-gpu-x86-oss
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  workerSetName: gpu-x86-oss
  machineClass:
    name: gpu-x86
    size: 2
  systemExtensions:
    - siderolabs/nvidia-open-gpu-kernel-modules-production
    - siderolabs/nvidia-container-toolkit-production
  # Also include the nvidia-kernel-modules patch and rolling strategies
  # from the GPU worker set example above.
```

Minimal arm64 GB10 proprietary worker set shape:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniWorkers
metadata:
  name: cluster-01-gpu-arm64-proprietary
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  workerSetName: gpu-arm64-proprietary
  machines:
    - 33333333-3333-4333-8333-333333333333
  systemExtensions:
    - siderolabs/nonfree-kmod-nvidia-production
    - siderolabs/nvidia-container-toolkit-production
  kernelArgs:
    - arm64.nobti
  # Also include the nvidia-kernel-modules patch and rolling strategies
  # from the GPU worker set example above.
```

If you prefer to select the arm64 nodes with an Omni machine class, put `arm64.nobti` into the boot assets or another machine-specific mechanism instead of `OmniWorkers.spec.kernelArgs`. Do not put `kernelArgs` on a machine-class worker set.

Kubernetes normally does not need separate runtime classes for the two driver families. The runtime handler is still `nvidia`; the separation is in node selection, labels, taints, tolerations, and workload scheduling. Use labels such as `gpu.nvidia.com/family=gb10` or your own platform labels if workloads need to target one hardware family.

## Install the NVIDIA GPU Operator

For Talos 1.13 and newer, use the NVIDIA GPU Operator rather than installing a separate legacy chart. Talos supplies the NVIDIA driver and container toolkit through system extensions, so disable those GPU Operator components.

Use `OmniHelmRelease` to install the GPU Operator into the Omni-created workload cluster. Keep the namespace as a small Omni-managed prerequisite on the parent cluster, because Helm's namespace setting does not create or label the namespace with the privileged Pod Security Admission mode the GPU Operator needs.

Add this manifest entry to the `OmniCluster` that owns the GPU worker set:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniCluster
metadata:
  name: cluster-01
  namespace: omni-cluster-operator-system
spec:
  connectionRef:
    name: omni
  kubernetes:
    version: v1.35.0
    manifests:
      - name: gpu-operator-namespace
        inline:
          apiVersion: v1
          kind: Namespace
          metadata:
            name: gpu-operator
            labels:
              pod-security.kubernetes.io/enforce: privileged
  talos:
    version: v1.13.2
```

Then export a scoped workload-cluster kubeconfig Secret in the same namespace as the parent `OmniCluster`:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniKubeconfigExport
metadata:
  name: cluster-01-gpu-operator-kubeconfig
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  targetSecretRef:
    name: cluster-01-gpu-operator-kubeconfig
  serviceAccount:
    user: cluster-01-gpu-operator
    groups:
      - gpu-operator-installers
  ttl: 24h
  renewBefore: 4h
  deletionPolicy: Delete
```

Bind that exported user or group in the workload cluster with the permissions the GPU Operator chart needs. Then create the direct Helm release:

```yaml
apiVersion: omni.texashpc.com/v1alpha1
kind: OmniHelmRelease
metadata:
  name: cluster-01-gpu-operator
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  kubeconfigSecretRef:
    name: cluster-01-gpu-operator-kubeconfig
  releaseName: gpu-operator
  namespace: gpu-operator
  wait: true
  timeout: 10m
  deletionPolicy: Uninstall
  chart:
    repository: https://helm.ngc.nvidia.com/nvidia
    chart: gpu-operator
    version: <gpu-operator-chart-version>
    values:
      driver:
        enabled: false
      toolkit:
        enabled: false
      hostPaths:
        driverInstallDir: /usr/local
```

Use NVIDIA's GPU Operator documentation and the [NVIDIA GPU Operator repository](https://github.com/NVIDIA/gpu-operator) to choose the chart version and any additional values for your cluster. Pin that chart version in production rather than relying on the latest chart at reconcile time.

The direct Helm controller runs Helm against the Omni-created workload cluster, not the management cluster where `omni-cluster-operator` is installed. Do not also install the GPU Operator through Flux, Argo CD, or manual Helm unless you intentionally want that tool to own the release instead.

## Runtime behavior

Talos 1.13 uses CDI for NVIDIA GPU support. Recent GPU Operator releases enable CDI by default, so prefer the Operator's generated Kubernetes resources and explicit workload requests over hand-writing containerd runtime defaults.

Avoid making `nvidia` the default containerd runtime on GPU nodes unless you have tested that every workload scheduled there should inherit it. The safer default is to let GPU workloads request `nvidia.com/gpu` normally and let the GPU Operator configure the required runtime plumbing.

## Verify

Use the Talos config and kubeconfig for the Omni-created workload cluster. See [Access the workload cluster](create-a-cluster.md#access-the-workload-cluster) if you need to download them first.

Check Talos first:

```sh
talosctl get modules --nodes <gpu-node>
talosctl get extensions --nodes <gpu-node>
```

The loaded modules should include:

- `nvidia`
- `nvidia_uvm`
- `nvidia_drm`
- `nvidia_modeset`

Check Kubernetes next:

```sh
kubectl get pods --namespace gpu-operator
kubectl describe node <gpu-node-name> | grep -A5 -i nvidia
```

Run a GPU smoke pod with a current CUDA base image from the [NVIDIA CUDA container catalog](https://catalog.ngc.nvidia.com/orgs/nvidia/containers/cuda/tags). Choose a tag that is compatible with the driver version published by the Sidero extension track you installed.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nvidia-smi
spec:
  restartPolicy: Never
  containers:
    - name: nvidia-smi
      # Replace with a current nvcr.io/nvidia/cuda:<tag> from the NGC catalog.
      image: nvcr.io/nvidia/cuda:<current-base-ubuntu24.04-tag>
      command: ["nvidia-smi"]
      resources:
        limits:
          nvidia.com/gpu: 1
```

```sh
kubectl apply -f nvidia-smi.yaml
kubectl logs pod/nvidia-smi
kubectl delete -f nvidia-smi.yaml
```

The logs should show `nvidia-smi` output from inside the container. If the pod cannot start or cannot see the GPU, debug in this order: Talos extensions and loaded modules, GPU Operator pods, node labels from Node Feature Discovery, and finally the workload pod events.

## Upgrade notes

NVIDIA driver extensions are tied to Talos and NVIDIA driver versions. When upgrading Talos:

1. Check the current Talos NVIDIA GPU documentation and extension catalog for the target Talos version.
2. Update GPU worker `systemExtensions` if the extension names or families change.
3. Keep NVIDIA driver, toolkit, and Fabric Manager extension families aligned.
4. Use `OmniCluster.spec.suspend: true` while preparing a multi-object upgrade.
5. Use rolling `upgradeStrategy` on GPU worker sets.
6. Verify modules, extensions, the GPU Operator, and a GPU test workload after the first node rolls.

Do not treat a Kubernetes version bump, Talos version bump, and NVIDIA driver family change as a trivial manifest edit. Stage and verify those changes deliberately.

## References

- [Talos 1.13 NVIDIA GPU OSS driver guide](https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/hardware-and-drivers/nvidia-gpu)
- [Talos 1.13 NVIDIA GPU proprietary driver guide](https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/hardware-and-drivers/nvidia-gpu-proprietary)
- [Talos 1.8 NVIDIA extension naming change](https://docs.siderolabs.com/talos/v1.8/getting-started/what%27s-new-in-talos)
- [Talos system extensions guide](https://www.talos.dev/latest/talos-guides/configuration/system-extensions/)
- [Sidero system extensions catalog](https://github.com/siderolabs/extensions#nvidia-gpu)
- [Talos boot assets guide](https://docs.siderolabs.com/talos/v1.12/platform-specific-installations/boot-assets)
- [Talos 1.13 NVIDIA Fabric Manager guide](https://docs.siderolabs.com/talos/v1.13/configure-your-talos-cluster/hardware-and-drivers/nvidia-fabricmanager)
- [Omni Talos Linux extensions guide](https://docs.siderolabs.com/omni/infrastructure-and-extensions/install-talos-linux-extensions)
- [NVIDIA GPU Operator installation guide](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html)
- [NVIDIA GPU Operator repository](https://github.com/NVIDIA/gpu-operator)
- [NVIDIA CUDA container catalog](https://catalog.ngc.nvidia.com/orgs/nvidia/containers/cuda/tags)
