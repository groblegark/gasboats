# beads3d

Interactive 3D visualization of beads issues using `3d-force-graph`.

## Setup

```bash
npm install
npm run dev   # starts vite on http://localhost:3333 (or http://beads3d.local:3333)
```

### Local DNS (optional)

Add to `/etc/hosts` for a friendly URL:

```
127.0.0.1  beads3d.local
```

Then access at `http://beads3d.local:3333`. Vite binds to all interfaces (`host: true`) so this works out of the box.

## Local Dev (connecting to remote daemon)

Two approaches — both work. The `.env` file (untracked, gitignored) configures the connection.

### Option A: Direct HTTPS (default, zero setup)

```
# .env
VITE_BD_API_URL=https://gastown-next.app.e2e.dev.fics.ai
VITE_BD_TOKEN=<bearer-token-from-daemon>
```

Vite proxies `/api` → remote daemon with auth header injection. Works for RPC, SSE events, and bus SSE.

### Option B: kubectl port-forward (in-cluster, no auth needed)

```bash
# Terminal 1: port-forward the daemon service
kubectl -n gastown-next port-forward svc/gastown-next-bd-daemon-daemon 9080:9080
```

```
# .env
VITE_BD_API_URL=http://localhost:9080
# VITE_BD_TOKEN not needed for port-forward
```

### URL param override

You can also override at runtime: `?api=http://localhost:9080&token=xyz`

## Tech Stack

- `3d-force-graph` v1.79+ — Three.js-based 3D force-directed graph
- `three` — WebGL rendering, bloom post-processing
- `vite` — dev server + build

## API

Connects to beads daemon HTTP API (Connect-RPC JSON). All RPC via `POST /api/bd.v1.BeadsService/<Method>`.

Read: `Ping`, `List`, `Show`, `Stats`, `Ready`, `Blocked`, `Graph`, `DepTree`, `EpicOverview`
Write: `Update`, `Close`, `Create`
Decisions: `DecisionGet`, `DecisionList`, `DecisionListRecent`, `DecisionResolve`, `DecisionCancel`, `DecisionRemind`
SSE: `GET /api/events` (mutation stream), `GET /api/bus/events?stream=<names>` (NATS bus)

## Design Vision

Biological cell metaphor:
- Beads = vacuoles floating in cytoplasm
- Agents = ribosomes attaching and processing
- Dependencies = structural connections
- Completed work = chromatin flowing toward nucleus (codebase)
- Cell membrane = fleet boundary
