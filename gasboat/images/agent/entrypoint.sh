#!/bin/bash
# Gasboat agent entrypoint: starts a coop session with Claude.
#
# This entrypoint handles all agent roles. The controller sets role-specific
# env vars before the pod starts; this script reads BOAT_ROLE to configure
# the workspace and launch Claude with the correct context.
#
# Required environment variables (set by pod manager):
#   BOAT_ROLE       - agent role
#   BOAT_PROJECT        - project name (empty for town-level roles)
#   BOAT_AGENT      - agent name
#
# Optional:
#   BOAT_COMMAND    - command to run in screen (default: "claude --dangerously-skip-permissions")
#   BEADS_DAEMON_HOST - beads daemon URL
#   BEADS_DAEMON_PORT - beads daemon port
#   BOAT_SESSION_RESUME - set to "1" to auto-resume previous Claude session on restart
#
# Config beads (type=project) drive per-project agent configuration:
#   image            - agent container image override
#   storage_class    - PVC storage class for crew-mode workspaces
#   service_account  - K8s ServiceAccount override
#   rtk_enabled      - enable RTK token optimization
#   cpu_request/cpu_limit/memory_request/memory_limit - pod resource overrides
#   secrets          - per-project K8s secret → env var mappings (JSON)
#   env              - per-project plain env vars (JSON)
#   env_json         - per-project env var overrides (JSON object)
#   repos            - multi-repo clone definitions (JSON)
#   slack_channel    - Slack channel(s) for project resolution
#   auto_assign      - enable/disable auto-assignment for agents
#   prewarmed_pool   - pool config: {enabled, mode, role, min_size, max_size}
#
# All of the above are set on project beads (kd update <project-bead-id> -f key=value)
# and read by the controller at pod creation time. Do NOT hardcode project-specific
# behavior in this entrypoint — use config beads instead.

set -euo pipefail

ROLE="${BOAT_ROLE:-unknown}"
PROJECT="${BOAT_PROJECT:-}"
MODE="${BOAT_MODE:-crew}"
AGENT="${BOAT_AGENT:-unknown}"
WORKSPACE="/home/agent/workspace"
SESSION_RESUME="${BOAT_SESSION_RESUME:-1}"
BOAT_COMMAND="${BOAT_COMMAND:-claude --dangerously-skip-permissions}"

# Detect mock mode: BOAT_COMMAND contains "claudeless"
MOCK_MODE=0
if echo "${BOAT_COMMAND}" | grep -q "claudeless"; then
    MOCK_MODE=1
    echo "[entrypoint] Mock mode enabled (command: ${BOAT_COMMAND})"
fi

# Export platform version for version commands
if [ -f /etc/platform-version ]; then
    export BEADS_PLATFORM_VERSION
    BEADS_PLATFORM_VERSION=$(cat /etc/platform-version)
fi

echo "[entrypoint] Starting ${ROLE} agent (mode: ${MODE}): ${AGENT} (project: ${PROJECT:-none})"

# ── Workspace setup ──────────────────────────────────────────────────────

# Set global git config FIRST so safe.directory is set before any repo ops.
# The workspace volume mount is owned by root (EmptyDir/PVC) but we run as
# UID 1000 — git's dubious-ownership check would block all operations without this.
git config --global user.name "${GIT_AUTHOR_NAME:-${ROLE}}"
git config --global user.email "${GIT_AUTHOR_EMAIL:-${ROLE}@gasboat.local}"
git config --global --add safe.directory '*'

# ── Git credentials ────────────────────────────────────────────────────
# If GIT_USERNAME and GIT_TOKEN are set (from ExternalSecret), configure
# git credential-store so clone/push to github.com works automatically.
CRED_FILE="${HOME}/.git-credentials"
CRED_WRITTEN=0

if [ -n "${GIT_USERNAME:-}" ] && [ -n "${GIT_TOKEN:-}" ]; then
    echo "https://${GIT_USERNAME}:${GIT_TOKEN}@github.com" > "${CRED_FILE}"
    CRED_WRITTEN=1
    echo "[entrypoint] Git credentials configured for ${GIT_USERNAME}@github.com"
