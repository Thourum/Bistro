# Phase 2.4 — Unresolved-threads view + resolve/unresolve write-back + resolved/reopened event

> **Slice of:** Phase 2 re-scope (2026-06-20-phase-2-decomposition.md)
> **Builds on:** Phase 2.0 Foundation (2026-06-20-phase-2-0-foundation-design.md) — `review_threads` table, `ThreadProvider` capability, `GetReviewThreads`, githubv4 client, `comments.thread_id`/`kind`.
> **Shrunk scope:** thread *data layer* (ingestion, `review_threads` table, `GetReviewThreads`) moved into Phase 2.0 Foundation. This slice is UI-only + write-back mutations only.
> **Sub-slices:** 2.4.1 = resolve/unresolve write-back (was Phase 1.3); 2.4.2 = resolved/reopened WS event (was Phase 1.5).
> **Migration:** NONE — `review_threads` is created by Foundation migration 012.

---

## Goal

1. Surface every review thread in the PR via `gu` keystroke: show lifecycle state (Open / Resolved / Outdated) from `review_threads.resolved` / `outdated` / `state_known` populated by Foundation.
2. Let the user resolve or unresolve a thread in-app via GraphQL mutations, routed through the existing offline action queue.
3. Push a `THREAD_RESOLVED` / `THREAD_REOPENED` WS event so the UI updates immediately on mutation success — no full sync cycle needed.
4. Degrade honestly when GraphQL is unavailable: state shows "unknown", resolve/unresolve action is disabled.

---

## Settled decisions (do not re-litigate)

| # | Decision |
|---|----------|
| D1 | `resolveReviewThread` / `unresolveReviewThread` are GraphQL-only mutations. REST cannot do this. Confirmed. |
| D2 | Thread state is read from `review_threads` (written by Foundation). This slice never re-reads threads from the API for display. |
| D3 | Mutations route through the **existing** offline action queue (`store.ActionQueue` + `actions.Executor`). No parallel queue. |
| D4 | GraphQL capability is the **same** `shurcooL/githubv4` client Foundation adds in `github/graphql.go`. `ThreadResolver` is a new optional capability interface, sibling to `ThreadProvider`, extending the capability pattern. |
| D5 | `gu` opens a new `'threads'` AppMode. Escape returns to `'detail'`. Find-in-page is native browser `Ctrl+F` / `Cmd+F` within the overlay (no custom impl needed for v1). |
| D6 | Merge-readiness indicator: in `'detail'` mode, a footer badge shows "N open threads" derived from `review_threads` query. Zero = green / merged-ready indicator; nonzero = amber with count. |
| D7 | `state_known = 0` rows render lifecycle as "Unknown" with resolve action disabled; never guess. |
| D8 | neverthrow `Result`/`ResultAsync` in TS; idiomatic `fmt.Errorf("%w", err)` in Go; no `try/catch` in committed code. |
| D9 | The `ThreadResolver` capability is discovered by type assertion, identical pattern to `ThreadProvider`. |

---

## Architecture / module layout

### Go — new and modified files

```
go/internal/provider/
  interface.go                  MODIFY — add ThreadResolver capability interface
  github/
    graphql.go                  MODIFY — add ResolveThread / UnresolveThread mutation methods
    threads.go                  (exists after Foundation) — no change in 2.4

go/internal/actions/
  types.go                      MODIFY — add ResolveThreadCommand / UnresolveThreadCommand ActionCommand constants + ResolveThreadPayload struct
  handlers.go                   MODIFY — add resolveThreadHandler / unresolveThreadHandler implementing Handler
  executor.go                   MODIFY — add cases in handlerFor(); extend emitEvent()

go/internal/sync/
  orchestrator.go               MODIFY — extend ApplyAction: after successful resolve/unresolve, update review_threads.resolved locally + emit THREAD_RESOLVED/THREAD_REOPENED event

go/internal/store/
  review_thread_store.go        NEW — SetThreadResolved(ctx, threadID string, resolved bool) error (thin wrapper over review_threads UPDATE)
```

### TypeScript — new and modified files

```
electron/src/renderer/
  hooks/useWebSocket.ts         MODIFY — add THREAD_RESOLVED / THREAD_REOPENED case handlers
  store/ui.ts                   MODIFY — add 'threads' to AppMode union; add threadsViewPRId?: string field
  utils/keybindings.ts          MODIFY — add detail.threads binding (gu sequence); add threads.resolve / threads.unresolve bindings; add threads.escape back to detail
  pages/PRDetailPage.tsx        MODIFY — render merge-readiness badge; add 'threads' mode guard rendering <UnresolvedThreadsView>
  components/
    UnresolvedThreadsView.tsx   NEW — full list view: thread rows, lifecycle badges, resolve/unresolve action
    MergeReadinessBadge.tsx     NEW — footer badge showing open-thread count
```

