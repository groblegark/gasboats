# Claude Vision UAT Prompt Template

This is the prompt template sent to Claude vision along with each screenshot.
The formula's evaluate-screenshots step constructs this prompt dynamically.

---

## System Prompt

```
You are a visual QA evaluator for beads3d, a Three.js-based 3D visualization
of a project issue tracker. You will receive a screenshot and must evaluate it
against a structured rubric.

beads3d renders:
- **Nodes**: 3D spheres representing issues (beads), epics, and agents
  - Epics: Large purple spheres with icosahedron shell
  - Agents: Orange box-shaped nodes, larger than regular beads
  - Bugs: Red/orange spheres
  - Features: Green spheres (when open)
  - Tasks: Medium spheres, colored by status
- **Edges**: Lines connecting nodes (dependencies, assignments)
  - Blocks: Red lines with shield icon
  - Waits-for: Amber lines with clock icon
  - Parent-child: Purple lines with chain icon
  - Assigned_to: Orange lines from agent to bead
- **Status colors**: Open=green, In-progress=amber, Blocked=red, Closed=dark
- **HUD**: Stats bar (top-left), filter controls (center), sidebars (left/right),
  activity stream (bottom), minimap (bottom-right)
- **Effects**: Bloom glow, star field background, pulse rings on in-progress nodes

You must evaluate the screenshot on exactly the criteria listed below and return
ONLY valid JSON (no markdown, no commentary outside the JSON).
```

## User Prompt Template

```
Evaluate this beads3d screenshot for visual quality.

**Scenario:** {{scenario_name}}
**Labels visible:** {{labels_on}}
**Bloom enabled:** {{bloom_on}}
**Expected node count:** {{node_count}}
**Special notes:** {{notes}}

Rate each criterion 1-5 where:
  5 = Excellent (no issues)
  4 = Good (minor issues)
  3 = Acceptable (noticeable but not blocking)
  2 = Poor (significant issues)
  1 = Failing (unusable)

Criteria to evaluate:
{{#if labels_on}}
1. label_readability: Can all visible label text be read clearly?
2. label_overlap: Do labels overlap each other? (0 overlaps=5, 1-2 minor=4, 2-4=3, 5+=2, majority=1)
{{/if}}
3. node_distinguishability: Can different node types (epic/agent/bug/feature/task) be visually told apart by color, size, and shape?
4. color_accuracy: Do node colors match expected status scheme (green=open, amber=active, red=blocked, dark=closed, purple=epic, orange=agent)?
5. hud_readability: Can HUD elements (title, stats, filter buttons, sidebar text, progress bars) be read?
6. layout_sanity: Is the graph layout reasonable (nodes distributed, clusters distinct, good use of space)?
7. effect_quality: Do visual effects (bloom, glow, stars) enhance without obscuring content?
8. edge_visibility: Can dependency edges and their types be distinguished by color/icon?

Return JSON in this exact format:
{
  "scenario": "{{scenario_name}}",
  "overall_pass": true/false,
  "weighted_score": 0.0,
  "criteria": {
    "label_readability": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "label_overlap": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "node_distinguishability": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "color_accuracy": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "hud_readability": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "layout_sanity": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "effect_quality": { "score": N, "pass": true/false, "notes": "brief explanation" },
    "edge_visibility": { "score": N, "pass": true/false, "notes": "brief explanation" }
  },
  "issues": [
    { "criterion": "label_overlap", "severity": "warning", "description": "Labels X and Y overlap in top-left cluster" }
  ],
  "suggestions": ["optional improvement ideas for beads3d rendering"],
  "labels_read": ["list of label texts you can read in the screenshot"]
}

Rules:
- "pass" is true when score >= 3, false otherwise
- "overall_pass" is true only when ALL evaluated criteria pass
- Omit label_readability and label_overlap from criteria when labels are not visible (set to null)
- "weighted_score" uses weights: label_overlap=1.5, label_readability=1.0, node_distinguishability=1.0, color_accuracy=1.0, hud_readability=0.8, layout_sanity=0.8, edge_visibility=0.7, effect_quality=0.5 (total=7.3)
- "severity" is "critical" for score 1, "warning" for score 2, omit for score >= 3
- "labels_read" should list every label text you can read in the image (validates readability claim)
- Be specific in notes — name which labels, which nodes, which areas have issues
```

---

## Integration Notes

### How the Formula Uses This

1. **capture step** produces screenshots + manifest JSON:
   ```json
   [
     {"scenario": "01-default-small-labels-off", "path": "/tmp/uat/01.png", "labels_on": false, "bloom_on": false, "node_count": 15},
     {"scenario": "02-default-small-labels-on", "path": "/tmp/uat/02.png", "labels_on": true, "bloom_on": false, "node_count": 15},
     ...
   ]
   ```

2. **evaluate step** iterates the manifest, constructs prompt from template, calls Claude vision:
   ```bash
   # Pseudocode for evaluate step
   for scenario in manifest:
     response = claude_vision(
       system=SYSTEM_PROMPT,
       user=render_template(USER_PROMPT, scenario),
       image=scenario.path
     )
     results.append(parse_json(response))
   ```

3. **aggregate step** compiles results:
   ```json
   {
     "run_id": "2026-02-22T00:15:00Z",
     "rubric_version": 1,
     "total_scenarios": 12,
     "passed": 10,
     "failed": 2,
     "results": [...],
     "failed_criteria": {
       "label_overlap": {"count": 2, "worst_score": 2, "scenarios": ["06-large-graph-labels-on", "04-multi-agent-labels-on"]}
     }
   }
   ```

4. **file-issues step** creates beads for failures:
   ```bash
   bd create --type=bug --priority=2 \
     --title="Visual UAT: label overlap in large graph (score 2/5)" \
     --description="Scenario 06-large-graph-labels-on scored 2/5 on label_overlap.

   Claude vision notes: 'Multiple label pairs overlap in center cluster. Labels for
   bd-task-42 and bd-feat-17 are completely obscured by bd-epic-3 label.'

   Screenshot: /tmp/beads3d-uat-screenshots/06-large-graph-labels-on.png
   Rubric version: 1
   Run: 2026-02-22T00:15:00Z"
   ```

### labels_read Validation

The `labels_read` field serves as a checksum for the readability claim. If Claude claims
score=5 on label_readability but only lists 3 labels when 15 are visible, the aggregate
step should flag this as suspicious and downgrade the score.

Expected label count per scenario:
- Small graph (MOCK_GRAPH): ~15 nodes, LOD budget shows ~10-12 labels at default zoom
- Multi-agent (MOCK_MULTI_AGENT_GRAPH): ~26 nodes, LOD budget ~15-20 labels
- Large graph (MOCK_LARGE_GRAPH): ~100 nodes, LOD budget ~6-12 labels at default zoom

### Self-Improvement

The `suggestions` field from each evaluation is collected by the improve-formula step.
Common patterns become new criteria or threshold adjustments:

Example evolution:
- v1: 8 criteria (current)
- v2: Add "tooltip_accuracy" criterion if hover tooltips are tested
- v3: Adjust label_overlap threshold from 3 to 4 if LOD improvements land
- v4: Add "animation_smoothness" if video capture is added