fi

# Append GitLab credentials if GITLAB_TOKEN is set.
# Uses GITLAB_USERNAME env var if provided, otherwise defaults to "oauth2" (PAT auth).
if [ -n "${GITLAB_TOKEN:-}" ]; then
    GL_USER="${GITLAB_USERNAME:-oauth2}"
    GL_HOST="${GITLAB_HOST:-gitlab.com}"
    echo "https://${GL_USER}:${GITLAB_TOKEN}@${GL_HOST}" >> "${CRED_FILE}"
    CRED_WRITTEN=1
    echo "[entrypoint] Git credentials configured for ${GL_USER}@${GL_HOST}"
fi

if [ "${CRED_WRITTEN}" = "1" ]; then
    chmod 600 "${CRED_FILE}"
    git config --global credential.helper "store --file=${CRED_FILE}"
fi

# ── Git committer email resolution ─────────────────────────────────────
# GitLab rejects pushes if the committer email does not match a verified address
# on the account. If GIT_AUTHOR_EMAIL was not explicitly provided, look it up
# from the GitLab account associated with GITLAB_TOKEN.
if [ -z "${GIT_AUTHOR_EMAIL:-}" ] && [ -n "${GITLAB_TOKEN:-}" ]; then
    GL_HOST="${GITLAB_HOST:-gitlab.com}"
    GL_EMAIL=""
    # Try /user/emails first (lists verified addresses).
    GL_EMAIL=$(curl -sf "https://${GL_HOST}/api/v4/user/emails" \
        -H "PRIVATE-TOKEN: ${GITLAB_TOKEN}" 2>/dev/null \
        | jq -r 'first(.[] | select(.confirmed_at != null) | .email) // empty' 2>/dev/null || true)
    # Fallback: derive the noreply email from /user (works when email scope is unavailable).
    if [ -z "${GL_EMAIL}" ]; then
        GL_USER_ID=$(curl -sf "https://${GL_HOST}/api/v4/user" \
            -H "PRIVATE-TOKEN: ${GITLAB_TOKEN}" 2>/dev/null \
            | jq -r '.id // empty' 2>/dev/null || true)
        if [ -n "${GL_USER_ID}" ]; then
            GL_EMAIL="${GL_USER_ID}-noreply@${GL_HOST#https://}"
        fi
    fi
    if [ -n "${GL_EMAIL}" ]; then
        git config --global user.email "${GL_EMAIL}"
        echo "[entrypoint] Git committer email resolved from GitLab account: ${GL_EMAIL}"
    else
        echo "[entrypoint] WARNING: could not resolve GitLab committer email; using ${ROLE}@gasboat.local"
        echo "[entrypoint] TIP: set GIT_AUTHOR_EMAIL env var on the project bead to fix this"
    fi
fi

# ── Repo cloning ──────────────────────────────────────────────────────────
# Clone source repos that agents reference.  Stored under ${WORKSPACE}/repos/
# (PVC-backed) so they persist across restarts.  Uses a shallow clone on first
# start for speed; skips if the repo directory already exists.
#
# Public repos: cloned unconditionally.
# Private repos: require credentials set above (GITLAB_TOKEN for gitlab.com).
#
# To force a fresh clone, delete the relevant directory under workspace/repos/.

_clone_repo() {
    local url="$1"
    local dest="$2"
    if [ -d "${dest}/.git" ]; then
        echo "[entrypoint] Repo already present: ${dest} (skipping clone)"
    else
        echo "[entrypoint] Cloning ${url} → ${dest}"
        git clone --depth 1 --quiet "${url}" "${dest}" 2>&1 \
            || echo "[entrypoint] WARNING: clone failed for ${url} (credentials missing?)"
    fi
}

