#!/bin/bash
# go-tools.sh — install Go toolchain, gopls, golangci-lint, and Task CLI.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/versions.env"

ARCH=${TARGETARCH:-amd64}
PREFIX=${INSTALL_PREFIX:-/usr/local}

# Go
GO_LATEST=$(curl -fsSL "https://go.dev/dl/?mode=json" \
  | jq -r "[.[] | select(.version | startswith(\"go${GO_VERSION}\"))][0].version")
[ "${GO_LATEST}" != "null" ] && [ -n "${GO_LATEST}" ] && \
curl -fsSL "https://go.dev/dl/${GO_LATEST}.linux-${ARCH}.tar.gz" | tar -C "$PREFIX" -xz

GOBIN="$PREFIX/go/bin"

# gopls
GOROOT="$PREFIX/go" GOPATH=/tmp/gopath "$GOBIN/go" install golang.org/x/tools/gopls@latest 2>/dev/null || true
cp /tmp/gopath/bin/gopls "$GOBIN/gopls" 2>/dev/null || true

# golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sh -s -- -b "$PREFIX/bin" "v${GOLANGCI_LINT_VERSION}"

# Task CLI
GOROOT="$PREFIX/go" GOPATH=/tmp/gopath "$GOBIN/go" install github.com/go-task/task/v3/cmd/task@latest
mv /tmp/gopath/bin/task "$PREFIX/bin/task"

# Symlinks
ln -sf "$PREFIX/go/bin/go" "$PREFIX/bin/go"
ln -sf "$PREFIX/go/bin/gofmt" "$PREFIX/bin/gofmt"
ln -sf "$PREFIX/go/bin/gopls" "$PREFIX/bin/gopls" 2>/dev/null || true

# Cleanup
chmod -R u+w /tmp/gopath 2>/dev/null || true
rm -rf /tmp/gopath

echo "Go ${GO_LATEST} + tools installed."
