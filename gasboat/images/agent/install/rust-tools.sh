#!/bin/bash
# rust-tools.sh — install Rust toolchain, rust-analyzer, and quench.
set -e

PREFIX=${INSTALL_PREFIX:-/usr/local}
export RUSTUP_HOME="$PREFIX/rustup"
export CARGO_HOME="$PREFIX/cargo"

curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
  | sh -s -- -y --default-toolchain stable --profile default \
      --component rust-analyzer

# quench (code quality checker)
"$CARGO_HOME/bin/cargo" install quench --git https://github.com/alfredjeanlab/quench
mv "$CARGO_HOME/bin/quench" "$PREFIX/bin/quench"

chmod -R a+rw "$RUSTUP_HOME" "$CARGO_HOME"

echo "Rust + tools installed."