# Clone reference repos declared on the project bead.
# The init container clones them into /home/agent/bot/{project}/repos/ for
# PVC-backed (crew) pods.  This block is the fallback for job mode (EmptyDir)
# where no init container runs.  Skip if the init container already cloned.
# Format: "name=https://host/path.git:branch,name2=https://host2/path2.git:branch2"
# The branch suffix is always present (controller defaults empty to "main").
INIT_REPOS_DIR="/home/agent/bot/${PROJECT}/repos"
if [ -n "${BOAT_REFERENCE_REPOS:-}" ] && [ ! -d "${INIT_REPOS_DIR}" ]; then
    REPOS_DIR="${WORKSPACE}/repos"
    mkdir -p "${REPOS_DIR}"
    IFS=',' read -ra REPO_ENTRIES <<< "${BOAT_REFERENCE_REPOS}"
    for entry in "${REPO_ENTRIES[@]}"; do
        repo_name="${entry%%=*}"
        repo_rest="${entry#*=}"
        # Strip the trailing :branch suffix. Use %:* (shortest suffix) to
        # preserve the scheme colon in https:// and any port colons.
        repo_url="${repo_rest%:*}"
        # Guard: if stripping produced a bare scheme, use the full value.
        if [ "${repo_url}" = "https" ] || [ "${repo_url}" = "http" ]; then
            repo_url="${repo_rest}"
        fi
        _clone_repo "${repo_url}" "${REPOS_DIR}/${repo_name}"
    done
fi

# Initialize git repo in workspace if not already present.
# Persistent roles keep state across restarts via PVC.
if [ ! -d "${WORKSPACE}/.git" ]; then
    echo "[entrypoint] Initializing git repo in ${WORKSPACE}"
    cd "${WORKSPACE}"
    git init -q
    git config user.name "$(git config --global user.name)"
    git config user.email "$(git config --global user.email)"
    # Keep cloned repos out of the workspace's own git index.
    echo "repos/" >> "${WORKSPACE}/.gitignore"
else
    echo "[entrypoint] Git repo already exists in ${WORKSPACE}"
    cd "${WORKSPACE}"

    # Auto-fix stale branch in workspace root on restart.
    CURRENT_BRANCH="$(git branch --show-current 2>/dev/null || true)"
    if [ -n "${CURRENT_BRANCH}" ] && [ "${CURRENT_BRANCH}" != "main" ] && [ "${CURRENT_BRANCH}" != "master" ]; then
        echo "[entrypoint] WARNING: Workspace on stale branch '${CURRENT_BRANCH}', resetting to main"
        git checkout -- . 2>/dev/null || true
        git clean -fd 2>/dev/null || true
        if git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
            git checkout main 2>/dev/null || echo "[entrypoint] ERROR: git checkout main failed"
        else
            git checkout -b main 2>/dev/null || echo "[entrypoint] ERROR: git checkout -b main failed"
        fi
        echo "[entrypoint] Workspace now on branch: $(git branch --show-current 2>/dev/null)"
    fi
fi

# ── Daemon connection ────────────────────────────────────────────────────
# Configure .beads/config.yaml so kd/gb CLIs can talk to the remote daemon.

if [ -n "${BEADS_DAEMON_HOST:-}" ]; then
    DAEMON_HTTP_PORT="${BEADS_DAEMON_HTTP_PORT:-9080}"
    DAEMON_URL="http://${BEADS_DAEMON_HOST}:${DAEMON_HTTP_PORT}"
    echo "[entrypoint] Configuring daemon connection at ${DAEMON_URL}"
    mkdir -p "${WORKSPACE}/.beads"
    cat > "${WORKSPACE}/.beads/config.yaml" <<BEADSCFG
daemon-host: "${DAEMON_URL}"
daemon-token: "${BEADS_DAEMON_TOKEN:-}"
BEADSCFG
fi

# ── Session persistence ──────────────────────────────────────────────────
#
# Persist Claude state (~/.claude) and coop session artifacts on the
# workspace PVC so they survive pod restarts.  The PVC is already mounted
# at the workspace.  We store session state under .state/ on the PVC
# and symlink the ephemeral home-directory paths into it.
#
#   PVC layout:
#     {workspace}/.state/claude/     →  symlinked from ~/.claude
#     {workspace}/.state/coop/       →  symlinked from $XDG_STATE_HOME/coop

STATE_DIR="${WORKSPACE}/.state"
CLAUDE_STATE="${STATE_DIR}/claude"
COOP_STATE="${STATE_DIR}/coop"

