# beads3d Visual UAT Rubric v1

## Overview

This rubric defines how Claude vision evaluates beads3d screenshots for visual quality.
Each screenshot is scored on 8 criteria, each rated 1-5. A scenario passes if all
criteria score >= 3 (weighted average is also computed for prioritization).

## Criteria

### 1. label_readability (weight: 1.0)

**What to evaluate:** Can the text of every visible node label be read?

| Score | Description |
|-------|-------------|
| 5 | All visible labels are crisp, clear, fully readable |
| 4 | Most labels readable, 1-2 slightly blurry but still legible |
| 3 | Majority readable, a few partially obscured but gist is clear |
| 2 | Many labels illegible — truncated, blurry, or too small |
| 1 | Labels are unreadable or not visible at all |

**Pass threshold:** >= 3
**Notes:** Labels may be intentionally hidden (labels toggled off). If labels are off, score N/A.
When labels are on, evaluate only labels that the LOD system chose to display.

### 2. label_overlap (weight: 1.5)

**What to evaluate:** Do node labels overlap each other to the point of being unreadable?

| Score | Description |
|-------|-------------|
| 5 | Zero label overlap — all labels fully separated |
| 4 | 1-2 pairs have minor overlap (<20% area) but both still readable |
| 3 | 2-4 pairs overlap, some text partially obscured but gist is clear |
| 2 | Many labels overlap — 5+ pairs, several completely obscured |
| 1 | Majority of labels overlap — text is a jumbled mess |

**Pass threshold:** >= 3
**Notes:** This is the highest-weighted criterion. Dense graphs (100+ nodes) are expected
to have some overlap at default zoom — the LOD budget should prevent most of it.
Score N/A when labels are off.

### 3. node_distinguishability (weight: 1.0)

**What to evaluate:** Can different node types be visually told apart?

Expected visual language:
- **Epic nodes**: Large purple spheres with icosahedron shell
- **Feature nodes**: Green spheres (when open)
- **Bug nodes**: Red/orange spheres
- **Task nodes**: Medium-sized spheres, colored by status
- **Agent nodes**: Orange, distinctly shaped (box-like), noticeably larger

| Score | Description |
|-------|-------------|
| 5 | All node types clearly distinguishable by color, size, and shape |
| 4 | Most types distinguishable, 1 type slightly ambiguous |
| 3 | Can tell apart major types (epic/agent vs regular), some confusion among similar |
| 2 | Difficult to distinguish most types — colors/sizes too similar |
| 1 | All nodes look the same — no visual differentiation |

**Pass threshold:** >= 3

### 4. color_accuracy (weight: 1.0)

**What to evaluate:** Do node colors match the expected status color scheme?

Expected colors:
- **Open**: Green/teal glow
- **In-progress/Active**: Amber/yellow glow
- **Blocked**: Red glow
- **Closed**: Dark/dim (desaturated)
- **Epic**: Purple
- **Agent**: Orange

| Score | Description |
|-------|-------------|
| 5 | All node colors clearly match expected scheme, status is obvious at a glance |
| 4 | Colors mostly correct, 1-2 nodes ambiguous |
| 3 | General color scheme is apparent, some nodes hard to classify |
| 2 | Colors are washed out or inconsistent — hard to determine status |
| 1 | No meaningful color differentiation — everything looks the same |

**Pass threshold:** >= 3
**Notes:** Bloom post-processing can shift perceived colors. This is expected but
should not prevent identification.

### 5. hud_readability (weight: 0.8)

**What to evaluate:** Can the HUD (heads-up display) elements be read?

HUD elements:
- **Top-left**: "BEADS3D" title, stats (open/active/blocked counts)
- **Center**: Search bar, filter buttons (status, type, agents, age)
- **Right sidebar**: Epic Progress bars, Decisions, Dep Health
- **Bottom**: Quick Actions buttons, Activity Stream, legend
- **Bottom-right**: Minimap, Project Pulse

| Score | Description |
|-------|-------------|
| 5 | All HUD text crisp, buttons clearly labeled, progress bars readable |
| 4 | Most HUD elements readable, 1-2 small items slightly hard to read |
| 3 | Major HUD elements readable (title, stats, buttons), some fine text blurry |
| 2 | Many HUD elements hard to read — small font, low contrast |
| 1 | HUD is unreadable or not visible |

**Pass threshold:** >= 3

### 6. layout_sanity (weight: 0.8)

**What to evaluate:** Is the force-directed graph layout reasonable?