---

## Provider capability interface

```go
// go/internal/provider/interface.go

// ThreadResolver is an optional capability for providers that support
// resolve/unresolve mutations via GraphQL. Discovered by type assertion.
// Providers that do not implement this interface (REST-only) degrade gracefully.
type ThreadResolver interface {
    // ResolveThread resolves an open review thread by its provider-opaque ID.
    ResolveThread(ctx context.Context, threadID string) error
    // UnresolveThread reopens a resolved review thread by its provider-opaque ID.
    UnresolveThread(ctx context.Context, threadID string) error
}
```

### GitHub implementation — `github/graphql.go` additions

The githubv4 client is already constructed in Foundation (`graphql.go`). Add two mutation methods on the existing struct:

```go
// ResolveThread executes the resolveReviewThread GraphQL mutation.
// threadID is the global node ID (e.g. "PRRT_kwDOA...").
func (g *GraphQLClient) ResolveThread(ctx context.Context, threadID string) error {
    var m struct {
        ResolveReviewThread struct {
            Thread struct {
                ID githubv4.ID
            }
        } `graphql:"resolveReviewThread(input: $input)"`
    }
    input := githubv4.ResolveReviewThreadInput{ThreadID: githubv4.ID(threadID)}
    return fmt.Errorf("resolveReviewThread: %w", g.client.Mutate(ctx, &m, map[string]interface{}{"input": input}))
}

// UnresolveThread executes the unresolveReviewThread GraphQL mutation.
func (g *GraphQLClient) UnresolveThread(ctx context.Context, threadID string) error {
    var m struct {
        UnresolveReviewThread struct {
            Thread struct {
                ID githubv4.ID
            }
        } `graphql:"unresolveReviewThread(input: $input)"`
    }
    input := githubv4.UnresolveReviewThreadInput{ThreadID: githubv4.ID(threadID)}
    return fmt.Errorf("unresolveReviewThread: %w", g.client.Mutate(ctx, &m, map[string]interface{}{"input": input}))
}
```

`github.Client` embeds `*GraphQLClient` (added in Foundation). The compile-time assertion `var _ provider.ThreadResolver = (*GraphQLClient)(nil)` lives in `graphql.go`.

---

## Action queue — new commands

### `go/internal/actions/types.go` additions

```go
const (
    // existing ...
    ResolveThreadCommand   ActionCommand = "resolve_thread"
    UnresolveThreadCommand ActionCommand = "unresolve_thread"
)

// ResolveThreadPayload is JSON-serialised into ActionQueue.payload.
type ResolveThreadPayload struct {
    ThreadID string `json:"thread_id"`
    PRID     string `json:"pr_id"`
}
```

`ActionRequest.Validate()` gains two new cases:
- `ResolveThreadCommand` / `UnresolveThreadCommand`: require non-empty `Body` (carries the thread node ID) and non-empty `PRID`.

### Handler wiring — `go/internal/actions/handlers.go`

New unexported types `resolveThreadHandler` and `unresolveThreadHandler`, each implementing the `Handler` interface (`Handle(ctx, req) (*ActionResult, error)`):

1. Type-assert the provider to `provider.ThreadResolver`. If the assertion fails, return a permanent error ("provider does not support thread resolution") so the queue marks it failed rather than retrying.
2. Call `ResolveThread` or `UnresolveThread` with `req.Body` as the thread ID.
3. On success return `&ActionResult{PRID: req.PRID, Action: req.Action, Success: true}`.
4. On error: if the error wraps a 404 / "not found" message, return a permanent error (thread deleted upstream); otherwise return a transient error so the queue retries.

### Executor wiring — `go/internal/actions/executor.go`

`handlerFor` switch gains:
```go
case ResolveThreadCommand:
    return &resolveThreadHandler{provider: ae.provider}, true
case UnresolveThreadCommand:
    return &unresolveThreadHandler{provider: ae.provider}, true
```

`emitEvent` switch gains:
```go
case ResolveThreadCommand:
    ae.emitter.Emit(&events.Event{
        Type: "thread_resolved",
        Data: map[string]interface{}{"pr_id": req.PRID, "thread_id": req.Body},
        Timestamp: time.Now(),
    })
case UnresolveThreadCommand:
    ae.emitter.Emit(&events.Event{
        Type: "thread_reopened",
        Data: map[string]interface{}{"pr_id": req.PRID, "thread_id": req.Body},
        Timestamp: time.Now(),
    })
```

