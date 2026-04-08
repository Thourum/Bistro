# Bistro Sync Engine — Logging Guide

## Quick Start

### Default (Info level to stderr)
```bash
./bistro-sync
```

### Debug mode with file output
```bash
./bistro-sync --log-level debug --log-file logs/sync.log
```

### Info level to file
```bash
./bistro-sync --log-level info --log-file logs/sync.log
```

### Error level only
```bash
./bistro-sync --log-level error
```

---

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--ws-port` | 8080 | WebSocket server port |
| `--db-path` | ./data/sync.db | SQLite database path |
| `--replay-queue` | true | Replay offline queue on startup |
| `--log-level` | info | Log level: debug, info, warn, error |
| `--log-file` | (empty) | Write logs to file (in addition to stderr) |

---

## Log Levels

### Debug
- **When to use:** Troubleshooting issues, understanding flow
- **Output:** Everything (info + debug traces)
- **Includes:** Source file:line information
- **Example:**
```bash
./bistro-sync --log-level debug --log-file debug.log
```

### Info (Default)
- **When to use:** Normal operation
- **Output:** Important events (sync start/complete, rate limits, errors)
- **Example:**
```bash
./bistro-sync  # Default
```

### Warn
- **When to use:** Production monitoring
- **Output:** Warnings and errors only
- **Example:**
```bash
./bistro-sync --log-level warn
```

### Error
- **When to use:** Production (minimal output)
- **Output:** Errors only
- **Example:**
```bash
./bistro-sync --log-level error
```

---

## Log Output

### Stderr (Console)
All logs are printed to stderr immediately. You can redirect:

```bash
# Redirect to file
./bistro-sync 2> console.log

# Redirect to /dev/null (suppress console)
./bistro-sync 2> /dev/null
```

### Log File (Optional)
When using `--log-file`, logs are written to **both stderr and the log file**:

```bash
./bistro-sync --log-file logs/sync.log
# Output:
# - Console: stderr (immediate)
# - File: logs/sync.log (appended)
```

### Log Directory
Log directories are created automatically:

```bash
./bistro-sync --log-file logs/my-app/sync.log
# Creates: logs/my-app/ if it doesn't exist
```

---

## Common Scenarios

### 1. Troubleshooting Connection Issues
```bash
mkdir -p logs
./bistro-sync --log-level debug --log-file logs/debug.log

# Watch logs in real-time
tail -f logs/debug.log
```

**What to look for:**
- `WebSocket server listening on :8080` — server started
- `connection accepted` — client connected
- `rate limited` — rate limit hit
- `action queued` — action saved to queue
- `action failed` — action execution error

### 2. Testing Offline Queue
```bash
mkdir -p logs
./bistro-sync --log-level debug --log-file logs/queue.log --replay-queue=true

# Watch for:
# - "replaying queued action" — queue replay in progress
# - "action completed" — action executed
# - "deleted" — old actions cleaned up
```

### 3. Production Monitoring
```bash
./bistro-sync --log-level warn --log-file logs/production.log

# Only errors and warnings logged
# Minimal performance impact
```

### 4. Verifying Rate Limiting
```bash
./bistro-sync --log-level info --log-file logs/rate-limit.log

# Watch for:
# - "rate limited" events
# - "queueing action" entries
# - "replaying queued actions" on restart
```

---

## Log Format

Logs are printed in structured text format:

```
time=2026-04-08T20:17:30.123+02:00 level=INFO msg="sync completed" count=42 duration_ms=1234
time=2026-04-08T20:17:31.456+02:00 level=WARN msg="rate limited" reset_in=300
time=2026-04-08T20:17:32.789+02:00 level=ERROR msg="action failed" error="network timeout"
```

### Fields
- `time` — Timestamp (ISO 8601)
- `level` — Log level (DEBUG, INFO, WARN, ERROR)
- `msg` — Log message
- Additional fields — Context-specific (e.g., `count`, `duration_ms`, `error`)

### Debug Mode
Debug logs include source location:

```
time=2026-04-08T20:17:30.123+02:00 level=DEBUG msg="fetching prs" source=orchestrator.go:123
```

---

## Viewing Logs

### Real-time monitoring
```bash
tail -f logs/sync.log
```

### Search for errors
```bash
grep "ERROR" logs/sync.log
```

### Search for rate limits
```bash
grep "rate_limited" logs/sync.log
```

### Line count
```bash
wc -l logs/sync.log
```

### Last N lines
```bash
tail -50 logs/sync.log
```

---

## Troubleshooting

### Q: I don't see any logs
**A:** Make sure you're not redirecting stderr:
```bash
# WRONG: hides stderr
./bistro-sync > output.log

# RIGHT: captures both stdout and stderr
./bistro-sync 2> output.log

# RIGHT: use --log-file instead
./bistro-sync --log-file logs/sync.log
```

### Q: Logs are too verbose
**A:** Use higher log level (warn or error):
```bash
./bistro-sync --log-level warn
```

### Q: Log file is too large
**A:** The log file only appends. You can rotate manually:
```bash
# Rename current log
mv logs/sync.log logs/sync.log.$(date +%s)

# New logs will start fresh
./bistro-sync --log-file logs/sync.log
```

### Q: I need to see everything
**A:** Use debug level with file output:
```bash
./bistro-sync --log-level debug --log-file logs/debug.log
```

---

## Example: Full E2E Debugging

```bash
# Terminal 1: Start sync engine with debug logging
mkdir -p logs
./bistro-sync \
  --ws-port 8080 \
  --db-path data/sync.db \
  --log-level debug \
  --log-file logs/sync.log

# Terminal 2: Monitor logs in real-time
tail -f logs/sync.log

# Terminal 3: Start Electron UI
cd ../electron && pnpm dev

# Now when you use the UI:
# - Every action appears in the log
# - You can see sync progress
# - Rate limits are logged immediately
# - Queue replay is visible
```

---

## Environment Variables

No environment variables needed. All configuration is via CLI flags.

---

**Commit:** 3dd56c7
**Files:** `go/cmd/bistro-sync/main.go`, `go/internal/server/app.go`