| Score | Description |
|-------|-------------|
| 5 | Nodes well-distributed, clusters distinct, good use of space |
| 4 | Mostly good layout, 1 minor cluster overlap |
| 3 | Acceptable — some crowding in center but overall structure visible |
| 2 | Poor layout — most nodes bunched together, hard to see structure |
| 1 | All nodes collapsed to single point or entirely offscreen |

**Pass threshold:** >= 3
**Notes:** Force-directed layouts are non-deterministic. Some variation is expected.
Agent nodes should appear near their assigned beads. Epic nodes should anchor clusters.

### 7. effect_quality (weight: 0.5)

**What to evaluate:** Do visual effects enhance the visualization without obscuring content?

Effects to check:
- **Bloom/glow**: Nodes should glow without washing out nearby content
- **Star field**: Subtle background particles for depth
- **Pulse rings**: In-progress nodes should have visible animation indicator
- **Selection highlight**: Selected nodes should be clearly marked

| Score | Description |
|-------|-------------|
| 5 | Effects are beautiful and enhance readability/aesthetics |
| 4 | Effects look good, minor bloom bleed on 1-2 nodes |
| 3 | Effects acceptable — some bloom overdrive but not obscuring content |
| 2 | Effects problematic — bloom washing out labels or node details |
| 1 | Effects are actively harmful — can't see nodes/labels through bloom |

**Pass threshold:** >= 3

### 8. edge_visibility (weight: 0.7)

**What to evaluate:** Can dependency edges and their types be distinguished?

Expected edge types:
- **Blocks** (red): Shield icon, red line
- **Waits-for** (amber): Clock icon, amber line
- **Parent-child** (purple): Chain icon, purple line
- **Relates-to** (blue): Dot icon, blue line
- **Assigned_to** (orange): Person icon, orange line connecting agent to bead

| Score | Description |
|-------|-------------|
| 5 | All edge types clearly distinguishable by color and icon |
| 4 | Most edges distinguishable, 1-2 types slightly ambiguous |
| 3 | Can see edges exist and tell major types apart, some overlap in dense areas |
| 2 | Edges hard to distinguish — colors similar, icons not visible |
| 1 | Edges not visible or all look the same |

**Pass threshold:** >= 3
**Notes:** Dense graphs will naturally have edge crossings. That's acceptable.
The key is whether edge *types* can be told apart.

---

## Scoring

### Per-Scenario Score
```
weighted_score = sum(criterion.score * criterion.weight) / sum(criterion.weight)
```

Weights: label_overlap(1.5), label_readability(1.0), node_distinguishability(1.0),
color_accuracy(1.0), hud_readability(0.8), layout_sanity(0.8), edge_visibility(0.7),
effect_quality(0.5)

Total weight: 7.3

### Overall Pass
A scenario **passes** if ALL criteria score >= 3.
A scenario **fails** if ANY criterion scores < 3.

### Severity
- Score 1 on any criterion: **Critical** — file P1 bug
- Score 2 on any criterion: **Warning** — file P2 bug
- All scores >= 4: **Excellent** — no issues to file

---

## Scenario Expectations

Different scenarios have different expectations:

| Scenario | Labels | Expected Difficulty | Notes |
|----------|--------|--------------------|----|
| Small graph, labels off | N/A for label criteria | Easy | Should score 5 on most criteria |
| Small graph, labels on | Evaluate | Easy | 15 nodes, little overlap expected |
| Multi-agent, labels off | N/A | Medium | 26 nodes, many agents |
| Multi-agent, labels on | Evaluate | Medium | Dense agent clusters may overlap |
| Large graph, labels off | N/A | Medium | 100 nodes, complex structure |
| Large graph, labels on | Evaluate | **Hard** | LOD limits labels but overlap likely |
| Bloom enabled | Evaluate all | Medium | Bloom may affect label readability |
| Zoomed in, labels on | Evaluate | Easy | Close-up = high readability |
| Node selected | Evaluate | Easy | Highlight should be clear |
| HUD focus | Evaluate all | Easy | All chrome should be readable |
| Live data | Evaluate all | Variable | Depends on current daemon state |
| DAG/Radial/Cluster | Evaluate all | Medium | Alternative layouts |

---

## N/A Handling

When labels are toggled off:
- `label_readability` → N/A (excluded from weighted average)
- `label_overlap` → N/A (excluded from weighted average)

When effects (bloom) are off:
- `effect_quality` → score based on basic rendering (no bloom is fine, score 3+)

When graph is empty (disconnected state):
- `node_distinguishability` → N/A
- `edge_visibility` → N/A
- `layout_sanity` → N/A
- `color_accuracy` → N/A
- Only evaluate `hud_readability` and `effect_quality` (star field, background)
