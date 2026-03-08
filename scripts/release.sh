#!/usr/bin/env bash
set -euo pipefail

# Gasboats monorepo release script.
# Usage: ./scripts/release.sh [--dry-run]
#
# Generates a calver tag (YYYY.DDD.N), bumps Chart.yaml,
# commits, tags, and pushes. CI handles the rest:
#   - RWX docker.yml builds + pushes all images
#   - RWX helm.yml packages + pushes Helm chart
#   - GitHub release.yml creates release + triggers deploy

DRY_RUN=false
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=true

# Generate calver: YYYY.DDD.N (year.day-of-year.sequence)
YEAR=$(date +%Y)
DOY=$(date +%-j)  # day of year, no leading zero
PREFIX="${YEAR}.${DOY}"

# Find next sequence number
EXISTING=$(git tag -l "${PREFIX}.*" | sort -t. -k3 -n | tail -1 || true)
if [[ -n "$EXISTING" ]]; then
    LAST_SEQ=$(echo "$EXISTING" | awk -F. '{print $3}')
    SEQ=$((LAST_SEQ + 1))
else
    SEQ=1
fi

VERSION="${PREFIX}.${SEQ}"
echo "Release version: ${VERSION}"

if $DRY_RUN; then
    echo "[dry-run] Would bump gasboat/helm/gasboat/Chart.yaml to ${VERSION}"
    echo "[dry-run] Would commit, tag ${VERSION}, and push"
    exit 0
fi

# Bump Chart.yaml version + appVersion
CHART_FILE="gasboat/helm/gasboat/Chart.yaml"
sed -i.bak "s/^version:.*/version: ${VERSION}/" "$CHART_FILE"
sed -i.bak "s/^appVersion:.*/appVersion: ${VERSION}/" "$CHART_FILE"
rm -f "${CHART_FILE}.bak"

# Commit and tag
git add "$CHART_FILE"
git commit -m "chore: release ${VERSION}"
git tag "${VERSION}"

echo "Tagged ${VERSION}. Push with:"
echo "  git push origin main ${VERSION}"