---

## Store helper

### `go/internal/store/review_thread_store.go` (new, ~40 lines)

```go
// SetThreadResolved updates the resolved flag on a review thread.
// Called by the orchestrator after a successful mutation so the local cache
// stays consistent without waiting for the next sync cycle.
func (db *DB) SetThreadResolved(ctx context.Context, threadID string, resolved bool) error {
    resolvedInt := 0
    if resolved {
        resolvedInt = 1
    }
    _, err := db.sqlDB.ExecContext(ctx,
        `UPDATE review_threads SET resolved = ? WHERE id = ?`,
        resolvedInt, threadID,
    )
    if err != nil {
        return fmt.Errorf("SetThreadResolved(%s): %w", threadID, err)
    }
    return nil
}
```

---

## Orchestrator — ApplyAction extension + event emission

`orchestrator.go` `ApplyAction` already calls `executor.Execute` and returns a result. After a successful `ResolveThreadCommand` / `UnresolveThreadCommand` execution, add a post-action hook:

```go
if result.Success {
    switch req.Action {
    case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
        resolved := req.Action == actions.ResolveThreadCommand
        if err := o.store.SetThreadResolved(ctx, req.Body, resolved); err != nil {
            o.log.Warn("failed to update review_threads after mutation",
                "thread_id", req.Body, "error", err)
            // non-fatal: next sync will correct the cache
        }
        // Event is already emitted inside executor.emitEvent above;
        // the WS server subscribes to the EventEmitter and broadcasts it.
    }
}
```

The EventEmitter → WS broadcast path already exists: `events.EventEmitter.Emit` → WS server subscriber → `Broadcast` to all clients. No new wiring needed.

### WS event names and payload shapes

| Go event type | WS message type | Payload fields |
|---|---|---|
| `thread_resolved` | `THREAD_RESOLVED` | `{ pr_id: string, thread_id: string }` |
| `thread_reopened` | `THREAD_REOPENED` | `{ pr_id: string, thread_id: string }` |

The WS server translates Go snake_case event type → TS SCREAMING_SNAKE_CASE following the existing `pr_approved` → `PR_APPROVED` convention in `ws/server.go`.

---

## Schema

**No migration required.** `review_threads` (with `resolved`, `outdated`, `state_known`, `pr_id`, `file_path`, `line`) is created by Foundation migration `012`. The `SetThreadResolved` helper issues a plain `UPDATE` against that existing table.

If Foundation's migration `012` has not yet landed when 2.4 is implemented, the implementer must confirm `012` is merged first. 2.4 is explicitly blocked on 2.0.

---

## Renderer

### AppMode extension — `electron/src/renderer/store/ui.ts`

Add `'threads'` to the mode union:
```ts
mode: 'list' | 'detail' | 'diff' | 'help' | 'search' | 'settings' | 'threads';
```

Add one field to `UIState`:
```ts
threadsViewPRId?: string;   // which PR's threads are shown; undefined when not in 'threads' mode
```

Initial value: `undefined`.

### WebSocket event handlers — `electron/src/renderer/hooks/useWebSocket.ts`

In the `onMessage` switch:

```ts
case 'THREAD_RESOLVED':
  handleThreadResolved(message.data as ThreadStateChangedData);
  break;
case 'THREAD_REOPENED':
  handleThreadReopened(message.data as ThreadStateChangedData);
  break;
```

`ThreadStateChangedData`:
```ts
interface ThreadStateChangedData {
  pr_id: string;
  thread_id: string;
}
```

`handleThreadResolved` / `handleThreadReopened`:
- Invalidate the TanStack Query cache for the key `['review_threads', data.pr_id]` (via `queryClient.invalidateQueries`). The `UnresolvedThreadsView` and `MergeReadinessBadge` both use this query key and will re-render with fresh data.
- Do not mutate appStore directly; keep thread state in React Query cache, not TanStack Store.

### `UnresolvedThreadsView` component — `electron/src/renderer/components/UnresolvedThreadsView.tsx`

Props:
```ts
interface UnresolvedThreadsViewProps {
  prId: string;
}
```

