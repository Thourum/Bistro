# Phase 5 Review — Actions & PR Modifications

**Date:** 2026-04-08
**Status:** Plan reviewed and ready for execution
**Scope:** PR action handlers (approve, request changes, post comment) with retry logic

---

## What is Phase 5?

Phase 5 implements PR modification capabilities. Users can:
- **Approve** a PR (simple action, optional comment)
- **Request Changes** on a PR (requires comment explaining what needs fixing)
- **Post Comment** on a PR (general comment or line-specific review comment)

All actions are:
- Executed against the Provider interface (GitHub API)
- Broadcast as WebSocket events to Electron client
- Persisted in SQLite (optional state tracking)
- Retried with exponential backoff on transient failures (100ms → 5s, 2.0x multiplier)

---

## Prerequisite: Remove Logger Wrapper

**Before Phase 5 tasks:**
- Delete `internal/logger/` package (entire directory)
- Update `server/app.go`, `sync/orchestrator.go`, `sync/writer.go` to use `slog` directly
- 1-2 hour refactoring task

**Rationale:** Standard library `slog` is more powerful than wrapper. Cleaner than maintaining custom wrapper.

---

## Phase 5 Tasks Breakdown

### Task 1: Action Types & Command Definitions
- **Files:** `actions/types.go`, `actions/types_test.go`
- **What:** Define ActionRequest, ActionResult, ActionCommand types
- **Tests:** 4 validation tests
- **Time:** 30 minutes

### Task 2: Action Handlers (Provider Calls)
- **Files:** `actions/handlers.go`, `actions/handlers_test.go`
- **What:** Three handler implementations (ApproveHandler, RequestChangesHandler, PostCommentHandler)
- **Tests:** 8 handler tests
- **Time:** 1 hour

### Task 3: Retry Logic with Exponential Backoff
- **Files:** `actions/retry.go`, `actions/retry_test.go`
- **What:** ExponentialBackoff calculator, RetryWithBackoff function
- **Tests:** 4 retry tests
- **Time:** 45 minutes

### Task 4: Action Executor (Coordination Layer)
- **Files:** `actions/executor.go`, `actions/executor_test.go`
- **What:** ActionExecutor coordinating handler + event emission + optional DB writes
- **Tests:** 6 executor tests
- **Time:** 1 hour

### Task 5: Full Integration with Sync Orchestrator
- **Files:** `sync/orchestrator.go` (add ApplyActions method), `actions/integration_test.go`
- **What:** Integration between executor and orchestrator, event emission, full flow tests
- **Tests:** 4 integration tests + 2 orchestrator tests
- **Time:** 1 hour

---

## Execution Summary

| Task | Time | Tests | Type |
|------|------|-------|------|
| Prerequisite: Remove logger | 1-2h | 0 | Refactoring |
| Task 1: Types | 30m | 4 | Implementation |
| Task 2: Handlers | 1h | 8 | Implementation |
| Task 3: Retry | 45m | 4 | Implementation |
| Task 4: Executor | 1h | 6 | Implementation |
| Task 5: Integration | 1h | 6 | Implementation |
| **TOTAL** | **5-6h** | **20+ tests** | **Complete Phase** |

---

## Current State vs. Phase 5

**Already Working (Phases 1-4):**
- ✅ Sync orchestration with concurrent goroutines
- ✅ PR fetching, comments, CI status
- ✅ Diff generation and caching
- ✅ SQLite persistence with transactions
- ✅ WebSocket event streaming

**Phase 5 Adds:**
- ✅ PR modification actions (approve, request changes, post comment)
- ✅ Retry logic with exponential backoff
- ✅ Error handling with typed errors (already in Phase 2 fixes)
- ✅ Full integration with existing sync pipeline

---

## Integration Points

### Provider Interface
- Uses existing: `Provider.Approve()`, `Provider.RequestChanges()`, `Provider.PostComment()`
- Already implemented in Phase 2, fixed in remediation phase
- Typed errors support retry logic from Phase 2 fixes

### Sync Orchestrator
- New method: `Orchestrator.ApplyActions(ctx, []*ActionRequest) []*ActionResult`
- Dispatches through ActionExecutor
- Emits events: `action_complete`, `actions_complete`

### Event Streaming
- Uses existing EventEmitter from Phase 4
- New events: `action_complete` (per action), `actions_complete` (batch)
- Non-blocking emit pattern (prevents subscriber backpressure)

### SQLite
- Optional: Can track PR state changes after actions (Phase 6+ enhancement)
- Currently: Actions execute but don't update DB (focused on API calls)

---

## Success Criteria

- ✅ 20+ new tests (all passing)
- ✅ 186+ cumulative tests (100% passing)
- ✅ Action handlers fully functional
- ✅ Retry logic working with exponential backoff
- ✅ WebSocket events emitted correctly
- ✅ Integration with sync orchestrator complete
- ✅ 0 race conditions (verified with `-race` flag)
- ✅ Ready for Electron integration

---

## Breaking Changes

**None expected.** Phase 5 only adds new functionality. All existing APIs remain unchanged.

**Optional enhancements for future phases:**
- PR state tracking in SQLite (Phase 6)
- Rate limit detection and adaptive backoff (Phase 7)
- Offline action queue (Phase 7)

---

## Recommendation

✅ **Ready to execute.** All prerequisites met:
- Phase 2 fixes provide typed errors
- Phase 3 provides git operations (not directly used in Phase 5)
- Phase 4 provides orchestrator base
- Provider interface fully functional
- EventEmitter ready

**Suggested execution:** Dispatch Opus 4.6 subagent with full TDD discipline (RED → GREEN → REFACTOR → COMMIT for each task).

---

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Logger wrapper removal breaks build | Already validated in Phase 2 fixes (slog working) |
| Retry logic causes infinite loops | Tests verify max attempts exceeded |
| Context cancellation during action | Tests verify ctx handling + timeout |
| Race conditions in executor | Tests verify thread-safe event emission |

---

## Conclusion

Phase 5 is a focused, well-scoped implementation of PR actions. All dependencies are met, all prerequisites completed. Estimated 5-6 hours to complete with TDD discipline.

**Status: 🟢 READY FOR EXECUTION**
