#!/bin/sh
# Read-only GitHub evidence collection. Run from any directory; gh must be authenticated.
set -eu
DEST=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=wailsapp/wails
SINCE=2025-10-27T00:00:00Z
UNTIL=2026-10-27T00:00:00Z
# Pin the development branch snapshot, so later branch movements cannot change counts.
V3_SHA=c40713d9523ec24a90aac28aa9cf9faac2ab384f
printf 'Collected UTC: '; date -u '+%Y-%m-%dT%H:%M:%SZ'
gh api "repos/$REPO/releases/tags/v3.0.0" > "$DEST/health-pin.json"
gh api "repos/$REPO/branches/master" > "$DEST/health-branch.json"
gh api "repos/$REPO" > "$DEST/health-repository.json"
gh api "repos/$REPO/commits?sha=master&until=2026-10-26T23:59:59Z&per_page=1" > "$DEST/health-cutoff-tip.json"
gh api users/taliesin-ai > "$DEST/health-bot-taliesin.json"
gh api "repos/WebKit/WebKit/commits/e34989160bcd4d4f2cad8fe879118d0402fff434" > "$DEST/health-webkit-pin.json"
gh api --paginate --slurp "repos/$REPO/releases?per_page=100" > "$DEST/health-releases.json"
gh api --paginate --slurp "repos/$REPO/commits?sha=$V3_SHA&since=$SINCE&until=$UNTIL&per_page=100" > "$DEST/health-commits.json"
# Search excludes PRs and fixes the time horizon. Latest 100 issues by creation, not updates.
gh api --method GET search/issues -f q="repo:$REPO is:issue created:<2026-10-27" -f sort=created -f order=desc -f per_page=100 > "$DEST/health-issues.json"
node "$DEST/health.mjs" collect
node "$DEST/health.mjs" details
node "$DEST/health.mjs" summarize > "$DEST/health-summary.json"
cat "$DEST/health-summary.json"
