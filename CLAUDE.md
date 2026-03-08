# Gasboats Monorepo

Unified repository for the gasboat platform — K8s agent controller, terminal mux, beads daemon, and 3D visualization.

## Architecture

| Directory | Language | Description |
|---|---|---|
| `gasboat/` | Go | K8s agent controller, Slack bridge, Helm chart, agent images |
| `coop/` | Rust | Agent terminal multiplexer + credential manager |
| `kbeads/` | Go | Beads daemon (gRPC/HTTP server) + kd CLI |
| `beads3d/` | JS/Vite | 3D force-graph visualization of beads |

## Workspaces

- **Go**: `go.work` at repo root links `gasboat/controller` and `kbeads`
- **Rust**: `Cargo.toml` workspace at repo root links `coop/crates/*`
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
