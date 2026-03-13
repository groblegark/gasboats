# Gasboat agent image (multi-stage).
#
# Build (from repo root):
#   docker build -t gasboat-agent -f images/agent/Dockerfile .
#   docker build --target base -t gasboat-agent:base -f images/agent/Dockerfile .  # minimal variant
#
# Tool versions and package lists are defined in install/versions.env.
# Install scripts under install/ are shared with .rwx/docker.yml (CI).

# ═════════════════════════════════════════════════════════════════
# Go builder — compiles gb (agent orchestration CLI) from source
# ═════════════════════════════════════════════════════════════════

FROM golang:1.25-bookworm AS gb-builder
WORKDIR /build
COPY controller/go.mod controller/go.sum ./controller/
RUN cd controller && go mod download
COPY controller/ ./controller/
RUN cd controller && CGO_ENABLED=0 go build -ldflags="-s -w" -o /gb ./cmd/gb/

# ═════════════════════════════════════════════════════════════════
# base — minimal agent with git, Node.js, Claude Code, coop, kd, gb
# ═════════════════════════════════════════════════════════════════

FROM ubuntu:24.04 AS base

ARG DEBIAN_FRONTEND=noninteractive
ARG COOP_VERSION=2026.67.2
ARG BEADS_VERSION=2026.67.2
ARG TARGETARCH

# ── Shared install scripts ─────────────────────────────────────────
COPY images/agent/install/ /tmp/install/

# ── System packages (minimal) ─────────────────────────────────────
RUN . /tmp/install/versions.env && \
    apt-get update && apt-get install -y --no-install-recommends $BASE_PACKAGES && \
    ln -sf /usr/bin/python3 /usr/bin/python && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# ── Node.js + Claude Code ─────────────────────────────────────────
RUN TARGETARCH=${TARGETARCH} /tmp/install/node.sh

# ── Coop (terminal session manager) ─────────────────────────────
RUN set -e && \
    if [ -z "${COOP_VERSION}" ]; then \
      COOP_VERSION=$(curl -fsSL "https://api.github.com/repos/groblegark/coop/releases/latest" | jq -r .tag_name); \
    fi && \
    case "${TARGETARCH}" in arm64) COOP_ARCH=arm64 ;; *) COOP_ARCH=amd64 ;; esac && \
    curl -fsSL "https://github.com/groblegark/coop/releases/download/${COOP_VERSION}/coop-linux-${COOP_ARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin coop

# ── Beads CLI (kd) ──────────────────────────────────────────────
RUN set -e && \
    if [ -z "${BEADS_VERSION}" ]; then \
      BEADS_VERSION=$(curl -fsSL "https://api.github.com/repos/groblegark/kbeads/releases/latest" | jq -r .tag_name); \
    fi && \
    case "${TARGETARCH}" in arm64) BD_ARCH=arm64 ;; *) BD_ARCH=amd64 ;; esac && \
    curl -fsSL "https://github.com/groblegark/kbeads/releases/download/${BEADS_VERSION}/kd_${BEADS_VERSION}_linux_${BD_ARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin ./kd

# ── Gasboat CLI (gb) — agent orchestration ──────────────────────
COPY --from=gb-builder /gb /usr/local/bin/gb

# ── Agent user ───────────────────────────────────────────────────
# Rename the default ubuntu user (UID 1000) to agent across passwd,
# shadow, and group so that sudo/PAM account checks work correctly
# in containers that lack CAP_AUDIT_WRITE.
RUN sed -i 's/^ubuntu:/agent:/' /etc/passwd && \
    sed -i 's|/home/ubuntu|/home/agent|' /etc/passwd && \
    sed -i 's|/bin/sh|/bin/bash|' /etc/passwd && \
    sed -i 's/^ubuntu:/agent:/' /etc/shadow && \
    sed -i 's/^ubuntu:/agent:/' /etc/group && \
    mkdir -p /home/agent/workspace && \
    chown -R 1000:1000 /home/agent && \
    echo 'agent ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/agent && \
    chmod 0440 /etc/sudoers.d/agent

# ── Hook scripts ─────────────────────────────────────────────────
COPY images/agent/hooks/ /hooks/
RUN chmod +x /hooks/*.sh

# ── Claudeless mock scenarios ────────────────────────────────────
COPY images/agent/scenarios/ /scenarios/

# ── kubectl ──────────────────────────────────────────────────────
RUN KUBECTL_VERSION=$(curl -fsSL https://dl.k8s.io/release/stable.txt) && \
    curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
      -o /usr/local/bin/kubectl && \
    chmod +x /usr/local/bin/kubectl

# ── Entrypoint ───────────────────────────────────────────────────
COPY images/agent/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER 1000
WORKDIR /home/agent/workspace

ENTRYPOINT ["/entrypoint.sh"]

# ═════════════════════════════════════════════════════════════════
# Full agent — base + language toolchains, CLIs, and dev tools
# ═════════════════════════════════════════════════════════════════

FROM base AS agent

USER root
ARG DEBIAN_FRONTEND=noninteractive
ARG TARGETARCH

# ── Additional system packages ───────────────────────────────────
RUN . /tmp/install/versions.env && \
    apt-get update && apt-get install -y --no-install-recommends $AGENT_PACKAGES && \
    ln -sf /usr/bin/python3 /usr/bin/python && \
    ln -sf gcc /usr/bin/cc && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# ── Go + gopls + golangci-lint + Task CLI ────────────────────────
RUN TARGETARCH=${TARGETARCH} /tmp/install/go-tools.sh
ENV PATH="/usr/local/go/bin:${PATH}"

# ── Rust toolchain (compiler + cargo + rust-analyzer + quench) ───
ENV RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo
ENV PATH="${CARGO_HOME}/bin:${PATH}"
RUN /tmp/install/rust-tools.sh

# ── CLI tools (gh, glab, docker, aws, helm, terraform, etc.) ────
RUN TARGETARCH=${TARGETARCH} /tmp/install/clis.sh

# ── Playwright + Chromium + Chrome + MCP server ─────────────────
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN /tmp/install/playwright.sh

# ── Environment ──────────────────────────────────────────────────
ENV BUN_INSTALL=/usr/local/bun
ENV PATH="${BUN_INSTALL}/bin:${PATH}"
ENV HELM_PLUGINS=/usr/local/share/helm/plugins

USER 1000
