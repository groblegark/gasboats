# Multi-Role Assignment for Agents

## Research & Design Document

**Epic:** kd-TNrihu5sTo
**Date:** 2026-03-07
**Status:** Complete (all phases implemented)

---

## 1. Current Architecture

### 1.1 How Roles Work Today

Agents are assigned a **single role** via the `BOAT_ROLE` environment variable. This role flows through the entire system:

1. **`gb spawn --role <role>`** — User specifies role (default: `crew`)
2. **`SpawnAgent()`** — Creates agent bead with `role` in fields + `role:<role>` label
3. **Reconciler** — Reads role from bead, passes to pod spec builder
4. **`BuildSpecFromBeadInfo()`** — Maps role to mode (`modeForRole()`), sets `BOAT_ROLE` env var
5. **Entrypoint** — Reads `BOAT_ROLE`, passes to `gb setup claude --role=<role>`
6. **`gb setup claude`** — Builds subscriptions, resolves role-specific config beads
7. **`gb prime`** — Uses role for advice matching and workflow context

### 1.2 Key Files

| File | Purpose |
|---|---|
| `controller/cmd/gb/spawn.go` | `--role` flag, calls `SpawnAgent()` |
| `controller/internal/beadsapi/client.go` | Creates bead with role field + label |
| `controller/cmd/controller/podspec.go` | `modeForRole()`, builds pod spec |
| `controller/internal/podmanager/manager.go` | Sets `BOAT_ROLE` env var in pod |
| `controller/cmd/gb/setup_config.go` | `buildSubscriptions()` — adds `role:<role>` |
| `controller/cmd/gb/config_resolve.go` | `ResolveConfigBeads()` — sorts by specificity |
| `controller/internal/advice/matching.go` | `CategorizeScope()`, `MatchesSubscriptions()` |
| `images/agent/entrypoint.sh` | Reads `BOAT_ROLE`, passes to `gb setup claude` |

### 1.3 Config Resolution Pipeline

Subscriptions are built from environment:

```go
// setup_config.go:440
func buildSubscriptions(role string) []string {
    subs := []string{"global"}
    if role != "" {
        subs = append(subs, "role:"+role)
    }
    if project := os.Getenv("BOAT_PROJECT"); project != "" {
        subs = append(subs, "project:"+project)
    }
    return subs
}
```

Config beads are sorted by specificity and merged:

```
global (0:) < project (1:) < role (2:) < agent (3:)
```

- **MergeOverride** — Later layers replace keys from earlier layers (used by most categories)
- **MergeConcat** — Hook arrays are concatenated across layers (used by `claude-hooks`)

### 1.4 Specificity Determination

`CategorizeScope()` in `matching.go` returns a single scope for a config bead based on its labels. If a bead has a `role:` label, its scope is `"role"` with sort key `"2:<rolename>"`.

**Critical limitation:** A config bead with labels `["role:crew", "role:reviewer"]` would only capture the last `role:` label processed due to the loop structure — `CategorizeScope()` returns a single (scope, target) pair.

### 1.5 Mode Mapping

`modeForRole()` maps role to execution mode:

```go
// podspec.go:18-30
func modeForRole(mode, role string) string {
    if mode != "" { return mode }
    switch role {
    case "captain", "crew": return "crew"
    case "job", "polecat":  return "job"
    default:                return "crew"
    }
}
```

### 1.6 Known Role Values

| Role | Mode | Characteristics |
|---|---|---|
| `crew` | crew | Persistent, PVC workspace, restart Always |
| `captain` | crew | Persistent, PVC workspace, restart Always |
| `job` | job | Ephemeral, EmptyDir, restart Never |
| `polecat` | job | Ephemeral, EmptyDir, restart Never |
| `thread` | crew | Thread-bound, Slack interaction |
| `reviewer` | crew | Code review specialization |

---

## 2. Problem Statement

A single role per agent limits flexibility. Real-world use cases require agents to inherit behaviors from multiple roles:

- A **thread agent** that also has **crew** capabilities (can claim work, push code)
- A **reviewer** that is also a **crew member** (can fix issues it finds during review)
- A **captain** with **reviewer** privileges (can review PRs and coordinate)

Currently, to give a thread agent crew-like behavior, you must duplicate all crew config beads with `role:thread` labels. This leads to config sprawl and maintenance burden.

---

## 3. Design Proposal

### 3.1 BOAT_ROLE Format

Change `BOAT_ROLE` from a single string to a **comma-separated list**, ordered by precedence (most specific first):

