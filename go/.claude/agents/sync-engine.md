# Bistro Go Sync Engine — Agent Context

## Mission

Build the **sync engine** — the backend that keeps Bistro's SQLite cache fresh and enables fast user interactions. You are **not building the UI**. You are building the invisible machine that powers it.

---

## What You Own

```
Go Sync Engine (sidecar)
├── WebSocket server (localhost, spawned by Electron)
├── Provider interface + GitHub adapter
├── go-git shallow fetch + diff generation
├── SQLite writes (all data persistence)
├── OAuth token storage (OS keychain)
└── Sync orchestration (fetch + write in parallel goroutines)
```

**Your boundary:** Data in, data out. Network (GitHub API) to local storage (SQLite + disk patches).

---

## Core Concepts (Read These First)

1. **Sync engine, not daemon**
   - Electron spawns you on app startup
   - You exit when parent exits
   - You are NOT a long-running service
   - No polling, no scheduled tasks — only sync when told to

2. **SQLite is the source of truth**
   - You write to it, nothing else does
   - The UI never writes to SQLite (only reads via IPC)
   - Your writes are atomic transactions
   - Every sync clears old data and writes fresh

3. **Goroutine parallelism is your superpower**
   - Fetch PR list → fan-out goroutines to fetch comments + CI for each
   - Shallow clone + diff generation runs in parallel
   - User feels instant response, not API latency

4. **Progressive API usage**
   - Fetch PR list first (lightweight: IDs + updated_at)
   - Only fetch comments/CI if updated_at changed
   - Only clone repo if diff not yet cached
   - Minimize API calls, minimize clones

---

## What NOT to Do

- ❌ Don't write to SQLite from multiple goroutines unsafely (use mutex or connection pooling)
- ❌ Don't leave goroutines hanging on network timeout (set timeouts, use context.Context)
- ❌ Don't log sensitive data (tokens, user emails, API responses with secrets)
- ❌ Don't assume network always works (handle errors gracefully, retry with backoff)
- ❌ Don't hardcode GitHub API client ID/secret (use environment variables or secure config)
- ❌ Don't run this as a standalone daemon (it is spawned and exits with parent)

---

## Stack & Dependencies

| Component | Library | Why |
|-----------|---------|-----|
| HTTP client | `net/http` (stdlib) | Fast, minimal deps |
| GitHub API | `go-octokit/go-octokit` or `google/go-github` | Official-grade lib |
| Git operations | `go-git` | Pure Go, no `git` CLI needed |
| SQLite | `mattn/go-sqlite3` (CGO) | Battle-tested, fast |
| Keychain | `zalando/go-keyring` | Cross-platform OS keychain |
| WebSocket | `gorilla/websocket` | Simple, robust |
| JSON | `encoding/json` (stdlib) | Sufficient |
| Logging | `slog` (stdlib, Go 1.21+) | Minimal, structured |

---

## API Contract with Electron

### WebSocket Events (Go → Electron)

```json
{"type": "sync_started", "timestamp": "2024-01-15T10:30:00Z"}
{"type": "pr_list_fetched", "count": 42}
{"type": "pr_updated", "pr_id": "github:owner/repo:123", "fields": ["comment_count", "ci_status"]}
{"type": "diff_ready", "pr_id": "github:owner/repo:123", "file_count": 5}
{"type": "sync_complete", "timestamp": "2024-01-15T10:30:15Z", "errors": 0}
{"type": "error", "message": "GitHub API rate limited"}
```

### WebSocket Commands (Electron → Go)

```json
{"type": "sync", "include": ["prs", "comments", "ci"]}
{"type": "approve_pr", "pr_id": "github:owner/repo:123"}
{"type": "request_changes", "pr_id": "github:owner/repo:123", "body": "..."}
{"type": "post_comment", "pr_id": "github:owner/repo:123", "file_path": "...", "line": 42, "body": "..."}
{"type": "shutdown"}
```

---

## Error Handling Philosophy

Every function returns `error`. No panics. No exceptions. Explicit handling:

```go
// Good
if err := sync.FetchPRs(ctx); err != nil {
    log.Error("fetch failed", "err", err)
    // Send error event to client
    ws.SendEvent(Event{Type: "error", Message: err.Error()})
    return
}

// Bad
data := sync.FetchPRs(ctx) // no error check
sync.WriteToSQL(data)      // crash if data is nil
```

---

## Testing

- **Unit tests** for provider adapters, store functions, git operations
- **Integration tests** with a test GitHub repo
- **Mock mode** for development (fake provider returning canned data)
- **Minimum coverage:** 80% on core paths

Run tests: `go test ./...`

---

## Development Workflow

1. Read `PLAN.md` (5 phases in order)
2. Implement Phase 1 (infrastructure) first
3. Each phase has a verification checklist — mark it off before moving forward
4. Error handling and tests **must** be done as you code, not after
5. Commit frequently with descriptive messages

---

## Key Files You'll Create

```
go/
  go.mod
  go.sum
  cmd/bistro-sync/
    main.go
  internal/
    provider/interface.go     ← Define Provider interface here
    provider/models.go
    provider/github/auth.go
    provider/github/prs.go
    provider/github/actions.go
    provider/github/ci.go
    git/clone.go
    git/diff.go
    git/cache.go
    store/db.go
    store/migrations.go
    store/prs.go
    store/comments.go
    store/files.go
    keychain/keychain.go
    keychain/provider.go
    ws/server.go
    ws/handlers.go
    ws/events.go
```

---

## Questions? Check Here

- **How do I structure the Provider interface?** → See Phase 2 of PLAN.md
- **How should I handle GitHub rate limits?** → Exponential backoff + respect X-RateLimit headers
- **How do I avoid cloning the same repo twice?** → See `git/cache.go` in Phase 3
- **How do I keep SQLite writes atomic?** → Use transactions for each sync batch
- **How do I test without hitting real GitHub API?** → Mock the Provider interface, return canned data

---

## Success Criteria

✅ App starts, Go binary spawns, WebSocket connects
✅ Pull PR list from GitHub, write to SQLite (verified via `sqlite3` CLI)
✅ Generate diffs via shallow clone (no full repo clone)
✅ Approve/comment actions complete and update SQLite
✅ All errors handled gracefully (no crashes, no hangs)
✅ Test coverage >= 80%
✅ No try/catch anywhere (Go's `if err != nil` pattern throughout)
