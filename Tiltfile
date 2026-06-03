allow_k8s_contexts("kind-omni-cluster-operator-dev")

local_resource(
    "manifests",
    "task manifests",
    deps=["api", "internal/controller"],
)

docker_build("omni-cluster-operator:dev", ".")

k8s_yaml(local("IMAGE=omni-cluster-operator:dev task render-default-with-image"))

k8s_resource(
    "omni-cluster-operator-controller-manager",
    port_forwards=["8081:8081"],
)
