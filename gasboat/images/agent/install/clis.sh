#!/bin/bash
# clis.sh — install binary CLI tools for gasboat agent image.
#
# Used by both Dockerfile (RUN /tmp/install/clis.sh) and RWX CI.
# All downloads run in parallel for speed.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/versions.env"

ARCH=${TARGETARCH:-amd64}
case "$ARCH" in arm64) ARCH_ALT=aarch64 ;; *) ARCH_ALT=x86_64 ;; esac

PREFIX=${INSTALL_PREFIX:-/usr/local}
mkdir -p "$PREFIX/bin" "$PREFIX/lib/docker/cli-plugins" "$PREFIX/share/helm/plugins"

(
  # kubectl
  KUBECTL_VERSION=$(curl -fsSL https://dl.k8s.io/release/stable.txt)
  curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl" \
    -o "$PREFIX/bin/kubectl"
  chmod +x "$PREFIX/bin/kubectl"
) &

(
  # GitHub CLI
  GH_VERSION=$(curl -fsSL "https://api.github.com/repos/cli/cli/releases/latest" \
    | jq -r .tag_name | sed "s/^v//")
  curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${ARCH}.tar.gz" \
    | tar -xz --strip-components=2 -C "$PREFIX/bin" "gh_${GH_VERSION}_linux_${ARCH}/bin/gh"
) &

(
  # GitLab CLI
  GLAB_VERSION=$(curl -fsSL "https://gitlab.com/api/v4/projects/gitlab-org%2Fcli/releases" \
    | jq -r '.[0].tag_name | ltrimstr("v")')
  curl -fsSL "https://gitlab.com/gitlab-org/cli/-/releases/v${GLAB_VERSION}/downloads/glab_${GLAB_VERSION}_linux_${ARCH}.tar.gz" \
    | tar -xz --strip-components=1 -C "$PREFIX/bin" bin/glab
) &

(
  # Docker CLI + Compose plugin
  curl -fsSL "https://download.docker.com/linux/static/stable/${ARCH_ALT}/docker-${DOCKER_VERSION}.tgz" \
    | tar -xz --strip-components=1 -C "$PREFIX/bin" docker/docker
  curl -fsSL "https://github.com/docker/compose/releases/download/v${DOCKER_COMPOSE_VERSION}/docker-compose-linux-${ARCH}" \
    -o "$PREFIX/lib/docker/cli-plugins/docker-compose"
  chmod +x "$PREFIX/lib/docker/cli-plugins/docker-compose"
) &

(
  # AWS CLI
  curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH_ALT}.zip" -o /tmp/awscli.zip
  unzip -q /tmp/awscli.zip -d /tmp
  /tmp/aws/install --install-dir "$PREFIX/../opt/aws-cli" --bin-dir "$PREFIX/bin" 2>/dev/null \
    || /tmp/aws/install --install-dir /opt/aws-cli --bin-dir "$PREFIX/bin"
  rm -rf /tmp/awscli.zip /tmp/aws
) &

(
  # Helm + plugins
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \
    | HELM_INSTALL_DIR="$PREFIX/bin" bash
  HELM_DATA_HOME="$PREFIX/share/helm" "$PREFIX/bin/helm" plugin install \
    https://github.com/helm-unittest/helm-unittest
  HELM_DATA_HOME="$PREFIX/share/helm" "$PREFIX/bin/helm" plugin install \
    https://github.com/chartmuseum/helm-push
) &

(
  # Terraform + Terragrunt
  curl -fsSL "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_${ARCH}.zip" \
    -o /tmp/terraform.zip
  unzip -q /tmp/terraform.zip -d "$PREFIX/bin"
  rm /tmp/terraform.zip
  curl -fsSL "https://github.com/gruntwork-io/terragrunt/releases/download/v${TERRAGRUNT_VERSION}/terragrunt_linux_${ARCH}" \
    -o "$PREFIX/bin/terragrunt"
  chmod +x "$PREFIX/bin/terragrunt"
) &