Behaviour:
- Fetches `GET /api/review-threads?pr_id=<prId>` (the WS `get_review_threads` inbound message, or a REST endpoint added in Foundation's server handler layer). Caches under `['review_threads', prId]`.
- Renders a scrollable list of `ThreadRow` sub-components grouped by file, then sorted: Open → Outdated → Resolved.
- Each row shows: lifecycle badge (Open = amber, Resolved = green, Outdated = dim, Unknown = grey), file path + line, first comment body (truncated at 120 chars), author.
- When `state_known = false`: lifecycle badge shows "Unknown"; resolve/unresolve button absent.
- Resolve / Unresolve button: sends `{ type: 'action', action: 'resolve_thread' | 'unresolve_thread', pr_id, thread_id }` inbound WS message to trigger `ApplyAction`. Disabled while `pendingAction` is set for that thread.
- Find-in-page: the standard browser find (`Cmd+F` / `Ctrl+F`) works naturally since this is rendered DOM. No custom implementation.
- Returns `Result<Thread[], FetchError>` from the query hook (neverthrow wrapping via a thin `useReviewThreads` hook).

### `MergeReadinessBadge` component — `electron/src/renderer/components/MergeReadinessBadge.tsx`

Props:
```ts
interface MergeReadinessBadgeProps {
  prId: string;
}
```

- Queries `['review_threads', prId]` (same cache key — no extra network call).
- Counts rows where `resolved = 0 AND outdated = 0` (open, non-outdated).
- `count === 0` → green dot + "Ready" (or nothing if `state_known = false` for all threads).
- `count > 0` → amber dot + "{N} open thread{s}".
- `state_known` uniformly false → renders nothing (no misleading green).
- Shown in the `PRDetailPage` footer, always visible in `'detail'` mode.

### Keybindings — `electron/src/renderer/utils/keybindings.ts`

Added to `detailBindings`:
```ts
{
  actionId: 'detail.threads',
  keys: ['g', 'u'],
  description: 'Unresolved threads',
  modes: ['detail'],
  action: (store) => {
    const prId = store.state.selectedPRId; // existing field
    store.setState((s) => ({ ...s, mode: 'threads', threadsViewPRId: prId }));
  },
},
```

New `threadsBindings` array (active in `'threads'` mode only):
```ts
{
  actionId: 'threads.resolve',
  keys: 'r',
  description: 'Resolve focused thread',
  modes: ['threads'],
  // action: dispatches resolve WS message for focused thread
},
{
  actionId: 'threads.unresolve',
  keys: 'u',
  description: 'Unresolve focused thread',
  modes: ['threads'],
  // action: dispatches unresolve WS message for focused thread
},
```

`globalBindings` Escape handler gains a `'threads'` branch:
```ts
} else if (mode === 'threads') {
  store.setState((s) => ({ ...s, mode: 'detail', threadsViewPRId: undefined }));
}
```

`getEffectiveBindings()` includes `threadsBindings` in its array.

### `PRDetailPage.tsx` modifications

1. Import and render `<MergeReadinessBadge prId={pr.id} />` in the footer bar.
2. When `mode === 'threads'` and `threadsViewPRId`, render `<UnresolvedThreadsView prId={threadsViewPRId} />` as a full-panel overlay (absolute positioned, same z-level as diff view).

---

## Control flow

### Read path (threads view)

```
gu pressed (detail mode)
  → UIState.mode = 'threads', threadsViewPRId = prId
  → UnresolvedThreadsView mounts, calls useReviewThreads(prId)
  → WS inbound: { type: 'get_review_threads', pr_id }
  → Go: query review_threads WHERE pr_id = ?
  → returns rows with resolved/outdated/state_known
  → rendered as thread list with lifecycle badges
```

### Write path (resolve/unresolve)

```
User presses 'r' on focused thread (or clicks Resolve button)
  → WS inbound: { type: 'action', action: 'resolve_thread', pr_id, thread_id }
  → Go ws.Dispatcher routes to ApplyAction handler
  → orchestrator.ApplyAction(ctx, &ActionRequest{Action: ResolveThreadCommand, PRID: prId, Body: threadID})
  → if rate-limited: Enqueue → queue persists to SQLite → return "queued" result
  → if online: executor.Execute → resolveThreadHandler.Handle
      → type-assert provider to ThreadResolver
      → if missing: permanent error → ActionResult{Success:false, Error:"provider does not support thread resolution"}
      → if present: graphql.ResolveThread(ctx, threadID)
          → on success: emitter.Emit(thread_resolved event)
                        store.SetThreadResolved(ctx, threadID, true)
          → on transient error: retry (executor.ExecuteWithRetry)
          → on permanent error (404): return failed result
  → WS broadcasts THREAD_RESOLVED{pr_id, thread_id}
  → renderer: handleThreadResolved → queryClient.invalidateQueries(['review_threads', prId])
  → UnresolvedThreadsView and MergeReadinessBadge re-render
```

### Queue replay path

Identical to existing approve/post_comment replay: on startup `orchestrator.ReplayQueue` calls `executor.Execute` for each pending `resolve_thread` / `unresolve_thread` action. Same retry / MarkFailed / MarkCompleted logic applies.

---

## Error handling

| Scenario | Behaviour |
|---|---|
| Provider lacks `ThreadResolver` | Handler returns permanent error; queue marks action `failed`; UI toast: "Thread resolution not supported by this provider" |
| GraphQL call fails (network) | Transient error; retried up to `maxAttempts`; UI shows pending indicator |
| GraphQL 404 (thread deleted) | Permanent error; queue marks `failed`; next sync will remove the thread row |
| `review_threads` row missing for a PR | `useReviewThreads` returns empty array; view renders "No threads found"; no crash |
| `state_known = 0` | Badge shows "Unknown"; resolve button absent; merge-readiness badge silent |
| `SetThreadResolved` DB error | Logged as Warn; non-fatal; next sync corrects cache |
| WS event arrives for unknown `pr_id` | `queryClient.invalidateQueries` is a no-op on unknown keys; no crash |

---

## Testing

### Go

| Test file | What to test |
|---|---|
| `go/internal/actions/handlers_test.go` | `resolveThreadHandler.Handle`: success, provider missing (permanent error), GraphQL 404 (permanent), network error (transient) |
| `go/internal/actions/executor_test.go` | `handlerFor` dispatches new commands; `emitEvent` emits `thread_resolved` / `thread_reopened` |
| `go/internal/provider/github/graphql_test.go` | `ResolveThread` / `UnresolveThread` against recorded githubv4 fixtures (httptest) |
| `go/internal/store/review_thread_store_test.go` | `SetThreadResolved` flips `resolved` flag; non-existent ID is a no-op (no error) |
| `go/internal/sync/orchestrator_test.go` | `ApplyAction` with `ResolveThreadCommand`: success path calls `SetThreadResolved` + emits event; rate-limited path enqueues |

Verify commands:
```bash
cd go && go vet ./... && go test -race ./internal/actions/... ./internal/provider/github/... ./internal/store/... ./internal/sync/...
```

### TypeScript

| Test file | What to test |
|---|---|
| `electron/tests/unit/components/UnresolvedThreadsView.test.tsx` | Renders Open/Resolved/Outdated/Unknown rows correctly; resolve button absent when state_known=false; button dispatches WS message |
| `electron/tests/unit/components/MergeReadinessBadge.test.tsx` | Count=0 → green/ready; count>0 → amber; all state_known=false → renders nothing |
| `electron/tests/unit/hooks/useWebSocket.test.ts` | `THREAD_RESOLVED` / `THREAD_REOPENED` calls `queryClient.invalidateQueries` |
| `electron/tests/unit/utils/keybindings.test.ts` | `gu` sequence sets mode to `'threads'`; Escape from `'threads'` returns to `'detail'` |

Verify commands:
```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

---

## Acceptance criteria

- [ ] `gu` in detail mode opens the threads overlay; Escape returns to detail.
- [ ] Threads are listed with correct Open / Resolved / Outdated / Unknown lifecycle badge.
- [ ] Open thread count shown in footer badge; 0 = green, N > 0 = amber.
- [ ] `r` on an open thread sends resolve mutation; thread badge flips to Resolved without full sync.
- [ ] `u` on a resolved thread sends unresolve mutation; thread badge flips to Open.
- [ ] When provider has no `ThreadResolver`: resolve/unresolve button absent, badge shows "N open (resolution unavailable)".
- [ ] When `state_known = 0`: lifecycle shown as "Unknown", resolve/unresolve absent.
- [ ] While rate-limited, resolve/unresolve is queued; replays on next startup.
- [ ] `THREAD_RESOLVED` / `THREAD_REOPENED` WS events arrive and update the view without a full sync.
- [ ] All Go tests pass with `-race`; no new data races.
- [ ] All TS tests pass; `pnpm type-check` clean.

---

## Out of scope

- Comment anchoring across rebase (Phase 2.3).
- Patchset timeline + since-last-review (Phase 2.1 + 2.2).
- Pure-Go range-diff engine (Stage 2 / S2.2).
- Server-based push capture (Stage 2 / S2.1).
- Reviewer-request write-back (Phase 1.3, separate slice).
- Bulk resolve-all action.
- Thread filtering by author or file.
- Inline diff comment threads wired to the diff view (diff view shows threads in context — that is a future enhancement on top of this foundation).