```bash
# Before
BOAT_ROLE=thread

# After
BOAT_ROLE=thread,crew
```

**Rationale:** Comma-separated is simple, shell-friendly, and requires minimal parsing. JSON arrays would be harder to work with in bash scripts and entrypoint.sh.

**Backwards compatibility:** A single value (no comma) works exactly as before.

### 3.2 Subscription Building

`buildSubscriptions()` adds a `role:<role>` subscription for **each** role:

```go
func buildSubscriptions(roles string) []string {
    subs := []string{"global"}
    for _, role := range splitRoles(roles) {
        subs = append(subs, "role:"+role)
    }
    if project := os.Getenv("BOAT_PROJECT"); project != "" {
        subs = append(subs, "project:"+project)
    }
    return subs
}

func splitRoles(roles string) []string {
    if roles == "" {
        return nil
    }
    parts := strings.Split(roles, ",")
    var result []string
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p != "" {
            result = append(result, p)
        }
    }
    return result
}
```

With `BOAT_ROLE=thread,crew`, subscriptions become:
```
["global", "role:thread", "role:crew", "project:gasboat"]
```

This means config beads labeled `role:crew` AND config beads labeled `role:thread` both match.

### 3.3 Inter-Role Precedence

**Problem:** When two role-scoped config beads match (e.g., `role:thread` and `role:crew` both define `model`), which one wins?

**Solution:** Extend specificity sort keys with role index. Roles listed earlier in `BOAT_ROLE` are more specific:

```
BOAT_ROLE=thread,crew
→ role:thread gets specificity "2:0:thread"  (index 0 = highest)
→ role:crew  gets specificity "2:1:crew"     (index 1 = lower)
```

This requires passing role order context into `CategorizeScope()` or adding a post-sort step in `ResolveConfigBeads()`.

**Proposed approach:** Add a `roleIndex` map parameter to resolution:

```go
func CategorizeScopeWithRoles(labels []string, roleIndex map[string]int) (scope, target, sortKey string) {
    scope, target = CategorizeScope(labels) // existing logic
    if scope == "role" {
        idx, ok := roleIndex[target]
        if !ok {
            idx = 99 // unknown role, lowest precedence
        }
        sortKey = fmt.Sprintf("2:%02d:%s", idx, target)
    } else {
        sortKey = GroupSortKey(scope, target)
    }
    return
}
```

**Merge order example** with `BOAT_ROLE=thread,crew`:

```
1. global                    (specificity: 0:)
2. project:gasboat           (specificity: 1:gasboat)
3. role:crew (index 1)       (specificity: 2:01:crew)
4. role:thread (index 0)     (specificity: 2:00:thread)
```

Result: `role:thread` values override `role:crew` values, which override project/global.

### 3.4 MergeConcat (Hooks)

For `claude-hooks` (MergeConcat strategy), hooks from **all** matched roles are concatenated. Order follows the same specificity sort — global hooks first, then project, then lower-precedence roles, then higher-precedence roles.

**Duplicate handling:** No deduplication needed. If both `role:crew` and `role:thread` add the same hook command, it runs twice. This is consistent with current behavior where global and role hooks already concatenate without dedup. If dedup is desired later, it can be added as a separate enhancement.

### 3.5 Agent Bead Fields

The `role` field on agent beads becomes a comma-separated list:

```json
{
    "agent": "my-agent",
    "role": "thread,crew",
    "mode": "crew",
    "project": "gasboat"
}
```

### 3.6 Agent Bead Labels

Multiple `role:` labels are added to the bead:

```go
// In SpawnAgent()
for _, role := range splitRoles(roles) {
    c.AddLabel(ctx, id, "role:"+role)
}
```

### 3.7 Mode Resolution

`modeForRole()` uses the **first** (most specific) role for mode determination:

```go
func modeForRole(mode, roles string) string {
    if mode != "" {
        return mode
    }
    // Use first role for mode determination
    firstRole := splitRoles(roles)[0]
    switch firstRole {
    case "captain", "crew": return "crew"
    case "job", "polecat":  return "job"
    default:                return "crew"
    }
}
```

### 3.8 Spawn CLI

`gb spawn` accepts comma-separated roles:

```bash
gb spawn --role thread,crew
```

### 3.9 Entrypoint Changes

Minimal changes in `entrypoint.sh` — `BOAT_ROLE` is already read as a string and passed to `gb setup claude --role="${ROLE}"`. The parsing happens in Go code.

### 3.10 Advice Matching

