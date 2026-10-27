#!/usr/bin/env bash
# Gate 0 evidence for ADR 0001 (Electron vs Wails). Read-only; writes raw output
# beside this script. Run from anywhere: docs/adr/data/0001/gate0-collect.sh
set -uo pipefail

OUT="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(git -C "$OUT" rev-parse --show-toplevel)"
REPO="${BISTRO_REPO:-Thourum/Bistro}"
SERVER="${BISTRO_SERVER_DIR:-$HOME/dev/bistro-server}"
cd "$ROOT"

{
  echo "# collected $(date -u +%FT%TZ) at $(git rev-parse HEAD)"
  sw_vers
  sysctl -n hw.model machdep.cpu.brand_string hw.memsize
  pmset -g batt | head -2
  pmset -g | grep -i lowpowermode || echo "lowpowermode: (not reported)"
} >"$OUT/env.txt" 2>&1

# Telemetry access: report which credentials exist (names only, never values).
{
  for name in POSTHOG_PERSONAL_API_KEY POSTHOG_PROJECT_ID SENTRY_AUTH_TOKEN SENTRY_ORG SENTRY_PROJECT; do
    if [ -n "${!name:-}" ]; then echo "$name=set"; else echo "$name=unset"; fi
  done
  command -v sentry-cli || echo "sentry-cli: not installed"
  echo "## keys declared in electron/.env* (names only)"
  grep -hoE '^[A-Z_]+' electron/.env* "$HOME/dev/bistro/electron/.env"* 2>/dev/null | sort -u
} >"$OUT/telemetry-access.txt" 2>&1

# Raw telemetry, only when credentials are present. Each query covers 90 days of
# packaged-build events: OS mix, then crash classes.
if [ -n "${POSTHOG_PERSONAL_API_KEY:-}" ] && [ -n "${POSTHOG_PROJECT_ID:-}" ]; then
  curl -sS -H "Authorization: Bearer $POSTHOG_PERSONAL_API_KEY" -H 'Content-Type: application/json' \
    "${POSTHOG_HOST:-https://us.posthog.com}/api/projects/$POSTHOG_PROJECT_ID/query/" \
    -d '{"query":{"kind":"HogQLQuery","query":"SELECT properties.$os, properties.$os_version, properties.app_version, count(), count(DISTINCT distinct_id) FROM events WHERE timestamp > now() - INTERVAL 90 DAY GROUP BY 1,2,3 ORDER BY 4 DESC"}}' \
    >"$OUT/posthog-os-mix.json"
fi
if [ -n "${SENTRY_AUTH_TOKEN:-}" ] && [ -n "${SENTRY_ORG:-}" ] && [ -n "${SENTRY_PROJECT:-}" ]; then
  curl -sS -H "Authorization: Bearer $SENTRY_AUTH_TOKEN" \
    "https://sentry.io/api/0/projects/$SENTRY_ORG/$SENTRY_PROJECT/issues/?statsPeriod=90d&limit=100" \
    >"$OUT/sentry-issues.json"
fi

{
  echo "## DiagnosticReports total: $(ls "$HOME/Library/Logs/DiagnosticReports" | wc -l)"
  echo "## matching bistro|electron (case-insensitive):"
  ls "$HOME/Library/Logs/DiagnosticReports" | grep -iE 'bistro|electron' || echo "(none)"
} >"$OUT/ips.txt" 2>&1

{
  echo "## repo"; gh api "repos/$REPO" --jq '{stars:.stargazers_count,forks:.forks_count,has_discussions:.has_discussions,open_issues:.open_issues_count}'
  echo "## all issues (any state)"; gh issue list -R "$REPO" --state all --limit 500 --json number,title,state
  for term in size memory RAM startup slow performance lag freeze crash electron wails webkit; do
    echo "## search: $term"; gh issue list -R "$REPO" --state all --search "$term" --json number,title
  done
  echo "## releases"; gh api "repos/$REPO/releases" --jq '.[] | {tag:.tag_name,draft:.draft,downloads:[.assets[].download_count]|add}'
  echo "## tags"; git tag
} >"$OUT/github.txt" 2>&1

