# Beads

Beads is an event-driven work-tracking system. It manages hierarchical work items ("beads") with dependencies, labels, comments, and event history. The system exposes both gRPC and REST APIs and ships a Cobra-based CLI client (`kd`).

## kd vs gb

kd handles **data operations**: bead CRUD (create, show, list, close), deps, labels, comments, views, jacks, remotes, and `kd serve`. Agent **orchestration** commands (agent lifecycle, decisions, gates, hooks, bus emit, mail, prime, yield, ready, setup, advice, news) have moved to `gb` (gasboat CLI). Running deprecated kd commands prints a notice pointing to gb.

## Key concepts

- **Bead** — the core work item. Has a kind (`issue`, `data`, `config`), a type (`epic`, `task`, `feature`, `chore`, `bug`, `formula`, `molecule`, or custom), a status (`open`, `in_progress`, `deferred`, `closed`), and optional metadata, labels, dependencies, and comments.
- **Formula** — a reusable work definition (data-kind bead) with variable substitution. Created via `kd formula create`, instantiated via `kd formula pour` (persistent) or `kd formula wisp` (ephemeral).
- **Molecule** — an instantiated formula (issue-kind bead) acting as an epic with child beads for each step. Managed via `kd mol` commands (list, show, pour, wisp, burn, squash, bond).
- **Store** — persistence interface (`internal/store/store.go`) with a PostgreSQL implementation. All mutations are wrapped in transactions and recorded as events.
- **Events** — every mutation records an event row in Postgres and publishes to an event bus via the `Publisher` interface (`internal/events`). Publishing is optional; a no-op publisher is used when no bus is configured.
- **IDs** — nanoid format, prefixed `kd-` (see `internal/idgen`).
- **Type configuration** — bead types are extensible. Built-in types are defined in `internal/server/config.go` as the `builtinConfigs` map. Config is defined by `TypeConfig` / `FieldDef` in `internal/model/type_config.go`. Legacy type names `template` and `bundle` are aliased to `formula` and `molecule` at the API layer (see `model.TypeAliases`).

## Directory structure

```text
kbeads/
├── cmd/kd/              # CLI client (Cobra); one file per command
├── internal/
│   ├── config/          # Env-var configuration (BEADS_DATABASE_URL, etc.)
│   ├── events/          # Publisher interface and implementations
│   ├── idgen/           # Nanoid-based ID generation
│   ├── model/           # Domain types: Bead, Dependency, Comment, Event, Filter
│   ├── server/          # gRPC + HTTP server, proto ↔ model conversion, interceptors
│   └── store/           # Store interface + postgres/ implementation with migrations
├── proto/beads/v1/      # Protobuf service and message definitions
├── gen/beads/v1/        # Generated Go code from proto (do not edit)
├── Dockerfile           # Multi-stage build → /usr/local/bin/kd
├── quench.toml          # Quench quality-check config
└── go.mod
```

## Build & test

```sh
go build ./cmd/kd                # build the CLI
go test ./...                    # run all tests (uses sqlmock; no external deps)
make proto                       # regenerate protobuf Go code
```

Database tests use `go-sqlmock`; no running Postgres instance is needed for `go test`.

## Protobuf Generation

Generated Go files live in `gen/beads/v1/` and are produced from the proto definitions in `proto/beads/v1/`. The `go_package` option in each `.proto` file controls the output module path (`github.com/groblegark/kbeads/gen/beads/v1;beadsv1`).

**When to regenerate:**
- After modifying any `.proto` file under `proto/`
- After cherry-picking from upstream (`alfredjeanlab/beads`), which uses a different module path — the proto files already have the correct `go_package` for this fork, so regenerating fixes the generated code

**How to regenerate:**
```sh
make proto                       # or: ./scripts/gen-proto.sh
```

**Required tools:**
| Tool | Version | Install |
|------|---------|---------|
| `protoc` | v6.x | [GitHub releases](https://github.com/protocolbuffers/protobuf/releases) |
| `protoc-gen-go` | v1.36.11 | `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11` |
| `protoc-gen-go-grpc` | v1.6.1 | `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1` |

The plugin versions must match what is in `go.mod` (`google.golang.org/protobuf` and `google.golang.org/grpc`). Mismatched versions can produce code that fails to compile.

## Configuration (environment variables)

| Variable | Default | Purpose |
|---|---|---|
| `BEADS_DATABASE_URL` | *(required)* | Postgres connection string |
| `BEADS_GRPC_ADDR` | `:9090` | gRPC listen address |
| `BEADS_HTTP_ADDR` | `:8080` | HTTP listen address |
| `BEADS_NATS_URL` | *(optional)* | Event bus URL |
| `BEADS_SERVER` | `localhost:9090` | CLI client target address |

## Commits

Use short, imperative subject lines. Scope in parentheses when it helps: `fix(store): handle nil metadata on update`.

## Landing the plane

When finishing work on this codebase:

1. **Run tests** — `go test ./...` must pass.
2. **Run quench** — `quench check` must pass. Configured checks: cloc (max 1000 LOC), escapes (panic requires `// PANIC:` comment), agents (CLAUDE.md required), docs (TOC + links), tests (Go runner), and git conventional commits.
3. **Keep generated code in sync** — if you modify `.proto` files under `proto/`, regenerate `gen/` and commit both.
4. **Follow existing patterns** — the server layer converts between proto and model types via `internal/server/convert.go`; new RPCs should do the same. New CLI commands go in their own file under `cmd/kd/`.
5. **Record events** — any new mutation must call both `store.RecordEvent` and `publisher.Publish`.
6. **Migrations** — schema changes need a new numbered migration pair in `internal/store/postgres/migrations/`.
