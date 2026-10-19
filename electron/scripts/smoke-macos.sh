#!/usr/bin/env bash
# Smoke-tests a packaged Bistro.app: launches it with a fixed WS_PORT, waits for
# the bistro-sync engine and a WebSocket upgrade on /ws, then shuts both down.
# Usage: electron/scripts/smoke-macos.sh path/to/Bistro.app
set -euo pipefail

readonly APP_PATH="${1:?usage: smoke-macos.sh path/to/Bistro.app}"
readonly APP_BIN="$APP_PATH/Contents/MacOS/Bistro"
readonly ENGINE_BIN="$APP_PATH/Contents/Resources/go-bin/bistro-sync"
readonly READY_TIMEOUT_SECONDS=30
readonly LOG_TAIL_LINES=80
readonly APP_SUPPORT_DIR="$HOME/Library/Application Support/Bistro"
readonly ELECTRON_LOG="$HOME/Library/Logs/Bistro/main.log"
readonly ENGINE_LOG="$APP_SUPPORT_DIR/logs/bistro-sync.log"
readonly APP_OUTPUT="$(mktemp -t bistro-smoke-app)"

APP_PID=""
PORT=""

log() {
  echo "smoke: $*"
}

tail_log() {
  local label="$1"
  local file="$2"
  echo "--- $label ($file)" >&2
  if [[ -f "$file" ]]; then
    tail -n "$LOG_TAIL_LINES" "$file" >&2
  else
    echo "(missing)" >&2
  fi
}

fail() {
  echo "smoke: FAIL: $*" >&2
  tail_log "Electron log" "$ELECTRON_LOG"
  tail_log "engine log" "$ENGINE_LOG"
  tail_log "app stdout/stderr" "$APP_OUTPUT"
  exit 1
}

cleanup() {
  if [[ -n "$APP_PID" ]] && kill -0 "$APP_PID" 2>/dev/null; then
    kill -TERM "$APP_PID" 2>/dev/null || true
    for _ in $(seq 1 10); do
      kill -0 "$APP_PID" 2>/dev/null || break
      sleep 1
    done
    kill -KILL "$APP_PID" 2>/dev/null || true
  fi
  # The engine exits when its stdin closes, but kill any survivor from this bundle.
  pkill -KILL -f "$ENGINE_BIN" 2>/dev/null || true
  rm -f "$APP_OUTPUT"
}
trap cleanup EXIT

# Handshake against /ws with the packaged renderer's Origin. Succeeds only on
# a 101 response with a correct Sec-WebSocket-Accept header.
ws_upgrade_ok() {
  node - "$PORT" <<'JS'
const crypto = require('node:crypto');
const net = require('node:net');

const port = Number(process.argv[2]);
const key = crypto.randomBytes(16).toString('base64');
const expectedAccept = crypto
  .createHash('sha1')
  .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
  .digest('base64');
const socket = net.connect({ host: '127.0.0.1', port });
const timer = setTimeout(() => {
  socket.destroy();
  process.exit(1);
}, 3000);
let response = '';

socket.on('connect', () => {
  socket.write(
    `GET /ws HTTP/1.1\r\nHost: 127.0.0.1:${port}\r\nUpgrade: websocket\r\n` +
      `Connection: Upgrade\r\nSec-WebSocket-Key: ${key}\r\n` +
      'Sec-WebSocket-Version: 13\r\nOrigin: file://\r\n\r\n',
  );
});
socket.on('data', (chunk) => {
  response += chunk.toString('latin1');
  if (!response.includes('\r\n\r\n')) return;
  clearTimeout(timer);
  socket.destroy();
  const accepted = response.includes(`Sec-WebSocket-Accept: ${expectedAccept}`);
  process.exit(/^HTTP\/1\.1 101 /.test(response) && accepted ? 0 : 1);
});
socket.on('error', () => {
  clearTimeout(timer);
  process.exit(1);
});
JS
}

[[ -x "$APP_BIN" ]] || fail "no executable at $APP_BIN"
[[ -x "$ENGINE_BIN" ]] || fail "engine not packaged: $ENGINE_BIN is missing (stale build?)"

PORT="$(node -e "const net = require('node:net'); const s = net.createServer().listen(0, '127.0.0.1', () => { console.log(s.address().port); s.close(); });")"
log "launching $APP_BIN on WS_PORT=$PORT"
# main.log is appended across launches; only lines after this offset count.
LOG_START_LINE="$(wc -l <"$ELECTRON_LOG" 2>/dev/null || echo 0)"
renderer_loaded() {
  tail -n "+$((LOG_START_LINE + 1))" "$ELECTRON_LOG" 2>/dev/null | grep -q "Renderer loaded"
}

# env -i keeps ELECTRON_RUN_AS_NODE (set by some parent shells) out of the app.
env -i HOME="$HOME" PATH="$PATH" USER="$USER" WS_PORT="$PORT" "$APP_BIN" >"$APP_OUTPUT" 2>&1 &
APP_PID=$!

deadline=$((SECONDS + READY_TIMEOUT_SECONDS))
while (( SECONDS < deadline )); do
  kill -0 "$APP_PID" 2>/dev/null || fail "Bistro exited before the engine became ready"
  if pgrep -P "$APP_PID" -f "$ENGINE_BIN" >/dev/null && ws_upgrade_ok && renderer_loaded; then
    log "engine child running, ws://127.0.0.1:$PORT/ws accepted the upgrade, renderer loaded"
    log "PASS"
    exit 0
  fi
  sleep 1
done

fail "engine, ws://127.0.0.1:$PORT/ws, or renderer not loaded after ${READY_TIMEOUT_SECONDS}s"
