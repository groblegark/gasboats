# Gasboats Monorepo

Unified repository for the gasboat platform — K8s agent controller, terminal mux, beads daemon, and 3D visualization.

## Architecture

| Directory | Language | Description |
|---|---|---|
| `gasboat/` | Go | K8s agent controller, Slack bridge, Helm chart, agent images |
| `coop/` | Rust | Agent terminal multiplexer + credential manager |
| `kbeads/` | Go | Beads daemon (gRPC/HTTP server) + kd CLI |
| `beads3d/` | JS/Vite | 3D force-graph visualization of beads |

## Directory Structure

```
gasboat/              # K8s agent controller, Slack bridge, Helm chart, agent images
├── controller/       # Go module — controller + bridge + CLI binaries
├── helm/gasboat/     # Helm chart (controller, coopmux, slack-bridge, postgres, nats)
└── images/           # Dockerfiles for agent, slack-bridge, jira-bridge, etc.
coop/                 # Rust workspace — agent terminal multiplexer + credential manager
├── crates/cli/       # coop binary + library
├── crates/mux/       # coopmux multi-session dashboard
├── tests/specs/      # Binary smoke tests
└── proto/            # gRPC protobuf definitions
kbeads/               # Go module — beads daemon (gRPC/HTTP) + kd CLI
├── cmd/kd/           # kd CLI entry point
└── gen/              # Generated protobuf code
beads3d/              # JS/Vite — 3D force-graph visualization
├── src/              # Application source
└── tests/            # Playwright tests
.rwx/                 # RWX CI pipeline definitions
scripts/              # Release and utility scripts
```

## Workspaces

- **Go**: `go.work` at repo root links `gasboat/controller` and `kbeads`
- **Rust**: `coop/Cargo.toml` is a self-contained workspace (`coop/crates/*`)
- **JS**: `beads3d/` is standalone (no npm workspaces needed for single project)

## Build

```sh
make build            # build all components
make build-controller # gasboat controller binary
make build-bridge     # slack bridge binary
make build-kbeads     # kd CLI + beads server
make build-coop       # coop (Rust)
make build-beads3d    # beads3d (JS)
make test             # test all
make lint             # lint all
```

## Per-Component Details

Each component retains its own CLAUDE.md with component-specific instructions.
See `gasboat/CLAUDE.md`, `coop/CLAUDE.md`, `kbeads/CLAUDE.md`, `beads3d/CLAUDE.md`.

## Release

One tag, one pipeline, all components released together.

```sh
./scripts/release.sh          # bump Chart.yaml, commit, tag (calver YYYY.DDD.N)
git push origin main <TAG>    # triggers all CI:
#   - RWX docker.yml: builds + pushes all images to GHCR
#   - RWX helm.yml: packages + pushes Helm chart
#   - GitHub release.yml: creates release + triggers deploy
```

Formula: `kd formula apply kd-GwMFKXnPvR --var version=YYYY.DDD.N --http-url https://beads.gasboat.app.e2e.dev.fics.ai`

## CI

- **GitHub Actions**: path-filtered validation (only test changed components)
- **RWX**: parallel build/test/lint with per-component caching
- All images pushed to `ghcr.io/groblegark/gasboats/<component>:<calver>`

## Commits

Use short, imperative subject lines. Scope in parentheses with component prefix:
- `fix(gasboat/bridge): handle nil bead metadata`
- `feat(kbeads): add search command`
- `fix(coop): credential refresh race`
- `feat(beads3d): add timeline view`

## Landing the Plane

1. `make build` — all components must build
2. `make test` — all tests must pass
3. `make lint` — all linters must pass
4. `helm lint gasboat/helm/gasboat/` — Helm chart must lint
