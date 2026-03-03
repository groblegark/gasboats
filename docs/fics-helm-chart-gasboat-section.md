# Gasboat Chart Section for fics-helm-chart CLAUDE.md

> Add this section to fics-helm-chart/CLAUDE.md under "Core Charts"

## Gasboat Chart

**Chart**: `charts/gasboat/` — wrapper chart for the gasboat agent platform.

**Architecture**: This is a subchart wrapper that depends on the upstream gasboat OCI chart:
```yaml
dependencies:
  - name: gasboat
    version: ">=2026.0.0"
    repository: "oci://ghcr.io/groblegark/charts"
  - name: beads3d
    version: "2026.59.0"
    repository: "oci://ghcr.io/groblegark/charts"
    condition: beads3d.enabled
```

**Key Points**:
- **Fuzzy version constraint**: `helm dependency update` resolves the latest upstream chart matching `>=2026.0.0`
- **Subchart values prefix**: All upstream values must be nested under `gasboat:` prefix in `values/gasboat.yaml`
- **beads3d templates inlined**: Wrapper chart has custom beads3d templates because upstream lacks daemon token auth
- Do NOT pin exact upstream chart versions — the deploy job resolves latest automatically

**Values Files**:
- `values/gasboat.yaml` — production config (agents, bridges, NATS, etc.)
- `values/gasboat-e2e.yaml` — minimal E2E config (no agents/bridges, just controller + beads)

**Deploy Pipeline** (`.gitlab/ci/gasboat-deploy.yml`):
```bash
# Automatic: triggered by schedule, API, or web with GASBOAT_DEPLOY=1
# What it does:
#   1. helm registry login ghcr.io  (GHCR_TOKEN)
#   2. helm dependency update        (resolves latest upstream chart)
#   3. helm upgrade --install gasboat ./ -n gasboat --values values/gasboat.yaml
```

**Trigger Methods**:
- **Schedule**: `GASBOAT_DEPLOY=1` in scheduled pipeline variables
- **Manual (UI)**: Build > Pipelines > Run pipeline, set `GASBOAT_DEPLOY=1`
- **API**: `glab api -X POST projects/:id/pipeline -f ref=main -f "variables[][key]=GASBOAT_DEPLOY" -f "variables[][value]=1"`
- **Auto on changes**: Chart changes on main trigger manual deploy option
- **From gasboat release**: `release.yml` GitHub Action triggers pipeline via API

**Manual Deploy (Emergency)**:
```bash
cd charts/gasboat
echo "$GHCR_TOKEN" | helm registry login ghcr.io -u _token --password-stdin
helm dependency update
helm upgrade --install gasboat ./ -n gasboat --values values/gasboat.yaml --wait --timeout 5m
kubectl get pods -n gasboat
```