mkdir -p "${CLAUDE_STATE}" "${COOP_STATE}"

# Persist ~/.claude on PVC.
CLAUDE_DIR="${HOME}/.claude"
# If ~/.claude is a mount point (subPath mount from controller),
# it's already PVC-backed — skip the symlink dance.
if mountpoint -q "${CLAUDE_DIR}" 2>/dev/null; then
    echo "[entrypoint] ${CLAUDE_DIR} is a mount point (subPath) — already PVC-backed"
    CLAUDE_STATE="${CLAUDE_DIR}"
else
    rm -rf "${CLAUDE_DIR}"
    ln -sfn "${CLAUDE_STATE}" "${CLAUDE_DIR}"
    echo "[entrypoint] Linked ${CLAUDE_DIR} → ${CLAUDE_STATE} (PVC-backed)"
fi

# ── Claude credential provisioning ────────────────────────────────────
# Priority: (1) PVC credentials (preserved from refresh), (2) K8s secret mount,
# (3) CLAUDE_CODE_OAUTH_TOKEN env var (coop auto-writes .credentials.json),
# (4) ANTHROPIC_API_KEY env var (API key mode — no credentials file needed),
# (5) coopmux distribute endpoint (fetch from centralized credential manager).
# Mock mode: skip credential provisioning entirely (claudeless doesn't need credentials).
CREDS_STAGING="/tmp/claude-credentials/credentials.json"
CREDS_PVC="${CLAUDE_STATE}/.credentials.json"
if [ "${MOCK_MODE}" = "1" ]; then
    echo "[entrypoint] Mock mode — skipping credential provisioning"
elif [ -f "${CREDS_PVC}" ]; then
    echo "[entrypoint] Using existing PVC credentials (preserved from refresh)"
elif [ -f "${CREDS_STAGING}" ]; then
    cp "${CREDS_STAGING}" "${CREDS_PVC}"
    echo "[entrypoint] Seeded Claude credentials from K8s secret"
