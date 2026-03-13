#!/bin/bash
# node.sh — install Node.js and Claude Code for gasboat agent image.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/versions.env"

ARCH=${TARGETARCH:-amd64}
case "$ARCH" in arm64) NODE_ARCH=arm64 ;; *) NODE_ARCH=x64 ;; esac

PREFIX=${INSTALL_PREFIX:-/usr/local}

NODE_FULL=$(curl -fsSL "https://nodejs.org/dist/index.json" \
  | jq -r "[.[] | select(.version | startswith(\"v${NODE_MAJOR}.\"))][0].version")
[ "${NODE_FULL}" != "null" ] && [ -n "${NODE_FULL}" ] && \
curl -fsSL "https://nodejs.org/dist/${NODE_FULL}/node-${NODE_FULL}-linux-${NODE_ARCH}.tar.xz" \
  | tar -xJ --strip-components=1 -C "$PREFIX"

PATH="$PREFIX/bin:$PATH" npm install -g @anthropic-ai/claude-code

echo "Node.js ${NODE_FULL} + Claude Code installed."