(
  # uv (Python package manager)
  curl -LsSf https://astral.sh/uv/install.sh | env UV_UNMANAGED_INSTALL="$PREFIX/bin" INSTALLER_NO_MODIFY_PATH=1 sh
) &

(
  # Poetry
  curl -sSL https://install.python-poetry.org | POETRY_HOME="$PREFIX" python3 -
  chmod +x "$PREFIX/bin/poetry"
) &

(
  # Bun
  BUN_DIR="$PREFIX/bun"
  curl -fsSL https://bun.sh/install | BUN_INSTALL="$BUN_DIR" bash
  ln -sf "$BUN_DIR/bin/bun" "$PREFIX/bin/bun"
  ln -sf "$BUN_DIR/bin/bunx" "$PREFIX/bin/bunx"
) &

(
  # yq
  YQ_VERSION=$(curl -fsSL "https://api.github.com/repos/mikefarah/yq/releases/latest" | jq -r .tag_name)
  curl -fsSL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${ARCH}" \
    -o "$PREFIX/bin/yq"
  chmod +x "$PREFIX/bin/yq"
) &

(
  # stern
  curl -fsSL "https://github.com/stern/stern/releases/download/v${STERN_VERSION}/stern_${STERN_VERSION}_linux_${ARCH}.tar.gz" \
    | tar -xz -C "$PREFIX/bin" stern
) &

(
  # hadolint
  case "$ARCH" in arm64) HL_ARCH=arm64 ;; *) HL_ARCH=x86_64 ;; esac
  curl -fsSL "https://github.com/hadolint/hadolint/releases/latest/download/hadolint-Linux-${HL_ARCH}" \
    -o "$PREFIX/bin/hadolint"
  chmod +x "$PREFIX/bin/hadolint"
) &

(
  # RTK
  case "$ARCH" in arm64) RTK_TRIPLE=aarch64-unknown-linux-gnu ;; *) RTK_TRIPLE=x86_64-unknown-linux-musl ;; esac
  curl -fsSL "https://github.com/rtk-ai/rtk/releases/download/v${RTK_VERSION}/rtk-${RTK_TRIPLE}.tar.gz" \
    | tar -xz -C "$PREFIX/bin" rtk
) &

(
  # Claudeless
  curl -fsSL "https://github.com/alfredjeanlab/claudeless/releases/download/${CLAUDELESS_VERSION}/claudeless-linux-${ARCH_ALT}.tar.gz" \
    | tar -xz -C "$PREFIX/bin" claudeless
  chmod +x "$PREFIX/bin/claudeless"
) &

(
  # RWX CLI
  curl -fsSL "https://github.com/rwx-cloud/cli/releases/download/v${RWX_CLI_VERSION}/rwx-linux-${ARCH_ALT}" \
    -o "$PREFIX/bin/rwx"
  chmod +x "$PREFIX/bin/rwx"
) &

(
  # k6 (load testing)
  curl -fsSL "https://github.com/grafana/k6/releases/download/v${K6_VERSION}/k6-v${K6_VERSION}-linux-${ARCH}.tar.gz" \
    | tar -xz --strip-components=1 -C "$PREFIX/bin" "k6-v${K6_VERSION}-linux-${ARCH}/k6"
) &

(
  # whisper-cli (needs cmake, gcc — must be available in PATH)
  curl -fsSL "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v${WHISPER_CPP_VERSION}.tar.gz" \
    | tar -xz -C /tmp
  cmake -S "/tmp/whisper.cpp-${WHISPER_CPP_VERSION}" -B /tmp/whisper-build \
    -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF
  cmake --build /tmp/whisper-build --config Release -j"$(nproc)" --target whisper-cli
  cp /tmp/whisper-build/bin/whisper-cli "$PREFIX/bin/whisper-cli"
  rm -rf "/tmp/whisper.cpp-${WHISPER_CPP_VERSION}" /tmp/whisper-build
) &

wait
echo "All CLI tools installed."