elif [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then
    echo "[entrypoint] CLAUDE_CODE_OAUTH_TOKEN set — coop will auto-write credentials"
    # Unset API key when OAuth is present to avoid Claude's
    # "Detected a custom API key" confirmation prompt.
    if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
        echo "[entrypoint] Unsetting ANTHROPIC_API_KEY (OAuth takes precedence)"
        unset ANTHROPIC_API_KEY
    fi
elif [ -n "${ANTHROPIC_API_KEY:-}" ]; then
    echo "[entrypoint] ANTHROPIC_API_KEY set — using API key mode"
elif [ -n "${COOP_MUX_URL:-}" ]; then
    # Attempt to fetch credentials from coopmux's distribute endpoint.
    echo "[entrypoint] No credentials found, requesting from coopmux..."
    mux_auth="${COOP_MUX_AUTH_TOKEN:-${COOP_BROKER_TOKEN:-}}"
    mux_creds=$(curl -sf "${COOP_MUX_URL}/api/v1/credentials/distribute" \
        ${mux_auth:+-H "Authorization: Bearer ${mux_auth}"} \
        -H 'Content-Type: application/json' \
        -d "{\"session_id\":\"${HOSTNAME:-$(hostname)}\"}" 2>/dev/null) || true
    if [ -n "${mux_creds}" ] && echo "${mux_creds}" | jq -e '.claudeAiOauth.accessToken' >/dev/null 2>&1; then
        echo "${mux_creds}" > "${CREDS_PVC}"
        echo "[entrypoint] Seeded credentials from coopmux"
    else
        echo "[entrypoint] WARNING: No Claude credentials available — agent may not authenticate"
    fi
else
    echo "[entrypoint] WARNING: No Claude credentials available — agent may not authenticate"
fi

# Set XDG_STATE_HOME so coop writes session artifacts to the PVC.
export XDG_STATE_HOME="${STATE_DIR}"
echo "[entrypoint] XDG_STATE_HOME=${XDG_STATE_HOME}"

# ── Dev tools PATH ─────────────────────────────────────────────────────

if [ -d "/usr/local/go/bin" ]; then
    export PATH="/usr/local/go/bin:${PATH}"
    echo "[entrypoint] Added /usr/local/go/bin to PATH"
fi
if [ -d "/usr/local/cargo/bin" ]; then
    export PATH="/usr/local/cargo/bin:${PATH}"
    echo "[entrypoint] Added /usr/local/cargo/bin to PATH"
fi
if [ -d "/ms-playwright" ] && [ -z "${PLAYWRIGHT_BROWSERS_PATH:-}" ]; then
    export PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
    echo "[entrypoint] Set PLAYWRIGHT_BROWSERS_PATH=/ms-playwright"
fi

# ── Resolve coop working directory ────────────────────────────────────────
#
# If the init container cloned the primary project repo (at
# /home/agent/bot/{project}/work), use that as the cwd so agents start
# inside the actual codebase. Falls back to the scaffold workspace.
# This mirrors resolveCoopWorkdir() in gb agent start --k8s.
#
# IMPORTANT: This must run BEFORE Claude settings materialization so that
# workspace-level hooks (.claude/settings.json) are written to the directory
# where Claude Code will actually start. Previously, hooks were written to
# /home/agent/workspace but Claude started in /home/agent/bot/{project}/work,
# so SessionStart hooks (gb hook prime) never fired.

COOP_WORKDIR="${WORKSPACE}"
if [ -n "${PROJECT}" ] && [ -d "/home/agent/bot/${PROJECT}/work/.git" ]; then
    COOP_WORKDIR="/home/agent/bot/${PROJECT}/work"
    export KD_WORKSPACE="${COOP_WORKDIR}"
    echo "[entrypoint] Using project repo as cwd: ${COOP_WORKDIR}"

    # Symlink repos/ into the working directory so that nudge-prompt hints
    # ("ls repos/") work from the agent's cwd.  The init container clones
    # reference repos to /home/agent/bot/{project}/repos/ which is a sibling
    # of work/, not a child — without this symlink agents cannot find them.
    REPOS_PARENT="/home/agent/bot/${PROJECT}/repos"
    if [ -d "${REPOS_PARENT}" ] && [ ! -e "${COOP_WORKDIR}/repos" ]; then
        ln -s "${REPOS_PARENT}" "${COOP_WORKDIR}/repos"
        # Keep the symlink out of git status noise.
        grep -qxF 'repos/' "${COOP_WORKDIR}/.gitignore" 2>/dev/null \
            || echo 'repos/' >> "${COOP_WORKDIR}/.gitignore"
        echo "[entrypoint] Symlinked repos/ → ${REPOS_PARENT}"
    fi
else
    echo "[entrypoint] No project repo found, using scaffold workspace: ${COOP_WORKDIR}"
fi

# ── Claude settings ──────────────────────────────────────────────────────
#
# User-level settings (permissions, plugins, model) and workspace hooks are
# both materialized from config beads via `gb setup claude`. The command
# fetches claude-settings:* and claude-hooks:* config beads, merges them by
# specificity (global → role), and writes:
#   ~/.claude/settings.json              — user-level (permissions, plugins)
#   {COOP_WORKDIR}/.claude/settings.json — workspace-level (hooks)
#
# When config beads are unavailable, --defaults installs hardcoded fallbacks.
# Mock mode: skip Claude settings entirely (claudeless doesn't use them).

if [ "${MOCK_MODE}" = "1" ]; then
    echo "[entrypoint] Mock mode — skipping Claude settings materialization"
    # Write minimal workspace settings so hooks still work with claudeless.
    mkdir -p "${COOP_WORKDIR}/.claude"
    echo '{}' > "${CLAUDE_DIR}/settings.json"
else

# Disable interactive features that interrupt autonomous agents.
export CLAUDE_CODE_ENABLE_TASKS="${CLAUDE_CODE_ENABLE_TASKS:-false}"

# Materialize settings + hooks from config beads.
# gb setup claude writes both:
#   ~/.claude/settings.json              (user-level: permissions, plugins, model)
#   {COOP_WORKDIR}/.claude/settings.json (workspace-level: hooks)
mkdir -p "${COOP_WORKDIR}/.claude"
MATERIALIZED=0

if command -v gb &>/dev/null; then
    echo "[entrypoint] Materializing settings and hooks from config beads (role: ${ROLE})"
    if gb setup claude --workspace="${COOP_WORKDIR}" --role="${ROLE}" 2>&1; then
        if grep -q '"hooks"' "${COOP_WORKDIR}/.claude/settings.json" 2>/dev/null; then
            MATERIALIZED=1
            echo "[entrypoint] Settings and hooks materialized from config beads"
        fi
    fi
fi

if [ "${MATERIALIZED}" = "0" ]; then
    echo "[entrypoint] No config beads found, installing defaults"
    gb setup claude --defaults --workspace="${COOP_WORKDIR}" 2>&1 || \
        echo "[entrypoint] WARNING: gb setup --defaults failed"
fi

# ── Skip Claude onboarding wizard ─────────────────────────────────────────

printf '{"hasCompletedOnboarding":true,"lastOnboardingVersion":"2.1.37","preferredTheme":"dark","bypassPermissionsModeAccepted":true}\n' > "${HOME}/.claude.json"

fi  # end of MOCK_MODE != 1 (Claude settings block)

# ── Start coop + Claude ──────────────────────────────────────────────────
#
# We keep bash as PID 1 (no exec) so the pod survives if Claude/coop exit.
# On child exit we clean up FIFO pipes and restart with --resume.
# SIGTERM from K8s is forwarded to coop for graceful shutdown.

cd "${COOP_WORKDIR}"

COOP_CMD="coop --agent=claude --port 8080 --port-health 9090 --cols 200 --rows 50"

# Use the pod hostname as the mux session ID so that Slack deep-links
# (which embed the pod name as the URL #fragment) resolve correctly.
export COOP_MUX_SESSION_ID="${HOSTNAME:-$(hostname)}"

# Agent command: use BOAT_COMMAND if set (e.g., "claudeless run /path/to/scenario.toml"
# for E2E testing without consuming API credits). Defaults to real Claude Code.
AGENT_CMD="${BOAT_COMMAND:-claude --dangerously-skip-permissions}"

# Append --model if CLAUDE_MODEL is set and agent command is real Claude (not claudeless).
if [ -n "${CLAUDE_MODEL:-}" ] && [ "${MOCK_MODE}" != "1" ]; then
    AGENT_CMD="${AGENT_CMD} --model ${CLAUDE_MODEL}"
fi

# Coop log level (overridable via pod env).
export COOP_LOG_LEVEL="${COOP_LOG_LEVEL:-info}"

# ── Signal forwarding ─────────────────────────────────────────────────────
# gb init handles the coop API calls (Escape + shutdown). The entrypoint
# only needs to forward SIGTERM to the coop process for hard-kill fallback.
COOP_PID=""
GB_INIT_PID=""
forward_signal() {
    if [ -n "${GB_INIT_PID}" ]; then
        # gb init handles graceful shutdown via coop API (Escape + shutdown).
        # Give it time to complete, then fall back to SIGTERM on coop PID.
        kill -"$1" "${GB_INIT_PID}" 2>/dev/null || true
        sleep 5
    fi
    if [ -n "${COOP_PID}" ] && kill -0 "${COOP_PID}" 2>/dev/null; then
        echo "[entrypoint] Forwarding $1 to coop (pid ${COOP_PID})"
        kill -"$1" "${COOP_PID}" 2>/dev/null || true
        wait "${COOP_PID}" 2>/dev/null || true
    fi
    exit 0
}
trap 'forward_signal TERM' TERM
trap 'forward_signal INT' INT

# Clean up stale standby flags from previous runs.
rm -f /tmp/standby_active /tmp/standby_expired /tmp/standby_done

# ── Restart loop ──────────────────────────────────────────────────────────
MAX_RESTARTS="${COOP_MAX_RESTARTS:-10}"
restart_count=0
MIN_RUNTIME_SECS=30

while true; do
    if [ "${restart_count}" -ge "${MAX_RESTARTS}" ]; then
        echo "[entrypoint] Max restarts (${MAX_RESTARTS}) reached, exiting"
        exit 1
    fi

    # Clean up stale FIFO pipes before each start.
    if [ -d "${COOP_STATE}/sessions" ]; then
        find "${COOP_STATE}/sessions" -name 'hook.pipe' -delete 2>/dev/null || true
    fi

    # Find latest session log for resume.
    RESUME_FLAG=""
    LATEST_LOG=""
    MAX_STALE_RETRIES=3
    STALE_COUNT=$( (find "${CLAUDE_STATE}/projects" -maxdepth 3 -name '*.jsonl.stale' -type f 2>/dev/null || true) | wc -l | tr -d ' ')
    if [ "${MOCK_MODE}" != "1" ] && [ "${SESSION_RESUME}" = "1" ] && [ -d "${CLAUDE_STATE}/projects" ] && [ "${STALE_COUNT:-0}" -lt "${MAX_STALE_RETRIES}" ]; then
        LATEST_LOG=$( (find "${CLAUDE_STATE}/projects" -maxdepth 3 -name '*.jsonl' -not -path '*/subagents/*' -type f -printf '%T@ %p\n' 2>/dev/null || true) \
            | sort -rn | head -1 | cut -d' ' -f2-)
        if [ -n "${LATEST_LOG}" ] && [ -f "${LATEST_LOG}" ]; then
            # Validate last line is complete JSON. A partial write from an
            # abrupt kill leaves an incomplete line that breaks --resume.
            last_line=$(tail -1 "${LATEST_LOG}" 2>/dev/null)
            if [ -n "${last_line}" ] && ! echo "${last_line}" | jq empty 2>/dev/null; then
                echo "[entrypoint] Truncating corrupted last line from ${LATEST_LOG}"
                # Remove the last (incomplete) line, keeping all complete lines.
                head -n -1 "${LATEST_LOG}" > "${LATEST_LOG}.tmp" && mv "${LATEST_LOG}.tmp" "${LATEST_LOG}"
            fi
            echo "[entrypoint] Found session log for resume: ${LATEST_LOG}"
            RESUME_FLAG="--resume ${LATEST_LOG}"
        fi
    elif [ "${STALE_COUNT:-0}" -ge "${MAX_STALE_RETRIES}" ]; then
        echo "[entrypoint] Skipping resume: ${STALE_COUNT} stale session(s) found (max ${MAX_STALE_RETRIES}), starting fresh"
        # Clean up stale files older than 24h to prevent permanent resume lockout.
        find "${CLAUDE_STATE}/projects" -maxdepth 3 -name '*.jsonl.stale' -type f -mmin +1440 -delete 2>/dev/null || true
    fi

    start_time=$(date +%s)

    # Build gb init flags.
    GB_INIT_FLAGS=""
    if [ "${BOAT_STANDBY:-}" = "true" ] && [ -n "${BOAT_AGENT_BEAD_ID:-}" ] && [ ! -f /tmp/standby_done ]; then
        GB_INIT_FLAGS="--standby"
    fi
    if [ "${MOCK_MODE}" = "1" ]; then
        GB_INIT_FLAGS="${GB_INIT_FLAGS} --mock"
    fi

    if [ -n "${RESUME_FLAG}" ]; then
        echo "[entrypoint] Starting coop + ${AGENT_CMD%% *} (${ROLE}/${AGENT}) with resume"
        ${COOP_CMD} ${RESUME_FLAG} -- ${AGENT_CMD} &
        COOP_PID=$!
        gb init ${GB_INIT_FLAGS} &
        GB_INIT_PID=$!
        wait "${COOP_PID}" 2>/dev/null && exit_code=0 || exit_code=$?
        COOP_PID=""
        # Stop gb init when coop exits.
        kill "${GB_INIT_PID}" 2>/dev/null || true
        wait "${GB_INIT_PID}" 2>/dev/null || true
        GB_INIT_PID=""

        if [ "${exit_code}" -ne 0 ] && [ -n "${LATEST_LOG}" ] && [ -f "${LATEST_LOG}" ]; then
            echo "[entrypoint] Resume failed (exit ${exit_code}), retiring stale session log"
            mv "${LATEST_LOG}" "${LATEST_LOG}.stale"
            echo "[entrypoint]   renamed: ${LATEST_LOG} -> ${LATEST_LOG}.stale"
        fi
    else
        echo "[entrypoint] Starting coop + ${AGENT_CMD%% *} (${ROLE}/${AGENT})"
        ${COOP_CMD} -- ${AGENT_CMD} &
        COOP_PID=$!
        gb init ${GB_INIT_FLAGS} &
        GB_INIT_PID=$!
        wait "${COOP_PID}" 2>/dev/null && exit_code=0 || exit_code=$?
        COOP_PID=""
        # Stop gb init when coop exits.
        kill "${GB_INIT_PID}" 2>/dev/null || true
        wait "${GB_INIT_PID}" 2>/dev/null || true
        GB_INIT_PID=""
    fi

    # Mark standby done after first successful gb init cycle.
    if [ "${BOAT_STANDBY:-}" = "true" ] && [ ! -f /tmp/standby_done ]; then
        touch /tmp/standby_done
    fi

    elapsed=$(( $(date +%s) - start_time ))
    echo "[entrypoint] Coop exited with code ${exit_code} after ${elapsed}s"

    # If standby TTL expired, exit cleanly — the pool manager will create
    # a replacement prewarmed agent on its next reconciliation pass.
    if [ -f /tmp/standby_expired ]; then
        echo "[entrypoint] Standby TTL expired, exiting"
        rm -f /tmp/standby_expired /tmp/standby_active /tmp/standby_done
        exit 0
    fi

    # Check if the agent requested a polite stop (gb stop sets stop_requested=true).
    # Close the bead so the reconciler stops tracking this pod, then exit cleanly.
    if [ -n "${KD_AGENT_ID:-}" ] && command -v kd &>/dev/null && command -v jq &>/dev/null; then
        stop_req=$(kd show "${KD_AGENT_ID}" --json 2>/dev/null | jq -r '.fields.stop_requested // empty')
        if [ "${stop_req}" = "true" ]; then
            echo "[entrypoint] stop_requested — closing agent bead and exiting cleanly"
            kd close "${KD_AGENT_ID}" 2>/dev/null || true
            exit 0
        fi
    fi

    # If rate-limited, sleep until the reset time before restarting.
    if [ -f /tmp/rate_limit_reset ]; then
        rate_msg=$(cat /tmp/rate_limit_reset)
        rm -f /tmp/rate_limit_reset
        # Extract reset hour from message (e.g., "resets 9pm (UTC)" -> "9pm").
        reset_hour=$(echo "${rate_msg}" | grep -oP '\d{1,2}(am|pm)' | head -1)
        sleep_secs=""
        if [ -n "${reset_hour}" ]; then
            target_epoch=$(date -u -d "today ${reset_hour}" +%s 2>/dev/null) || true
            now_epoch=$(date -u +%s)
            if [ -n "${target_epoch}" ] && [ "${target_epoch}" -le "${now_epoch}" ]; then
                target_epoch=$(date -u -d "tomorrow ${reset_hour}" +%s 2>/dev/null) || true
            fi
            if [ -n "${target_epoch}" ] && [ "${target_epoch}" -gt "${now_epoch}" ]; then
                sleep_secs=$((target_epoch - now_epoch + 60))
            fi
        fi
        if [ -n "${sleep_secs}" ]; then
            echo "[entrypoint] Rate limited — sleeping ${sleep_secs}s until ${reset_hour} UTC"
        else
            sleep_secs=1800
            echo "[entrypoint] Rate limited — sleeping ${sleep_secs}s (could not parse reset time)"
        fi
        sleep "${sleep_secs}"
        restart_count=0
        continue
    fi

    if [ "${elapsed}" -ge "${MIN_RUNTIME_SECS}" ]; then
        restart_count=0
    fi

    restart_count=$((restart_count + 1))
    echo "[entrypoint] Restarting (attempt ${restart_count}/${MAX_RESTARTS}) in 2s..."
    sleep 2
done
