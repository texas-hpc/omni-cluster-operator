allow_k8s_contexts("kind-omni-cluster-operator-dev")

local_resource(
    "manifests",
    "make manifests",
    deps=["api", "internal/controller"],
)

docker_build("controller:latest", ".")

k8s_yaml(local("kustomize build config/default"))

k8s_resource(
    "omni-cluster-operator-controller-manager",
    port_forwards=["8081:8081"],
)
