allow_k8s_contexts("kind-omni-cluster-operator-dev")

local_resource(
    "manifests",
    "mise run manifests",
    deps=["api", "internal/controller"],
)

docker_build("omni-cluster-operator:dev", ".")

k8s_yaml(local("mise exec -- hack/mise/kustomize-build-with-image config/default omni-cluster-operator:dev"))

k8s_resource(
    "omni-cluster-operator-controller-manager",
    port_forwards=["8081:8081"],
)
