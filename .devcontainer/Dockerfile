FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV MISE_INSTALL_PATH="/usr/local/bin/mise"
ENV MISE_DATA_DIR="/usr/local/share/mise"
ENV MISE_CONFIG_DIR="/usr/local/etc/mise"
ENV MISE_CACHE_DIR="/usr/local/share/mise/cache"
ENV PATH="/usr/local/share/mise/shims:/usr/local/bin:${PATH}"

RUN apt-get update \
    && apt-get -y --no-install-recommends install \
      bash-completion \
      ca-certificates \
      curl \
      git \
      build-essential \
      pkg-config \
    && rm -rf /var/lib/apt/lists/*

RUN curl https://mise.run | sh

WORKDIR /tmp/omni-cluster-operator
COPY mise.toml mise.toml
RUN mise trust /tmp/omni-cluster-operator/mise.toml \
    && mise install --system go@1.26.3 \
    && mise install --system \
    && rm -rf /tmp/omni-cluster-operator \
    && mkdir -p /usr/local/etc/mise \
    && chown -R vscode:vscode /usr/local/share/mise /usr/local/etc/mise

WORKDIR /workspaces/omni-cluster-operator
