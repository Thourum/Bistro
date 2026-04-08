# Phase 4 Code Review — Sync Orchestration

## Summary

Phase 4 delivers a well-structured, readable sync pipeline with correct goroutine lifecycle management and sound transaction semantics. However, it contains several issues ranging from critical (no SQL executed, context cancellation not respected inside workers) to important (single-error-per-PR loss, goroutine panic propagation, Submit blocking under backpressure). These must be resolved before Phase 5.

---

## Strengths

1. **WorkerPool lifecycle is correct.** `NewWorkerPool` starts workers, `Submit` feeds them, `Wait` closes the channel and waits — a clean, idiomatic Go pattern with no goroutine leak under normal use.

2. **Result ordering is preserved safely.** Pre-allocating `results := make([]*PRFetchResult, len(prs))` and writing by `idx` under a mutex guarantees callers get results in the same order as input without needing an extra sort step.

3. **EventEmitter uses a dual-key map correctly.** Keying `subscribers` by `<-chan` (read-only) but storing the send-end `chan<-` is idiomatic Go and prevents callers from accidentally sending to their own channel.

4. **Transaction rollback is deferred correctly.** `defer tx.Rollback()` on a committed transaction is a no-op in `database/sql`, so the pattern is safe and concise. Per-PR write failures skip that PR but do not abort the transaction, enabling partial-success writes.

5. **Clean separation of concerns.** Five focused files — types, orchestrator, concurrent, writer, events — each under ~150 LOC. This makes onboarding and Phase 5 extension straightforward.

---

## Critical Issues

### C1 — `writePR` and `writeComment` execute no SQL (data loss)

**File:** `go/internal/sync/writer.go`, functions `writePR` and `writeComment`

Both functions validate the ID then immediately `return nil` with `_ = tx`. No data is ever written to SQLite. The transaction commits successfully, all callers believe PRs and comments were persisted, but the database is always empty. This is silent data loss.

**Fix:** Implement the `INSERT OR REPLACE` statements, or — if the schema is genuinely not ready — return a sentinel error so callers know persistence was skipped. A `nil` return masquerading as success is the most dangerous outcome possible.

**Effort:** 1–2 hours once the schema migrations are in place.

---

### C2 — Workers do not respect context cancellation mid-fetch

**File:** `go/internal/sync/concurrent.go`, `FetchPRDetails`

The `ctx` is passed into each worker closure and forwarded to provider calls (`GetComments`, `GetCIStatus`). However, the loop that submits tasks to `pool.Submit` does not check `ctx.Err()` before enqueuing the next task. If context is cancelled after 50 of 1000 PRs have been enqueued, the remaining 950 tasks are already in the buffer and will execute to completion, ignoring the cancellation. `Submit` blocks when the buffer is full (`taskCh` buffer = `workers*2 = 8`), which can deadlock if the context is cancelled while the submitting goroutine is blocked.

**Fix:**
```go
for i, pr := range prs {
    select {
    case <-ctx.Done():
        break // drain remaining, return partial
    default:
    }
    // ... submit task
}
```
Inside each task, check `ctx.Err()` at the top and skip provider calls if already cancelled.

**Effort:** ~30 minutes.

---

### C3 — No panic recovery in workers (crashes the entire process)

**File:** `go/internal/sync/concurrent.go`, `worker()`

If any task panics (e.g., a nil pointer in a provider implementation), the panic propagates up through `worker()`, killing the goroutine and causing `wg.Done()` to be skipped, which makes `pool.Wait()` hang forever — or if the panic is not recovered anywhere in the call stack, it terminates the process.

**Fix:**
```go
func (p *WorkerPool) worker() {
    defer p.wg.Done()
    for task := range p.taskCh {
        func() {
            defer func() {
                if r := recover(); r != nil {
                    slog.Error("worker panic recovered", "panic", r)
                }
            }()
            task()
        }()
    }
}
```

**Effort:** 15 minutes.

---

## Important Issues

### I1 — Only the first per-PR error is recorded; subsequent errors are silently discarded

**File:** `go/internal/sync/concurrent.go`, `FetchPRDetails`

The task closure checks `if result.Error == nil` before assigning CI and diff errors. If comment fetch fails, CI and diff errors for the same PR are lost. The `Writer` then skips the PR entirely based on the first error. This is misleading: a PR whose comments failed but CI succeeded should be partially written.

**Recommendation:** Use a slice of errors, or at minimum log all errors even if only storing the first. Consider multi-error type or `errors.Join` (Go 1.20+).

**Effort:** ~1 hour.

---

### I2 — `Submit` blocks indefinitely with no context escape

**File:** `go/internal/sync/concurrent.go`, `Submit`

```go
func (p *WorkerPool) Submit(task func()) {
    p.taskCh <- task  // blocks forever if channel full and workers stuck
}
```

If workers are stalled (e.g., a provider call hangs), the buffer fills and `Submit` blocks the calling goroutine with no timeout or context check. Under a cancelled context this becomes a deadlock.

