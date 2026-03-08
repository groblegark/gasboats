# RWX Directory Guide

## WARNING: RWX reads ALL .yml files in this directory

RWX automatically picks up every `.yml` file in `.rwx/` as a workflow
definition. **Do NOT store backup, draft, or old workflow files here** —
they will trigger duplicate runs on every push.

If you need to keep old versions for reference, move them outside `.rwx/`
(e.g. `docs/rwx-archive/`) or use git history instead.

## Cache Control Lock Files

These lock files control when cached dependencies are rebuilt:

- **cache-config.lock** - Go + golangci-lint versions (touch to rebuild)
- **system-deps.lock** - System packages (touch weekly)
- **zig-version.lock** - Zig cross-compiler (touch to update version)
- **release-deps.lock** - Release dependencies + gh CLI (touch monthly)

To force a cache rebuild, simply touch the corresponding lock file:
```bash
touch .rwx/system-deps.lock
git add .rwx/system-deps.lock
git commit -m "chore: rebuild system dependency cache"
```

## Version Sync Requirements

Some versions appear in multiple pipeline files and must be updated together:

### Go version
Update `go.mod` — CI workflows read the Go version from `go-version-file: go.mod`.

Files to update: `go.mod`, touch `cache-config.lock`

### golangci-lint version
Hardcoded in `.github/workflows/ci.yml` (`v1.64.8`).

Files to update: `.github/workflows/ci.yml`, touch `cache-config.lock`

### Zig version
Used for cross-compiling darwin targets.

Files to update: `zig-version.lock`