`BuildAgentSubscriptions()` and `EnrichAgentSubscriptions()` already derive subscriptions from bead labels. With multiple `role:` labels on the bead, these functions naturally produce multiple `role:` subscriptions. Minor adjustment needed to handle the plural-to-singular mapping for each role label.

---

## 4. Breaking Changes & Migration

### 4.1 No Breaking Changes for Single-Role Agents

- `BOAT_ROLE=crew` (no comma) works identically to today
- `buildSubscriptions("crew")` produces the same subscriptions
- `modeForRole("", "crew")` returns the same mode
- Config resolution produces the same merged output

### 4.2 Changes Required

| Component | Change | Breaking? |
|---|---|---|
| `buildSubscriptions()` | Split on comma, add multiple `role:` subs | No — superset of current behavior |
| `CategorizeScope()` / sort | Add role-index-aware sorting | No — single role gets index 0 |
| `modeForRole()` | Use first role from comma list | No — single role = first role |
| `SpawnAgent()` | Add multiple `role:` labels | No — one label for single role |
| `entrypoint.sh` | Pass full `BOAT_ROLE` string through | No — already passes as string |
| `gb spawn --role` | Accept comma-separated values | No — single value still works |
| `BuildAgentSubscriptions()` | Handle multiple role labels | No — already iterates labels |

### 4.3 Migration Path

1. Deploy code changes — all existing single-role agents continue working
2. Start creating multi-role agents: `gb spawn --role thread,crew`
3. No migration of existing config beads needed — they already have single `role:` labels that will match any agent subscribing to that role

---

## 5. Implementation Task Breakdown

### Task 1: Add `splitRoles()` helper and update `buildSubscriptions()`
- File: `controller/cmd/gb/setup_config.go`
- Add `splitRoles(roles string) []string` utility function
- Update `buildSubscriptions()` to iterate over split roles
- Unit tests for splitRoles and updated buildSubscriptions

### Task 2: Add role-index-aware specificity sorting
- Files: `controller/cmd/gb/config_resolve.go`, `controller/internal/advice/matching.go`
- Pass role ordering context through resolution pipeline
- Update `ResolveConfigBeads()` to use role-indexed sort keys
- Unit tests for multi-role specificity ordering

### Task 3: Update agent spawning to support multi-role
- Files: `controller/internal/beadsapi/client.go`, `controller/cmd/gb/spawn.go`
- `SpawnAgent()` adds multiple `role:` labels
- `gb spawn --role` help text updated for comma-separated values
- `modeForRole()` updated to use first role from comma list
- Unit tests

### Task 4: Update entrypoint and prime for multi-role
- Files: `images/agent/entrypoint.sh`, `controller/cmd/gb/prime.go`, `controller/cmd/gb/prime_shared.go`
- Ensure BOAT_ROLE comma-separated value passes through correctly
- `primeRole()` returns full comma-separated string
- `outputWorkflowContext()` builds subscriptions for all roles
- Integration verification

### Task 5: Update advice matching for multi-role labels
- Files: `controller/internal/advice/matching.go`, `controller/internal/advice/matching_test.go`
- `EnrichAgentSubscriptions()` handles multiple `role:` labels on bead
- `BuildAgentSubscriptions()` handles multi-role identity paths
- Test coverage for multi-role advice matching

### Task 6: Documentation and validation
- Update CLAUDE.md config beads section
- Add multi-role examples to docs/
- End-to-end test: spawn agent with `--role thread,crew`, verify config resolution

---

## 6. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Config key conflicts between roles | Medium | Low | Role order (index) determines winner — predictable |
| Hook duplication from multiple roles | Low | Low | Acceptable; hooks are idempotent by convention |
| Mode ambiguity (thread=crew, but what if thread,job?) | Low | Medium | First role determines mode; document clearly |
| Performance impact (more subscriptions to match) | Low | Low | N is small (2-3 roles max); linear scan already used |
| Advice over-matching (agent gets advice for all roles) | Medium | Low | Desired behavior — multi-role agent should see all relevant advice |

---

## 7. Open Questions

1. **Should there be a max number of roles?** Suggest 5 as a soft limit to prevent abuse.
2. **Should role combinations be validated?** E.g., `crew,job` is contradictory (different modes). Could warn but not block.
3. **Should config beads support multi-role scoping?** E.g., a bead with labels `["role:thread", "role:crew"]` meaning "only applies to agents with BOTH roles." Current AND-group semantics already support this via `g1:role:thread,g1:role:crew`.
4. **Should `kd show` display roles as a list?** Cosmetic but useful for clarity.
