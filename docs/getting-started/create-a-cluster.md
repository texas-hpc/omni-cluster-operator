# Create a Cluster

Create one `OmniConnection`, one `OmniCluster`, and exactly one `OmniControlPlane`. `OmniWorkers` and `OmniMachine` resources are optional.

If you do not create any workers, configure Talos to allow workloads on the control plane nodes.

These resources do not need to live in one multi-document YAML file. In a GitOps repository, it is usually clearer to keep them as separate manifests, for example:

```text
omni-cluster-operator/
  omni-connection.yaml
  clusters/
    cluster-01/
      cluster.yaml
      control-plane.yaml
      workers.yaml
      machines.yaml
      cilium.yaml
    cluster-02/
      cluster.yaml
      control-plane.yaml
      workers.yaml
      machines.yaml
```

The exact layout is up to you. The important part is that the `OmniConnection` can be shared by multiple `OmniCluster` resources when they use the same Omni endpoint and service account Secret.

The examples below use static Omni machine IDs. Replace the endpoint, versions, and machine IDs with values from your environment.

## Create the service account Secret

Create the Secret before creating the `OmniConnection` that references it. The Secret must be in the operator release namespace.

Do not commit the real service account key to Git. Create the Secret with your secret-management workflow, or create it directly with `kubectl`:

```sh
kubectl create secret generic omni-service-account \
  --namespace omni-cluster-operator-system \
  --from-literal=serviceAccountKey='<omni service account key>'
```

## Create the connection

`OmniConnection` tells the operator how to reach Omni and which Secret key contains the service account key.

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniConnection
metadata:
  name: omni
  namespace: omni-cluster-operator-system
spec:
  endpoint: https://omni.example.com
  auth:
    serviceAccountSecretRef:
      name: omni-service-account
      key: serviceAccountKey
```

## Optional: add machine-specific settings

Skip this section if the machine set definitions are enough for your cluster. Use `OmniMachine` resources only when you need per-machine settings such as install disk, patches, extensions, or kernel args.

`OmniMachine.spec.clusterRef.name` can point at the cluster name you are about to create. The resource may report `MissingCluster` until the matching `OmniCluster` exists.

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniMachine
metadata:
  name: cluster-01-control-plane-0
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  machineID: 11111111-1111-4111-8111-111111111111
  install:
    disk: /dev/nvme0n1
```

## Add cluster-01 control plane

Each cluster should have exactly one `OmniControlPlane`. It can select explicit machine IDs or a machine class.

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniControlPlane
metadata:
  name: cluster-01-control-plane
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  machines:
    - 11111111-1111-4111-8111-111111111111
```

## Optional: add cluster-01 workers

`OmniWorkers` defines one worker set. Add more `OmniWorkers` resources when a cluster needs multiple worker sets.

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniWorkers
metadata:
  name: cluster-01-workers
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  machines:
    - 22222222-2222-4222-8222-222222222222
```

## Optional: install Cilium

Use `OmniCilium` when the cluster should install Cilium through Omni manifest sync. See [Install Cilium](install-cilium.md) for the full workflow.

## Optional: use machine classes

Skip this section when you want to assign explicit machine IDs. `OmniControlPlane` and `OmniWorkers` accept either `machines` or `machineClass`, but not both.

This control plane uses a machine class instead of explicit machine IDs:

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniControlPlane
metadata:
  name: cluster-01-control-plane
  namespace: omni-cluster-operator-system
spec:
  clusterRef:
    name: cluster-01
  machineClass:
    name: control-plane
    size: 3
```

This worker set also uses a machine class:

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
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
    size: 3
```

## Create cluster-01

`OmniCluster` ties the template together. It owns the remote Omni cluster lifecycle, selects the shared connection, gathers child template resources by `clusterRef`, and defines cluster-level versions and settings.

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniCluster
metadata:
  name: cluster-01
  namespace: omni-cluster-operator-system
spec:
  connectionRef:
    name: omni
  kubernetes:
    version: v1.35.0
  talos:
    version: v1.13.2
  syncInterval: 5m
```

If the cluster has no `OmniWorkers`, the control plane nodes need to run normal workloads too. Talos does not schedule workloads on control plane nodes by default, so add a cluster-level patch that sets `cluster.allowSchedulingOnControlPlanes: true`:

```yaml
apiVersion: omni.texas-hpc.org/v1alpha1
kind: OmniCluster
metadata:
  name: cluster-01
  namespace: omni-cluster-operator-system
spec:
  connectionRef:
    name: omni
  kubernetes:
    version: v1.35.0
  talos:
    version: v1.13.2
  patches:
    - name: allow-control-plane-workloads
      inline:
        cluster:
          allowSchedulingOnControlPlanes: true
  syncInterval: 5m
```

Apply `OmniCluster` with `spec.suspend: true` if you want to create or update resources without syncing to Omni yet:

```yaml
spec:
  suspend: true
```

Remove the field or set it to `false` when the cluster template is ready to sync.

Apply the manifests with your normal Kubernetes or GitOps workflow. For example:

```sh
kubectl apply -f <manifest-file-or-directory>
```

Check status:

```sh
kubectl get omniconnections,omniclusters,omnicontrolplanes,omniworkers,omnimachines \
  --namespace omni-cluster-operator-system

kubectl describe omnicluster cluster-01 \
  --namespace omni-cluster-operator-system
```

## Pause remote sync

Set `spec.suspend: true` on `OmniCluster` to stop remote Omni sync while preserving Kubernetes resources, status, and finalizers.

```sh
kubectl patch omnicluster cluster-01 \
  --namespace omni-cluster-operator-system \
  --type merge \
  --patch '{"spec":{"suspend":true}}'
```