{
  echo "## imports of electron and shell-coupled packages (non-test)"
  grep -rnE "(from|require\()\s*['\"](electron(/[a-z]+)?|update-electron-app|electron-store|electron-log[a-z/]*|@sentry/electron[a-z/]*|posthog-node)['\"]" electron/src | grep -vE '\.test\.tsx?:'
  echo "## electron API call sites (count  api)"
  grep -rhoE '\b(app|ipcMain|ipcRenderer|contextBridge|BrowserWindow|Menu|shell|dialog|safeStorage|nativeTheme|screen|globalShortcut|session|protocol|webContents|crashReporter|autoUpdater|utilityProcess)\.[a-zA-Z]+' electron/src/main | sort | uniq -c | sort -rn
  echo "## IPC channels (handle/invoke/on/send)"
  grep -rnE "ipc(Main|Renderer)\.(handle|on|invoke|send)\(" electron/src | grep -vE '\.test\.tsx?:'
  echo "## renderer uses of window.electron.*"
  grep -rhoE 'window\.electron\.[a-zA-Z]+(\.[a-zA-Z]+)?' electron/src/renderer | sort | uniq -c | sort -rn
  echo "## forge: fuses / asar"
  grep -nE 'asar|[Ff]use' electron/forge.config.ts || echo "(no fuses configured)"
  echo "## main-process LOC (non-test)"
  find electron/src/main -name '*.ts' ! -name '*.test.ts' -print0 | xargs -0 wc -l | tail -1
} >"$OUT/shell-inventory.txt" 2>&1

{
  echo "## renderer wsClient.request/send call sites"
  grep -rnE 'wsClient\.(request|send)\b' electron/src/renderer | grep -vE '\.test\.tsx?:'
  echo "## push-event handler table keys (services/ws/events/index.ts)"
  grep -oE '^\s+[A-Z_]{3,}\s*:' electron/src/renderer/services/ws/events/index.ts | tr -d ' :' | sort
  echo "## engine message-type string literals in go/internal/server (count  type)"
  grep -rhoE '"[A-Z][A-Z_]{3,}"' go/internal/server --include='*.go' | grep -v _test | sort | uniq -c | sort -rn
  echo "## request ids and cancellation"
  grep -rnE 'onSent|SCAN_CANCEL|AUTH_CANCEL|request_?[iI]d\b|requestId' electron/src go/internal --include='*.ts' --include='*.tsx' --include='*.go' | grep -vE '(_test\.go|\.test\.tsx?):'
  echo "## origin enforcement"
  grep -rnE 'CheckOrigin|allowedOrigin|isAllowedOrigin' go/internal/ws --include='*.go' | grep -v _test
  echo "## main-process engine clients (main talks to the engine too)"
  grep -nE 'WebSocket|ws://|go:command|go:event|sync:action' electron/src/main/*.ts | grep -v '\.test\.'
  echo "## other WS consumers in this repo (outside electron/src and go/internal/server)"
  grep -rlE 'ws://|WS_PORT' --include='*.ts' --include='*.mjs' --include='*.go' --include='*.sh' --include='*.py' . 2>/dev/null \
    | grep -vE 'node_modules|^\./electron/src/|^\./go/internal/server/' || echo "(none)"
  echo "## consumers in $SERVER (excluding upstream/ submodule and node_modules)"
  if [ -d "$SERVER" ]; then
    grep -rlE 'ws://|WS_PORT|bistro-sync' "$SERVER" --include='*.ts' --include='*.tsx' --include='*.go' 2>/dev/null \
      | grep -vE 'node_modules|/upstream/' || echo "(none)"
  else
    echo "(server checkout not present)"
  fi
} >"$OUT/transport-inventory.txt" 2>&1

# Informational only (n=1, not a gate metric): size of the installed alpha.
{
  APP=/Applications/Bistro.app
  if [ -d "$APP" ]; then
    defaults read "$APP/Contents/Info.plist" CFBundleShortVersionString
    du -sk "$APP" "$APP/Contents/Frameworks" "$APP/Contents/Resources/app.asar" "$APP/Contents/Resources/go-bin"
  else
    echo "(not installed)"
  fi
} >"$OUT/installed-size.txt" 2>&1

echo "wrote $(ls "$OUT"/*.txt | wc -l | tr -d ' ') files to $OUT"