**Recommendation:**
```go
func (p *WorkerPool) SubmitCtx(ctx context.Context, task func()) error {
    select {
    case p.taskCh <- task:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**Effort:** ~30 minutes.

---

### I3 — `WriteSyncResults` commits an empty transaction when all PRs have errors

**File:** `go/internal/sync/writer.go`

If every `result.Error != nil`, the loop skips all items but still calls `tx.Commit()`. This is a benign no-op today, but once real SQL is added it opens a window for phantom commits. More importantly, the caller receives `nil` error even though nothing was written — identical to a full-success result.

**Recommendation:** Track `prCount` before commit; if `prCount == 0 && len(results) > 0`, return a descriptive error or warning so the orchestrator can surface this to the client.

**Effort:** 15 minutes.

---

### I4 — Unsubscribe while Emit is in progress can send to a closed channel

**File:** `go/internal/sync/events.go`

`Emit` acquires `RLock`. `Unsubscribe` acquires a write lock and `close(send)`. These are mutually exclusive by the RWMutex, so there is no direct data race. However: after `Emit` releases `RLock` and before the select's `case ch <- event` executes, another goroutine could call `Unsubscribe` which closes `ch`. This window does not exist because channel operations on a closed channel panic only on the send direction, and `Emit` holds `RLock` throughout the select. **This is actually safe.** Documenting this explicitly in the code would prevent future misreads.

**Recommendation:** Add a comment explaining why `RLock` is held for the full select block.

**Effort:** 5 minutes.

---

### I5 — Logger wrapper instead of `slog` (already flagged for Phase 5 pre-requisite)

**Files:** `orchestrator.go`, `writer.go`, `events.go`

All three files import `github.com/samuelasvanyi/bistro/internal/logger` rather than stdlib `log/slog`. Phase 5 plan correctly identifies this as a pre-requisite cleanup. This adds an unnecessary indirection layer and prevents structured logging configuration from the call site.

**Effort:** ~1 hour (already planned).

---

## Minor Issues

### M1 — Magic number `4` hardcoded in `FetchPRDetails`

**File:** `go/internal/sync/concurrent.go`, line calling `NewWorkerPool(4)`

`NewWorkerPool` accepts a configurable count, but `FetchPRDetails` always passes `4`. Extract a package-level constant:
```go
const DefaultWorkerCount = 4
```

### M2 — `TestConcurrentGoroutinePool` does not call `pool.Wait()`

**File:** `go/internal/sync/concurrent_test.go`

The pool is created but `Wait()` is never called, leaving 4 goroutines alive until the test process exits. Add `pool.Wait()` or `t.Cleanup(pool.Wait)`.

### M3 — `TestFetchPRDetailsSequential` unconditionally skips

**File:** `go/internal/sync/concurrent_test.go`

```go
func TestFetchPRDetailsSequential(t *testing.T) {
    if true {
        t.Skip("integration test - requires provider")
    }
}
```

This test is dead code. Since the mock provider exists and is used in integration tests, this test should be implemented using `mock.NewMockProvider()` rather than skipped. It is the single most valuable missing test — verifying that `FetchPRDetails` returns results in the correct order across concurrent workers.

### M4 — `TestContextCancellation` tests stdlib, not the codebase

**File:** `go/internal/sync/concurrent_test.go`

The test verifies that `context.WithCancel` behaves correctly — not anything in Phase 4's code. Replace with a test that passes a cancelled context to `FetchPRDetails` and verifies it returns quickly with zero or partial results.

### M5 — `SyncPRsWithDetails` hardcodes `Limit: 100`

**File:** `go/internal/sync/concurrent.go`

`filters.Limit = 100` is hardcoded. For repositories with >100 open PRs this silently truncates. Either expose the limit via `SyncRequest` or add a pagination loop.

### M6 — Integration test `TestSyncContextCancellation` asserts nothing useful

**File:** `go/internal/sync/integration_test.go`

After calling `orch.Sync(ctx, req)` with a pre-cancelled context, the test only verifies `ctx.Err() != nil` — which is always true since the test cancelled the context itself. The test should assert that `Sync` returns quickly (timeout guard) and returns an error or zero results.

---

## Phase 5 Integration Recommendations

1. **ActionExecutor should follow the same WorkerPool pattern.** PR actions (approve, request-changes, comment) are independent per-PR and can be parallelized the same way. Reuse `WorkerPool` directly — it is already a generic task executor.

2. **Add `ApplyActions` to `Orchestrator` (not a new top-level struct).** Phase 5 plan already shows this. Pass `*slog.Logger`, provider, and the existing `EventEmitter` — actions should emit `action_started`, `action_complete`, `action_error` events through the same channel WebSocket clients already subscribe to.

3. **Fix C1 (no SQL) before Phase 5 begins.** Action results also need to be persisted. If the writer is a no-op, Phase 5 persistence will be equally broken and harder to diagnose.

4. **Pass `context.Context` through `Submit` (fix I2) before Phase 5.** Actions need user-triggered cancellation (e.g., the user closes the app mid-operation). The current blocking `Submit` will deadlock on cancellation.

5. **No `Orchestrator` interface changes are needed for Phase 5.** The current fields (`provider`, `git`, `db`, `emitter`, `log`) are sufficient. Phase 5 simply adds `ActionExecutor` as an injected dependency in the same constructor style.

---

## Verdict

**STATUS: NEEDS REWORK**

**Blockers:**
- C1: `writePR`/`writeComment` execute no SQL — all data is silently dropped
- C2: Context cancellation not respected in task submission loop — potential deadlock
- C3: Worker panic crashes the process or hangs `Wait()` forever

**Ready for Phase 5: NO**

Resolve the three critical issues (estimated 2–3 hours total), then re-run `go test ./internal/sync/... -race`. After that the codebase reaches APPROVED WITH MINOR FIXES status and Phase 5 can begin.
