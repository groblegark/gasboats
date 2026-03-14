# Gasboat

K8s agent controller and Slack bridge for beads automation — extracted from Gastown.

## Architecture

- **controller/** — Go module. K8s agent controller that translates bead lifecycle events into pod operations.
- **controller/internal/bridge/** — Slack notification bridge (decisions watcher, mail watcher, Slack interactions). Zero K8s dependencies.
- **controller/cmd/controller/** — Controller entry point (single binary).
- **controller/cmd/slack-bridge/** — Standalone Slack bridge binary.
- **helm/gasboat/** — Helm chart for all components (controller, coopmux, slack-bridge, PostgreSQL, NATS).
- **images/** — Dockerfiles for agent pods and slack-bridge.

## Directory Structure

```
controller/              # Go module — agent controller + slack bridge
├── cmd/controller/      # Controller entry point
├── cmd/slack-bridge/    # Standalone Slack bridge binary
└── internal/
    ├── beadsapi/        # HTTP client to beads daemon
    ├── bridge/          # Slack notifications (decisions, mail, interactions)
    ├── config/          # Env var parsing
    ├── podmanager/      # Pod spec construction & CRUD
    ├── reconciler/      # Periodic desired-vs-actual sync
    ├── statusreporter/  # Pod phase → bead state updates
    └── subscriber/      # SSE/NATS event listener
helm/gasboat/            # Helm chart (controller, coopmux, slack-bridge, postgres, nats)
images/
├── agent/               # Agent pod image + entrypoint
└── slack-bridge/        # Slack bridge Dockerfile
Makefile                 # Top-level build
quench.toml              # Quality checks
```

## Build

```sh
cd controller && go build ./cmd/controller/    # controller binary
cd controller && go build ./cmd/slack-bridge/   # slack bridge binary
make test                                       # run all tests
quench check                                    # quality checks
```

## Key patterns

- **beadsapi client** (`internal/beadsapi/`) — HTTP/JSON client to beads daemon. Used by both controller and bridge.
- **podmanager** (`internal/podmanager/`) — Pod spec construction and CRUD against K8s API.
- **reconciler** (`internal/reconciler/`) — Periodic desired-vs-actual sync loop.
- **subscriber** (`internal/subscriber/`) — SSE/NATS event listener for bead lifecycle events.
- **bridge** (`internal/bridge/`) — Standalone notification subsystem: NATS subscriptions for decisions/mail beads, Slack HTTP interactions.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `BEADS_HTTP_ADDR` | `http://localhost:8080` | Beads daemon HTTP address |
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `SLACK_BOAT_TOKEN` | *(optional)* | Slack bot OAuth token |
| `SLACK_CHANNEL` | *(optional)* | Slack channel for notifications |

## Before Picking Up Any Task (Agents)

When multiple agents are running, always follow this sequence to avoid duplicating work:

1. `gb news` — Check what teammates are actively working on
2. `kd claim <id>` — Claim BEFORE starting (atomically sets `in_progress` + assignee)

`gb ready` returns beads across all projects. Only work on tasks in your assigned project unless explicitly instructed otherwise.

## Release (Golden Release Path)

One canonical path. Template: `kd formula apply kd-GwMFKXnPvR --var version=YYYY.DDD.N --http-url https://beads.gasboat.app.e2e.dev.fics.ai`

```bash
# All components are in the monorepo — one tag releases everything
./scripts/release.sh              # bump Chart.yaml, commit, tag
git push origin main <TAG>        # triggers all CI automatically:
#   - RWX docker.yml: builds + pushes all 10 images from source to GHCR
#   - RWX helm.yml: packages + pushes Helm chart to oci://ghcr.io/groblegark/charts
#   - GitHub release.yml: creates GitHub Release + triggers fics-helm-chart deploy
#   - RWX E2E auto-dispatches; failures become bug beads
```

**Emergency manual fallback** (only if RWX CI is down):
```bash
make push-all                         # build + push all images locally
helm package helm/gasboat/ --version <TAG> --app-version <TAG>
helm push gasboat-<TAG>.tgz oci://ghcr.io/groblegark/charts
gh release create <TAG> --generate-notes
```

**Deployment repo**: `~/book/fics-helm-chart/charts/gasboat/` — wrapper chart that depends on upstream OCI chart.

## Agent Image

The agent image is the container that runs Claude Code agents in K8s pods. It has two build systems that **must be kept in sync**.

### Shared Install Scripts (Single Source of Truth)

Tool versions, package lists, and install logic are defined ONCE in shared scripts under `images/agent/install/`:

| File | Purpose |
|---|---|
| `install/versions.env` | All version numbers and package lists (sourced by both Dockerfile and RWX CI) |
| `install/clis.sh` | Binary CLI tools (kubectl, gh, glab, docker, aws, helm, terraform, etc.) |
| `install/node.sh` | Node.js + Claude Code |
| `install/go-tools.sh` | Go, gopls, golangci-lint, Task CLI |
| `install/rust-tools.sh` | Rust, rust-analyzer, quench |
| `install/playwright.sh` | Playwright, Chromium, Chrome, browser system deps |

### Two Build Systems (Shared Scripts)

| System | File | When | How |
|---|---|---|---|
| **Dockerfile** | `images/agent/Dockerfile` | `docker build` / local dev | Multi-stage Docker build, calls shared scripts |
| **RWX CI** | `.rwx/docker.yml` | Push to main / tag | `crane append` with parallel cached layers, calls shared scripts |

**Both produce `ghcr.io/groblegark/gasboats/agent:<tag>`**. Both source the same `versions.env` and call the same install scripts. RWX CI adds layer extraction (dpkg copy, ldd resolution) on top.

### RWX CI Architecture

RWX splits the agent image into 7 parallel install tasks, each with its own cache key:

| Task | What it installs | Cache key |
|---|---|---|
| `agent-install-syspackages` | apt packages (sources `versions.env` for package lists) | `install/versions.env` + `.rwx/agent-syspackages.lock` |
| `agent-install-node` | Node.js, Claude Code (calls `install/node.sh`) | `install/node.sh` + `.rwx/agent-node.lock` |
| `agent-install-playwright` | Playwright, Chromium, browser system deps (sources `versions.env`) | `install/playwright.sh` + `.rwx/agent-playwright.lock` |
| `agent-install-go` | Go, gopls, golangci-lint, Task CLI (calls `install/go-tools.sh`) | `install/go-tools.sh` + `.rwx/agent-go.lock` |
| `agent-install-rust` | Rust, rust-analyzer, quench (calls `install/rust-tools.sh`) | `install/rust-tools.sh` + `.rwx/agent-rust.lock` |
| `agent-install-clis` | kubectl, gh, docker, aws, helm, terraform, etc. (calls `install/clis.sh`) | `install/clis.sh` + `.rwx/agent-clis.lock` |
| `push-agent` (assembly) | Merges all layers + gb, coop, kd binaries → `crane append` | never cached |

Each task runs in its own container on `ubuntu:24.04`. They **cannot share installed packages** — if task A installs cmake, task B cannot use it. Install dependencies locally within each task.

### How to Add a Tool

1. **Add version** to `install/versions.env` (if pinned)
2. **Add install logic** to the appropriate script:
   - apt packages → add to `BASE_PACKAGES` or `AGENT_PACKAGES` in `versions.env`
   - Binary CLI tools → add install function to `install/clis.sh`
   - Go tools → add to `install/go-tools.sh`
   - Rust tools → add to `install/rust-tools.sh`
   - npm packages → add to `install/node.sh` or `install/playwright.sh`
3. **If the tool needs build deps** in RWX (e.g. cmake for whisper-cli), ensure they're installed within the same RWX task — they won't be available from other tasks
4. **Bump cache-epoch** in the relevant `.rwx/agent-*.lock` file
5. Both build paths pick up changes automatically (no dual-editing needed)

### How to Update a Tool Version

1. Update the version in `install/versions.env`
2. Bump `cache-epoch` in the relevant `.rwx/agent-*.lock` file
3. Commit, tag, push — both Dockerfile and RWX CI use the new version

### How to Verify the Agent Image

After a build, check that tools are present:

```bash
# Export and inspect (without pulling the full image)
crane export ghcr.io/groblegark/gasboats/agent:<tag> - | tar -tf - | grep <binary-name>

# Or run a container
docker run --rm ghcr.io/groblegark/gasboats/agent:<tag> which <tool-name>
```

Key tools to verify: `claude`, `coop`, `kd`, `gb`, `playwright`, `npx`, `ffmpeg`, `tmux`, `whisper-cli`, `go`, `rustc`, `gh`, `glab`, `helm`, `terraform`, `kubectl`, `psql`, `k6`

### Common Pitfalls

- **Cross-task dependencies in RWX**: Each `agent-install-*` task runs in a fresh container. If task B needs cmake, install cmake in task B — don't rely on it being in `agent-install-syspackages`.
- **apt metapackages**: The RWX CI uses before/after `dpkg-query` diffs to auto-resolve transitive deps, so metapackages like `postgresql-client` automatically include their providers (e.g., `postgresql-client-16`). No manual package lists needed.
- **Go template `default` with empty strings**: Helm's `default` function does NOT treat empty string as falsy. Use `{{ if .Values.x }}{{ .Values.x }}{{ else }}{{ .Chart.AppVersion }}{{ end }}` instead of `{{ .Values.x | default .Chart.AppVersion }}`.

### All Images Built From Monorepo Source

All images are built from source in the monorepo's `.rwx/docker.yml` — no cross-repo dispatch needed. The monorepo CI produces all 10 images under `ghcr.io/groblegark/gasboats/`:

| Image | Source directory |
|---|---|
| `controller` | `gasboat/controller/` |
| `agent` | `gasboat/images/agent/` |
| `slack-bridge` | `gasboat/images/slack-bridge/` |
| `jira-bridge` | `gasboat/images/jira-bridge/` |
| `gitlab-bridge` | `gasboat/images/gitlab-bridge/` |
| `advice-viewer` | `gasboat/images/advice-viewer/` |
| `kbeads` | `kbeads/` |
| `coop` | `coop/` |
| `coopmux` | `coop/` |
| `beads3d` | `beads3d/` |

## Commits

Use short, imperative subject lines. Scope in parentheses: `fix(bridge): handle nil bead metadata`.

## Landing the Plane

When finishing work on this codebase:

1. **Build** — `make build` and `make build-bridge` must succeed.
2. **Run tests** — `make test` must pass.
3. **Run quench** — `quench check` must pass.
4. **Helm lint** — `helm lint helm/gasboat/` must pass.
5. **Follow existing patterns** — bridge code lives in `internal/bridge/`, K8s logic in `internal/podmanager/` and `internal/reconciler/`.
