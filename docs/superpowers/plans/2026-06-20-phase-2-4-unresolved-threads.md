# Phase 2.4 — Unresolved-threads view + resolve/unresolve write-back + resolved/reopened event

> **REQUIRED SUB-SKILL:** Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Spec:** `docs/superpowers/specs/2026-06-20-phase-2-4-unresolved-threads-design.md`
>
> **Blocked on:** Phase 2.0 Foundation (migration 012, `review_threads` table, `ThreadProvider` capability, `github/graphql.go` githubv4 client) must be merged first.

---

## Goal

Add the `gu` unresolved-threads view, resolve/unresolve GraphQL mutations through the offline action queue, and the `THREAD_RESOLVED` / `THREAD_REOPENED` WS events so the UI updates without a full sync cycle.

## Architecture

- Go module: `github.com/itterno/bistro` (read from `go/go.mod` — do not assume)
- Go 1.26, SQLite (`_foreign_keys=on`), `shurcooL/githubv4`, gorilla/websocket
- Electron + React 19 + TypeScript, TanStack Store + Query, neverthrow, Vitest
- **No new migration.** `review_threads` is created by Foundation migration 012.
- Error handling: Go `fmt.Errorf("%w", err)`, TS `neverthrow` Result/ResultAsync — never `try/catch` in committed code.

## Tech stack reference files

- `go/go.mod` — authoritative module path + versions
- `go/internal/actions/types.go` — `ActionCommand`, `ActionRequest`, `ActionResult`
- `go/internal/actions/executor.go` — `handlerFor`, `emitEvent`, `ExecuteWithRetry`
- `go/internal/actions/handlers.go` — `Handler` interface, existing handler pattern
- `go/internal/store/action_queue.go` — `Enqueue`, `GetPending`, `MarkCompleted`, `MarkFailed`
- `go/internal/sync/orchestrator.go` — `ApplyAction`, `EventEmitter`, `emitter.Emit`
- `go/internal/sync/events.go` — `Event{Type, Data, Timestamp}`, `EventEmitter`
- `go/internal/provider/interface.go` — `Provider`, `ThreadProvider` (added in Foundation)
- `go/internal/provider/github/graphql.go` — `GraphQLClient` (added in Foundation)
- `go/internal/ws/server.go` — event-type → WS message-type mapping convention
- `electron/src/renderer/hooks/useWebSocket.ts` — `onMessage` switch, handler pattern
- `electron/src/renderer/store/ui.ts` — `UIState`, `AppMode`, `createUIStore`
- `electron/src/renderer/utils/keybindings.ts` — `KeyBinding`, `detailBindings`, `getEffectiveBindings`
- `electron/src/renderer/pages/PRDetailPage.tsx` — layout, footer bar

## File structure (all paths relative to repo root)

```
go/internal/provider/interface.go                        MODIFY
go/internal/provider/github/graphql.go                   MODIFY
go/internal/actions/types.go                             MODIFY
go/internal/actions/handlers.go                          MODIFY
go/internal/actions/executor.go                          MODIFY
go/internal/store/review_thread_store.go                 NEW
go/internal/sync/orchestrator.go                         MODIFY
electron/src/renderer/store/ui.ts                        MODIFY
electron/src/renderer/hooks/useWebSocket.ts              MODIFY
electron/src/renderer/utils/keybindings.ts               MODIFY
electron/src/renderer/components/UnresolvedThreadsView.tsx  NEW
electron/src/renderer/components/MergeReadinessBadge.tsx    NEW
electron/src/renderer/pages/PRDetailPage.tsx             MODIFY

Test files:
go/internal/actions/handlers_test.go                     MODIFY
go/internal/actions/executor_test.go                     MODIFY
go/internal/provider/github/graphql_test.go              MODIFY
go/internal/store/review_thread_store_test.go            NEW
go/internal/sync/orchestrator_test.go                    MODIFY
electron/tests/unit/components/UnresolvedThreadsView.test.tsx  NEW
electron/tests/unit/components/MergeReadinessBadge.test.tsx    NEW
electron/tests/unit/hooks/useWebSocket.test.ts           MODIFY
electron/tests/unit/utils/keybindings.test.ts            MODIFY
```

---

## Tasks

### Task 1 — `ThreadResolver` capability interface in `provider/interface.go`

**File:** `go/internal/provider/interface.go`

- [ ] **Step 1: Write failing test**

Create `go/internal/provider/interface_resolver_test.go`:
```go
package provider_test

import (
    "testing"
    "github.com/itterno/bistro/internal/provider"
)

// Verify the ThreadResolver interface is defined with the correct method set.
// This is a compile-time test — it fails if the interface doesn't exist.
type mockResolver struct{}

func (m *mockResolver) ResolveThread(_ context.Context, _ string) error   { return nil }
func (m *mockResolver) UnresolveThread(_ context.Context, _ string) error { return nil }

var _ provider.ThreadResolver = (*mockResolver)(nil)

func TestThreadResolverInterface(t *testing.T) {
    // Capability discovery via type assertion
    var p provider.Provider = &mockProvider{}
    _, ok := p.(provider.ThreadResolver)
    if ok {
        t.Error("base mock should NOT implement ThreadResolver")
    }
    var r provider.ThreadResolver = &mockResolver{}
    if r == nil {
        t.Error("ThreadResolver should be assignable")
    }
}
```

- [ ] **Step 2: Run test — expect FAIL** (interface not defined yet)
```bash
cd go && go test ./internal/provider/... 2>&1 | tail -5
```

- [ ] **Step 3: Add `ThreadResolver` interface to `interface.go`**

After the existing `ThreadProvider` interface block, add:
```go
// ThreadResolver is an optional capability for providers that support
// resolve/unresolve mutations. Discovered by type assertion at call sites.
// REST-only providers that do not implement this degrade gracefully.
type ThreadResolver interface {
    ResolveThread(ctx context.Context, threadID string) error
    UnresolveThread(ctx context.Context, threadID string) error
}
```

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test ./internal/provider/... && go vet ./internal/provider/...
```

- [ ] **Step 5: Commit**
```
feat(provider): add ThreadResolver optional capability interface
```

---

### Task 2 — GraphQL mutation methods on `github/graphql.go`

**File:** `go/internal/provider/github/graphql.go`

- [ ] **Step 1: Write failing test**

In `go/internal/provider/github/graphql_test.go`, add:
```go
func TestGraphQLClientResolveThread(t *testing.T) {
    // Use httptest to serve a minimal GraphQL response
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"data":{"resolveReviewThread":{"thread":{"id":"T1"}}}}`))
    }))
    defer srv.Close()

    client := newTestGraphQLClient(t, srv.URL) // helper that builds a githubv4 client pointed at srv
    err := client.ResolveThread(context.Background(), "T1")
    if err != nil {
        t.Fatalf("expected no error, got: %v", err)
    }
}

func TestGraphQLClientUnresolveThread(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"data":{"unresolveReviewThread":{"thread":{"id":"T1"}}}}`))
    }))
    defer srv.Close()

    client := newTestGraphQLClient(t, srv.URL)
    err := client.UnresolveThread(context.Background(), "T1")
    if err != nil {
        t.Fatalf("expected no error, got: %v", err)
    }
}

func TestGraphQLClientResolveThreadError(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"errors":[{"message":"Could not resolve to a ReviewThread"}]}`))
    }))
    defer srv.Close()

    client := newTestGraphQLClient(t, srv.URL)
    err := client.ResolveThread(context.Background(), "MISSING")
    if err == nil {
        t.Fatal("expected error for GraphQL errors response")
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd go && go test ./internal/provider/github/... 2>&1 | tail -10
```

- [ ] **Step 3: Implement `ResolveThread` and `UnresolveThread` on `GraphQLClient`**

In `go/internal/provider/github/graphql.go`, add after the existing methods:
```go
// ResolveThread executes the resolveReviewThread GraphQL mutation.
// threadID must be the global node ID (e.g. "PRRT_kwDO...").
func (g *GraphQLClient) ResolveThread(ctx context.Context, threadID string) error {
    var m struct {
        ResolveReviewThread struct {
            Thread struct{ ID githubv4.ID }
        } `graphql:"resolveReviewThread(input: $input)"`
    }
    input := githubv4.ResolveReviewThreadInput{ThreadID: githubv4.ID(threadID)}
    if err := g.client.Mutate(ctx, &m, map[string]interface{}{"input": input}); err != nil {
        return fmt.Errorf("resolveReviewThread: %w", err)
    }
    return nil
}

// UnresolveThread executes the unresolveReviewThread GraphQL mutation.
func (g *GraphQLClient) UnresolveThread(ctx context.Context, threadID string) error {
    var m struct {
        UnresolveReviewThread struct {
            Thread struct{ ID githubv4.ID }
        } `graphql:"unresolveReviewThread(input: $input)"`
    }
    input := githubv4.UnresolveReviewThreadInput{ThreadID: githubv4.ID(threadID)}
    if err := g.client.Mutate(ctx, &m, map[string]interface{}{"input": input}); err != nil {
        return fmt.Errorf("unresolveReviewThread: %w", err)
    }
    return nil
}
```

Add compile-time assertion in `graphql.go`:
```go
var _ provider.ThreadResolver = (*GraphQLClient)(nil)
```

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test -race ./internal/provider/github/... && go vet ./internal/provider/github/...
```

- [ ] **Step 5: Commit**
```
feat(github): add ResolveThread/UnresolveThread GraphQL mutations
```

---

### Task 3 — New ActionCommands and payload type in `actions/types.go`

**File:** `go/internal/actions/types.go`

- [ ] **Step 1: Write failing test**

In `go/internal/actions/types_test.go`, add:
```go
func TestResolveThreadCommandValidation(t *testing.T) {
    cases := []struct {
        name    string
        req     ActionRequest
        wantErr bool
    }{
        {
            name:    "valid resolve",
            req:     ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"},
            wantErr: false,
        },
        {
            name:    "missing thread id (body)",
            req:     ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: ""},
            wantErr: true,
        },
        {
            name:    "missing pr id",
            req:     ActionRequest{PRID: "", Action: ResolveThreadCommand, Body: "PRRT_abc"},
            wantErr: true,
        },
        {
            name:    "valid unresolve",
            req:     ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: "PRRT_abc"},
            wantErr: false,
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            err := tc.req.Validate()
            if (err != nil) != tc.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
            }
        })
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd go && go test ./internal/actions/... 2>&1 | tail -10
```

- [ ] **Step 3: Add constants, payload type, and Validate cases**

In `go/internal/actions/types.go`:
```go
const (
    // existing constants preserved ...
    ResolveThreadCommand   ActionCommand = "resolve_thread"
    UnresolveThreadCommand ActionCommand = "unresolve_thread"
)

// ResolveThreadPayload is JSON-serialised into ActionQueue.payload for
// resolve_thread / unresolve_thread actions.
type ResolveThreadPayload struct {
    ThreadID string `json:"thread_id"`
    PRID     string `json:"pr_id"`
}
```

In `ActionRequest.Validate()` switch, add:
```go
case ResolveThreadCommand, UnresolveThreadCommand:
    if ar.Body == "" {
        return fmt.Errorf("Body (thread node ID) is required for %s", ar.Action)
    }
    return nil
```

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test -race ./internal/actions/... && go vet ./internal/actions/...
```

- [ ] **Step 5: Commit**
```
feat(actions): add ResolveThreadCommand/UnresolveThreadCommand action types
```

---

### Task 4 — Action handlers for resolve/unresolve in `actions/handlers.go`

**File:** `go/internal/actions/handlers.go`

- [ ] **Step 1: Write failing test**

In `go/internal/actions/handlers_test.go`, add:

```go
// mockThreadResolver implements provider.ThreadResolver for tests.
type mockThreadResolver struct {
    resolveErr   error
    unresolveErr error
    resolved     []string
    unresolved   []string
}

func (m *mockThreadResolver) ResolveThread(_ context.Context, id string) error {
    if m.resolveErr != nil {
        return m.resolveErr
    }
    m.resolved = append(m.resolved, id)
    return nil
}

func (m *mockThreadResolver) UnresolveThread(_ context.Context, id string) error {
    if m.unresolveErr != nil {
        return m.unresolveErr
    }
    m.unresolved = append(m.unresolved, id)
    return nil
}

func TestResolveThreadHandler_Success(t *testing.T) {
    resolver := &mockThreadResolver{}
    // mockProvider that also implements ThreadResolver
    prov := &mockProviderWithResolver{resolver: resolver}
    h := &resolveThreadHandler{provider: prov}
    req := &ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"}
    result, err := h.Handle(context.Background(), req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !result.Success {
        t.Errorf("expected Success=true, got: %v", result.Error)
    }
    if len(resolver.resolved) != 1 || resolver.resolved[0] != "PRRT_abc" {
        t.Errorf("expected ResolveThread called with PRRT_abc, got %v", resolver.resolved)
    }
}

func TestResolveThreadHandler_ProviderMissing(t *testing.T) {
    // provider does NOT implement ThreadResolver
    prov := &mockProvider{} // existing mock without ThreadResolver
    h := &resolveThreadHandler{provider: prov}
    req := &ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"}
    result, err := h.Handle(context.Background(), req)
    if err == nil {
        t.Fatal("expected permanent error when provider lacks ThreadResolver")
    }
    if result != nil && result.Success {
        t.Error("expected success=false")
    }
    // Must be a permanent error (not retried)
    if !isPermanentError(err) {
        t.Errorf("expected permanent error, got: %v", err)
    }
}

func TestResolveThreadHandler_TransientError(t *testing.T) {
    resolver := &mockThreadResolver{resolveErr: errors.New("network timeout")}
    prov := &mockProviderWithResolver{resolver: resolver}
    h := &resolveThreadHandler{provider: prov}
    req := &ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"}
    _, err := h.Handle(context.Background(), req)
    if err == nil {
        t.Fatal("expected transient error")
    }
    if isPermanentError(err) {
        t.Error("network timeout should be transient, not permanent")
    }
}

func TestResolveThreadHandler_PermanentError(t *testing.T) {
    resolver := &mockThreadResolver{resolveErr: errors.New("Could not resolve to a ReviewThread")}
    prov := &mockProviderWithResolver{resolver: resolver}
    h := &resolveThreadHandler{provider: prov}
    req := &ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "DELETED"}
    _, err := h.Handle(context.Background(), req)
    if err == nil {
        t.Fatal("expected permanent error for not-found")
    }
    if !isPermanentError(err) {
        t.Errorf("not-found should be permanent, got: %v", err)
    }
}

func TestUnresolveThreadHandler_Success(t *testing.T) {
    resolver := &mockThreadResolver{}
    prov := &mockProviderWithResolver{resolver: resolver}
    h := &unresolveThreadHandler{provider: prov}
    req := &ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: "PRRT_abc"}
    result, err := h.Handle(context.Background(), req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !result.Success {
        t.Errorf("expected Success=true, got: %v", result.Error)
    }
    if len(resolver.unresolved) != 1 {
        t.Errorf("expected UnresolveThread called once, got %v", resolver.unresolved)
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd go && go test ./internal/actions/... 2>&1 | tail -10
```

- [ ] **Step 3: Implement handlers**

In `go/internal/actions/handlers.go`, add the two handlers and the `isPermanentError` helper:

```go
// permanentErrorKeywords are GraphQL error messages that indicate the action
// can never succeed and should not be retried.
var permanentErrorKeywords = []string{
    "Could not resolve to a ReviewThread",
    "not found",
    "404",
}

func isPermanentError(err error) bool {
    if err == nil {
        return false
    }
    msg := strings.ToLower(err.Error())
    for _, kw := range permanentErrorKeywords {
        if strings.Contains(msg, strings.ToLower(kw)) {
            return true
        }
    }
    return false
}

type resolveThreadHandler struct {
    provider provider.Provider
}

func (h *resolveThreadHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
    resolver, ok := h.provider.(provider.ThreadResolver)
    if !ok {
        return nil, &permanentError{msg: "provider does not support thread resolution"}
    }
    if err := resolver.ResolveThread(ctx, req.Body); err != nil {
        if isPermanentError(err) {
            return nil, &permanentError{msg: err.Error()}
        }
        return nil, fmt.Errorf("ResolveThread: %w", err)
    }
    return &ActionResult{PRID: req.PRID, Action: req.Action, Success: true}, nil
}

type unresolveThreadHandler struct {
    provider provider.Provider
}

func (h *unresolveThreadHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
    resolver, ok := h.provider.(provider.ThreadResolver)
    if !ok {
        return nil, &permanentError{msg: "provider does not support thread resolution"}
    }
    if err := resolver.UnresolveThread(ctx, req.Body); err != nil {
        if isPermanentError(err) {
            return nil, &permanentError{msg: err.Error()}
        }
        return nil, fmt.Errorf("UnresolveThread: %w", err)
    }
    return &ActionResult{PRID: req.PRID, Action: req.Action, Success: true}, nil
}
```

`permanentError` is an unexported struct wrapping a message string; `isPermanentError` checks for it via `errors.As` OR checks keyword list — use whichever pattern already exists in `handlers.go` for the `PermanentErrorKeywords` slice.

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test -race ./internal/actions/... && go vet ./internal/actions/...
```

- [ ] **Step 5: Commit**
```
feat(actions): add resolve/unresolve thread handlers
```

---

### Task 5 — Wire handlers into executor + emit events

**Files:** `go/internal/actions/executor.go`

- [ ] **Step 1: Write failing test**

In `go/internal/actions/executor_test.go`, add:
```go
func TestExecutorHandlerForResolveThread(t *testing.T) {
    emitter := events.NewEventEmitter()
    prov := &mockProvider{}
    ae := NewActionExecutor(slog.Default(), prov, nil, emitter)
    h, ok := ae.handlerFor(ResolveThreadCommand)
    if !ok {
        t.Fatal("handlerFor should return handler for ResolveThreadCommand")
    }
    if h == nil {
        t.Fatal("handler must not be nil")
    }
}

func TestExecutorEmitThreadResolved(t *testing.T) {
    emitter := events.NewEventEmitter()
    ch := emitter.Subscribe()
    defer emitter.Unsubscribe(ch)

    prov := &mockProvider{}
    ae := NewActionExecutor(slog.Default(), prov, nil, emitter)
    req := &ActionRequest{PRID: "pr1", Action: ResolveThreadCommand, Body: "PRRT_abc"}
    result := &ActionResult{PRID: "pr1", Action: ResolveThreadCommand, Success: true}
    ae.emitEvent(req, result)

    select {
    case evt := <-ch:
        if evt.Type != "thread_resolved" {
            t.Errorf("expected thread_resolved, got %s", evt.Type)
        }
        data, _ := evt.Data.(map[string]interface{})
        if data["thread_id"] != "PRRT_abc" {
            t.Errorf("expected thread_id PRRT_abc, got %v", data["thread_id"])
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("timed out waiting for event")
    }
}

func TestExecutorEmitThreadReopened(t *testing.T) {
    emitter := events.NewEventEmitter()
    ch := emitter.Subscribe()
    defer emitter.Unsubscribe(ch)

    prov := &mockProvider{}
    ae := NewActionExecutor(slog.Default(), prov, nil, emitter)
    req := &ActionRequest{PRID: "pr1", Action: UnresolveThreadCommand, Body: "PRRT_abc"}
    result := &ActionResult{PRID: "pr1", Action: UnresolveThreadCommand, Success: true}
    ae.emitEvent(req, result)

    select {
    case evt := <-ch:
        if evt.Type != "thread_reopened" {
            t.Errorf("expected thread_reopened, got %s", evt.Type)
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("timed out waiting for event")
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd go && go test ./internal/actions/... 2>&1 | tail -10
```

- [ ] **Step 3: Add cases to `handlerFor` and `emitEvent`**

In `executor.go` `handlerFor` switch:
```go
case ResolveThreadCommand:
    return &resolveThreadHandler{provider: ae.provider}, true
case UnresolveThreadCommand:
    return &unresolveThreadHandler{provider: ae.provider}, true
```

In `executor.go` `emitEvent` switch:
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

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test -race ./internal/actions/... && go vet ./internal/actions/...
```

- [ ] **Step 5: Commit**
```
feat(executor): wire resolve/unresolve thread commands and events
```

---

### Task 6 — `review_thread_store.go` — `SetThreadResolved`

**File:** `go/internal/store/review_thread_store.go` (new)

- [ ] **Step 1: Write failing test**

Create `go/internal/store/review_thread_store_test.go`:
```go
package store_test

import (
    "context"
    "testing"
    "github.com/itterno/bistro/internal/store"
)

func TestSetThreadResolved(t *testing.T) {
    db := openTestDB(t) // uses existing test helper that opens an in-memory DB + runs migrations
    ctx := context.Background()

    // Insert a PR and a review thread (migration 012 must be applied)
    _, err := db.SQLDB().ExecContext(ctx,
        `INSERT INTO prs (id, title, state) VALUES ('pr1', 'Test PR', 'open')`)
    if err != nil {
        t.Fatalf("insert PR: %v", err)
    }
    _, err = db.SQLDB().ExecContext(ctx,
        `INSERT INTO review_threads (id, pr_id, resolved, outdated, state_known)
         VALUES ('T1', 'pr1', 0, 0, 1)`)
    if err != nil {
        t.Fatalf("insert thread: %v", err)
    }

    // Resolve it
    if err := db.SetThreadResolved(ctx, "T1", true); err != nil {
        t.Fatalf("SetThreadResolved(true): %v", err)
    }
    var resolved int
    if err := db.SQLDB().QueryRowContext(ctx,
        `SELECT resolved FROM review_threads WHERE id = 'T1'`).Scan(&resolved); err != nil {
        t.Fatalf("scan: %v", err)
    }
    if resolved != 1 {
        t.Errorf("expected resolved=1, got %d", resolved)
    }

    // Unresolve it
    if err := db.SetThreadResolved(ctx, "T1", false); err != nil {
        t.Fatalf("SetThreadResolved(false): %v", err)
    }
    if err := db.SQLDB().QueryRowContext(ctx,
        `SELECT resolved FROM review_threads WHERE id = 'T1'`).Scan(&resolved); err != nil {
        t.Fatalf("scan: %v", err)
    }
    if resolved != 0 {
        t.Errorf("expected resolved=0, got %d", resolved)
    }
}

func TestSetThreadResolvedNonExistent(t *testing.T) {
    db := openTestDB(t)
    ctx := context.Background()
    // Should be a no-op (0 rows affected), not an error
    if err := db.SetThreadResolved(ctx, "MISSING", true); err != nil {
        t.Fatalf("expected no error for non-existent thread, got: %v", err)
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd go && go test ./internal/store/... 2>&1 | tail -10
```

- [ ] **Step 3: Implement `review_thread_store.go`**

```go
package store

import (
    "context"
    "fmt"
)

// SetThreadResolved updates the resolved column of a review_thread row.
// It is called immediately after a successful resolve/unresolve mutation to
// keep the local cache consistent without waiting for the next sync cycle.
// A non-existent threadID is a no-op (0 rows affected) — not an error.
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

Note: `db.sqlDB` is the field name; verify against `go/internal/store/db.go` before implementing — do not assume. Use the same field access pattern as other methods in the store package.

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test -race ./internal/store/... && go vet ./internal/store/...
```

- [ ] **Step 5: Commit**
```
feat(store): add SetThreadResolved helper for local cache update
```

---

### Task 7 — Orchestrator: post-action local update + event emission

**File:** `go/internal/sync/orchestrator.go`

- [ ] **Step 1: Write failing test**

In `go/internal/sync/orchestrator_test.go`, add:
```go
func TestApplyActionResolveThreadSuccess(t *testing.T) {
    // Build orchestrator with a provider that implements ThreadResolver
    resolver := &mockThreadResolver{}
    prov := &mockProviderWithResolver{resolver: resolver}
    db := openTestDB(t)
    emitter := events.NewEventEmitter()
    ch := emitter.Subscribe()
    defer emitter.Unsubscribe(ch)

    o := NewOrchestrator(slog.Default(), prov, db, nil)
    o.SetEmitter(emitter) // add a test-only setter or expose via constructor option

    req := &actions.ActionRequest{
        PRID:   "pr1",
        Action: actions.ResolveThreadCommand,
        Body:   "PRRT_abc",
    }
    result := o.ApplyAction(context.Background(), req)
    if !result.Success {
        t.Fatalf("expected success, got: %v", result.Error)
    }

    // Verify event emitted
    select {
    case evt := <-ch:
        if evt.Type != "thread_resolved" {
            t.Errorf("expected thread_resolved event, got %s", evt.Type)
        }
    case <-time.After(200 * time.Millisecond):
        t.Fatal("timed out waiting for thread_resolved event")
    }
}

func TestApplyActionResolveThreadQueued(t *testing.T) {
    // Simulate rate-limited state → action must be enqueued
    prov := &mockProviderWithResolver{resolver: &mockThreadResolver{}}
    db := openTestDB(t)
    queue := store.NewActionQueue(db.SQLDB())

    o := NewOrchestrator(slog.Default(), prov, db, nil)
    o.SetQueue(queue) // expose queue injection for tests (already done in existing code)
    o.SetRateLimited(time.Now().Add(10 * time.Second)) // helper to inject rate limit state

    req := &actions.ActionRequest{
        PRID:   "pr1",
        Action: actions.ResolveThreadCommand,
        Body:   "PRRT_abc",
    }
    result := o.ApplyAction(context.Background(), req)
    if result.Success {
        t.Error("should not succeed when rate limited")
    }
    if result.Error == "" {
        t.Error("should have an error message (rate limited)")
    }

    // Verify queued
    pending, err := queue.GetPending(context.Background(), 10)
    if err != nil {
        t.Fatalf("GetPending: %v", err)
    }
    if len(pending) != 1 || pending[0].ActionType != string(actions.ResolveThreadCommand) {
        t.Errorf("expected 1 resolve_thread queued action, got %v", pending)
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd go && go test ./internal/sync/... 2>&1 | tail -10
```

- [ ] **Step 3: Add post-action hook in `ApplyAction`**

In `orchestrator.go`, after `executor.Execute` succeeds (the `result.Success` branch), add:
```go
if result.Success {
    switch req.Action {
    case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
        resolved := req.Action == actions.ResolveThreadCommand
        if err := o.store.SetThreadResolved(ctx, req.Body, resolved); err != nil {
            o.log.Warn("failed to update review_threads after resolve mutation",
                "thread_id", req.Body,
                "resolved", resolved,
                "error", err,
            )
            // Non-fatal: next full sync will correct the cache.
        }
    }
}
```

No additional event emit needed here — `executor.emitEvent` already emits `thread_resolved` / `thread_reopened`. The EventEmitter → WS broadcast path already exists.

- [ ] **Step 4: Run test — expect PASS**
```bash
cd go && go test -race ./internal/sync/... && go vet ./internal/sync/...
```

- [ ] **Step 5: Commit**
```
feat(orchestrator): update review_threads cache after resolve/unresolve mutation
```

---

### Task 8 — WS server: map Go event types to TS message types

**File:** `go/internal/ws/server.go`

- [ ] **Step 1: Check existing mapping**

Read `go/internal/ws/server.go`. Confirm whether there is a `toWSType` conversion function or an explicit map that translates `pr_approved` → `PR_APPROVED`, etc.

- [ ] **Step 2: Add mapping entries**

Following the existing pattern (e.g. in the event subscriber goroutine or conversion map), add:
```go
"thread_resolved": "THREAD_RESOLVED",
"thread_reopened": "THREAD_REOPENED",
```

- [ ] **Step 3: Verify with existing server tests**
```bash
cd go && go test -race ./internal/ws/... && go vet ./internal/ws/...
```

- [ ] **Step 4: Commit**
```
feat(ws): map thread_resolved/thread_reopened events to WS message types
```

---

### Task 9 — Renderer: extend `UIState` with `'threads'` mode

**File:** `electron/src/renderer/store/ui.ts`

- [ ] **Step 1: Write failing test**

In `electron/tests/unit/utils/keybindings.test.ts`, add:
```ts
it('gu sequence in detail mode sets mode to threads', () => {
  const store = createTestStore({ mode: 'detail', selectedPRId: 'pr1' });
  // simulate gu — call the binding action directly
  const binding = getBinding('detail.threads');
  binding.action(store);
  expect(store.state.mode).toBe('threads');
  expect(store.state.threadsViewPRId).toBe('pr1');
});

it('Escape from threads mode returns to detail', () => {
  const store = createTestStore({ mode: 'threads', threadsViewPRId: 'pr1' });
  const binding = getBinding('global.escape');
  binding.action(store);
  expect(store.state.mode).toBe('detail');
  expect(store.state.threadsViewPRId).toBeUndefined();
});
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd electron && pnpm test -- --testPathPattern="keybindings" 2>&1 | tail -15
```

- [ ] **Step 3: Add `'threads'` to UIState mode union and `threadsViewPRId` field**

In `ui.ts`:
```ts
// In the UIState interface / type:
mode: 'list' | 'detail' | 'diff' | 'help' | 'search' | 'settings' | 'threads';
threadsViewPRId?: string;
```

In `initialUIState`:
```ts
threadsViewPRId: undefined,
```

- [ ] **Step 4: Run type-check + test**
```bash
cd electron && pnpm type-check && pnpm test -- --testPathPattern="keybindings" 2>&1 | tail -10
```

- [ ] **Step 5: Commit**
```
feat(ui): add threads mode and threadsViewPRId to UIState
```

---

### Task 10 — Keybindings: `gu`, threads mode bindings, Escape

**File:** `electron/src/renderer/utils/keybindings.ts`

- [ ] **Step 1: Tests already written in Task 9 — run them first (will still fail)**
```bash
cd electron && pnpm test -- --testPathPattern="keybindings" 2>&1 | tail -10
```

- [ ] **Step 2: Add `detail.threads` binding to `detailBindings`**

```ts
{
  actionId: 'detail.threads',
  keys: ['g', 'u'],                // Vim-style two-key sequence
  description: 'Unresolved threads',
  modes: ['detail'],
  action: (store) => {
    const prId = store.state.selectedPRId;  // confirm field name in ui.ts
    store.setState((s) => ({ ...s, mode: 'threads', threadsViewPRId: prId }));
  },
},
```

- [ ] **Step 3: Add `threadsBindings` array**

```ts
// ─── Threads view bindings (active in threads mode only) ─────────────────────

const threadsBindings: KeyBinding[] = [
  {
    actionId: 'threads.resolve',
    keys: 'r',
    description: 'Resolve focused thread',
    modes: ['threads'],
    action: (_store) => {
      // Dispatched to UnresolvedThreadsView via a custom event or shared state.
      // The component listens for this and sends the WS resolve action.
      document.dispatchEvent(new CustomEvent('bistro:threads:resolve'));
    },
  },
  {
    actionId: 'threads.unresolve',
    keys: 'u',
    description: 'Unresolve focused thread',
    modes: ['threads'],
    action: (_store) => {
      document.dispatchEvent(new CustomEvent('bistro:threads:unresolve'));
    },
  },
];
```

- [ ] **Step 4: Extend Escape handler in `globalBindings`**

Find the `global.escape` binding action and add a branch:
```ts
} else if (mode === 'threads') {
  store.setState((s) => ({ ...s, mode: 'detail', threadsViewPRId: undefined }));
}
```

- [ ] **Step 5: Include `threadsBindings` in `getEffectiveBindings()`**

Find the array spread / concat in `getEffectiveBindings` and add `...threadsBindings`.

- [ ] **Step 6: Run test — expect PASS**
```bash
cd electron && pnpm type-check && pnpm test -- --testPathPattern="keybindings" 2>&1 | tail -10
```

- [ ] **Step 7: Commit**
```
feat(keybindings): add gu unresolved-threads binding and threads mode keys
```

---

### Task 11 — `useWebSocket.ts`: handle `THREAD_RESOLVED` / `THREAD_REOPENED`

**File:** `electron/src/renderer/hooks/useWebSocket.ts`

- [ ] **Step 1: Write failing test**

In `electron/tests/unit/hooks/useWebSocket.test.ts`, add:
```ts
it('THREAD_RESOLVED invalidates review_threads query cache', async () => {
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
  simulateMessage({ type: 'THREAD_RESOLVED', data: { pr_id: 'pr1', thread_id: 'T1' } });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['review_threads', 'pr1'] });
});

it('THREAD_REOPENED invalidates review_threads query cache', async () => {
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
  simulateMessage({ type: 'THREAD_REOPENED', data: { pr_id: 'pr1', thread_id: 'T1' } });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['review_threads', 'pr1'] });
});
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd electron && pnpm test -- --testPathPattern="useWebSocket" 2>&1 | tail -10
```

- [ ] **Step 3: Implement handlers in `useWebSocket.ts`**

Add interface:
```ts
interface ThreadStateChangedData {
  pr_id: string;
  thread_id: string;
}
```

Add handler functions:
```ts
function handleThreadResolved(data: ThreadStateChangedData): void {
  queryClient.invalidateQueries({ queryKey: ['review_threads', data.pr_id] });
}

function handleThreadReopened(data: ThreadStateChangedData): void {
  queryClient.invalidateQueries({ queryKey: ['review_threads', data.pr_id] });
}
```

In `onMessage` switch:
```ts
case 'THREAD_RESOLVED':
  handleThreadResolved(message.data as ThreadStateChangedData);
  break;
case 'THREAD_REOPENED':
  handleThreadReopened(message.data as ThreadStateChangedData);
  break;
```

- [ ] **Step 4: Run test — expect PASS**
```bash
cd electron && pnpm type-check && pnpm test -- --testPathPattern="useWebSocket" 2>&1 | tail -10
```

- [ ] **Step 5: Commit**
```
feat(useWebSocket): handle THREAD_RESOLVED/THREAD_REOPENED events
```

---

### Task 12 — `MergeReadinessBadge` component

**File:** `electron/src/renderer/components/MergeReadinessBadge.tsx` (new)

- [ ] **Step 1: Write failing test**

Create `electron/tests/unit/components/MergeReadinessBadge.test.tsx`:
```tsx
import { render, screen } from '@testing-library/react';
import { MergeReadinessBadge } from '../../../src/renderer/components/MergeReadinessBadge';
import { vi } from 'vitest';

// Mock useReviewThreads
vi.mock('../../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: vi.fn(),
}));
import { useReviewThreads } from '../../../src/renderer/hooks/useReviewThreads';

it('shows green ready badge when no open threads', () => {
  vi.mocked(useReviewThreads).mockReturnValue({
    data: [
      { id: 'T1', resolved: true, outdated: false, state_known: true },
    ],
    isLoading: false,
  });
  render(<MergeReadinessBadge prId="pr1" />);
  expect(screen.getByText(/ready/i)).toBeInTheDocument();
});

it('shows amber badge with count when open threads exist', () => {
  vi.mocked(useReviewThreads).mockReturnValue({
    data: [
      { id: 'T1', resolved: false, outdated: false, state_known: true },
      { id: 'T2', resolved: false, outdated: false, state_known: true },
    ],
    isLoading: false,
  });
  render(<MergeReadinessBadge prId="pr1" />);
  expect(screen.getByText(/2 open thread/i)).toBeInTheDocument();
});

it('renders nothing when all threads have state_known=false', () => {
  vi.mocked(useReviewThreads).mockReturnValue({
    data: [
      { id: 'T1', resolved: false, outdated: false, state_known: false },
    ],
    isLoading: false,
  });
  const { container } = render(<MergeReadinessBadge prId="pr1" />);
  expect(container.firstChild).toBeNull();
});

it('renders nothing while loading', () => {
  vi.mocked(useReviewThreads).mockReturnValue({ data: undefined, isLoading: true });
  const { container } = render(<MergeReadinessBadge prId="pr1" />);
  expect(container.firstChild).toBeNull();
});
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd electron && pnpm test -- --testPathPattern="MergeReadinessBadge" 2>&1 | tail -10
```

- [ ] **Step 3: Create `useReviewThreads` hook**

Create `electron/src/renderer/hooks/useReviewThreads.ts`:
```ts
import { useQuery } from '@tanstack/react-query';
import { useWS } from './useWebSocket';  // or however WS send is exposed

export interface ReviewThreadRow {
  id: string;
  resolved: boolean;
  outdated: boolean;
  state_known: boolean;
  file_path?: string;
  line?: number;
}

export function useReviewThreads(prId: string) {
  return useQuery({
    queryKey: ['review_threads', prId],
    queryFn: () => fetchReviewThreads(prId),
    enabled: Boolean(prId),
  });
}

// fetchReviewThreads sends a WS inbound message and waits for the response,
// or calls a REST endpoint — follow the pattern used by other queries in the
// renderer (e.g. usePRDetail). Never use try/catch; wrap errors in Result.
async function fetchReviewThreads(prId: string): Promise<ReviewThreadRow[]> {
  // Implementation follows the existing WS request/response pattern in the codebase.
  // If a REST GET /api/review-threads?pr_id= endpoint is added by Foundation,
  // use fetch() wrapped in ResultAsync. If not, use the WS request pattern.
  throw new Error('not implemented'); // placeholder; replaced in implementation
}
```

- [ ] **Step 4: Create `MergeReadinessBadge.tsx`**

```tsx
import type { ReviewThreadRow } from '../hooks/useReviewThreads';
import { useReviewThreads } from '../hooks/useReviewThreads';

interface MergeReadinessBadgeProps {
  prId: string;
}

export function MergeReadinessBadge({ prId }: MergeReadinessBadgeProps) {
  const { data: threads, isLoading } = useReviewThreads(prId);

  if (isLoading || !threads) return null;

  const knownThreads = threads.filter((t: ReviewThreadRow) => t.state_known);
  if (knownThreads.length === 0) return null;

  const openCount = knownThreads.filter(
    (t: ReviewThreadRow) => !t.resolved && !t.outdated,
  ).length;

  if (openCount === 0) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-success" data-testid="merge-readiness-badge">
        <span className="w-2 h-2 rounded-full bg-success" aria-hidden="true" />
        Ready
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1 text-xs text-warning" data-testid="merge-readiness-badge">
      <span className="w-2 h-2 rounded-full bg-warning" aria-hidden="true" />
      {openCount} open thread{openCount !== 1 ? 's' : ''}
    </span>
  );
}
```

- [ ] **Step 5: Run test — expect PASS**
```bash
cd electron && pnpm type-check && pnpm test -- --testPathPattern="MergeReadinessBadge" 2>&1 | tail -10
```

- [ ] **Step 6: Commit**
```
feat(components): add MergeReadinessBadge showing open thread count
```

---

### Task 13 — `UnresolvedThreadsView` component

**File:** `electron/src/renderer/components/UnresolvedThreadsView.tsx` (new)

- [ ] **Step 1: Write failing test**

Create `electron/tests/unit/components/UnresolvedThreadsView.test.tsx`:
```tsx
import { render, screen, fireEvent } from '@testing-library/react';
import { UnresolvedThreadsView } from '../../../src/renderer/components/UnresolvedThreadsView';
import { vi } from 'vitest';

vi.mock('../../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: vi.fn(),
}));
import { useReviewThreads } from '../../../src/renderer/hooks/useReviewThreads';

const openThread = {
  id: 'T1', resolved: false, outdated: false, state_known: true,
  file_path: 'src/foo.ts', line: 42,
  comments: [{ author: 'alice', body: 'Please fix this.' }],
};
const resolvedThread = {
  id: 'T2', resolved: true, outdated: false, state_known: true,
  file_path: 'src/bar.ts', line: 10,
  comments: [{ author: 'bob', body: 'Looks good.' }],
};
const unknownThread = {
  id: 'T3', resolved: false, outdated: false, state_known: false,
  file_path: 'src/baz.ts', line: 5,
  comments: [{ author: 'carol', body: 'Needs work.' }],
};

it('renders Open badge for open thread', () => {
  vi.mocked(useReviewThreads).mockReturnValue({ data: [openThread], isLoading: false });
  render(<UnresolvedThreadsView prId="pr1" />);
  expect(screen.getByText('Open')).toBeInTheDocument();
  expect(screen.getByText(/Please fix this/)).toBeInTheDocument();
});

it('renders Resolved badge for resolved thread', () => {
  vi.mocked(useReviewThreads).mockReturnValue({ data: [resolvedThread], isLoading: false });
  render(<UnresolvedThreadsView prId="pr1" />);
  expect(screen.getByText('Resolved')).toBeInTheDocument();
});

it('renders Unknown badge and no resolve button when state_known=false', () => {
  vi.mocked(useReviewThreads).mockReturnValue({ data: [unknownThread], isLoading: false });
  render(<UnresolvedThreadsView prId="pr1" />);
  expect(screen.getByText('Unknown')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /resolve/i })).not.toBeInTheDocument();
});

it('shows Resolve button for open thread and dispatches WS action on click', () => {
  const sendAction = vi.fn();
  vi.mocked(useReviewThreads).mockReturnValue({ data: [openThread], isLoading: false });
  render(<UnresolvedThreadsView prId="pr1" sendAction={sendAction} />);
  const btn = screen.getByRole('button', { name: /resolve/i });
  fireEvent.click(btn);
  expect(sendAction).toHaveBeenCalledWith(
    expect.objectContaining({ action: 'resolve_thread', thread_id: 'T1' }),
  );
});

it('shows Unresolve button for resolved thread', () => {
  const sendAction = vi.fn();
  vi.mocked(useReviewThreads).mockReturnValue({ data: [resolvedThread], isLoading: false });
  render(<UnresolvedThreadsView prId="pr1" sendAction={sendAction} />);
  const btn = screen.getByRole('button', { name: /unresolve/i });
  fireEvent.click(btn);
  expect(sendAction).toHaveBeenCalledWith(
    expect.objectContaining({ action: 'unresolve_thread', thread_id: 'T2' }),
  );
});
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd electron && pnpm test -- --testPathPattern="UnresolvedThreadsView" 2>&1 | tail -10
```

- [ ] **Step 3: Implement `UnresolvedThreadsView.tsx`**

Key behaviour:
- `useReviewThreads(prId)` for data.
- Sort order: Open → Outdated → Resolved (by lifecycle, then by file/line).
- Lifecycle badge: `state_known=false` → "Unknown" (grey); `outdated=true` → "Outdated" (dim); `resolved=true` → "Resolved" (green); else → "Open" (amber).
- Resolve button present when: `state_known=true && !resolved && !outdated`.
- Unresolve button present when: `state_known=true && resolved`.
- Both buttons absent when `state_known=false`.
- `sendAction` prop is called with `{ action: 'resolve_thread' | 'unresolve_thread', pr_id: prId, thread_id: thread.id }`.
- Comment body truncated to 120 characters with `…`.
- Empty state: "No review threads found for this PR."

- [ ] **Step 4: Run test — expect PASS**
```bash
cd electron && pnpm type-check && pnpm test -- --testPathPattern="UnresolvedThreadsView" 2>&1 | tail -10
```

- [ ] **Step 5: Commit**
```
feat(components): add UnresolvedThreadsView with lifecycle badges and resolve actions
```

---

### Task 14 — Wire `UnresolvedThreadsView` + `MergeReadinessBadge` into `PRDetailPage`

**File:** `electron/src/renderer/pages/PRDetailPage.tsx`

- [ ] **Step 1: Read `PRDetailPage.tsx` to understand the layout (footer bar + mode guards)**

- [ ] **Step 2: Add `MergeReadinessBadge` to the footer bar**

Import and render in the footer:
```tsx
import { MergeReadinessBadge } from '../components/MergeReadinessBadge';

// In the footer section:
<MergeReadinessBadge prId={pr.id} />
```

- [ ] **Step 3: Add `UnresolvedThreadsView` overlay for `'threads'` mode**

Import and render conditionally:
```tsx
import { UnresolvedThreadsView } from '../components/UnresolvedThreadsView';

// Before or after the existing detail content, guarded by mode:
{mode === 'threads' && threadsViewPRId && (
  <div className="absolute inset-0 z-10 bg-surface overflow-y-auto">
    <UnresolvedThreadsView prId={threadsViewPRId} sendAction={sendWSAction} />
  </div>
)}
```

`mode` and `threadsViewPRId` are read from `useStore(appStore, s => s)` (follow existing pattern). `sendWSAction` follows the existing WS send pattern in the page.

- [ ] **Step 4: Run build + test**
```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

- [ ] **Step 5: Commit**
```
feat(PRDetailPage): add threads overlay (gu) and merge-readiness badge
```

---

### Task 15 — Final integration verification

- [ ] **Go full suite**
```bash
cd go && go vet ./... && go test -race ./...
```
Expected: all tests pass, 0 data races.

- [ ] **Electron full suite**
```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```
Expected: all tests pass, no type errors, no lint errors.

- [ ] **Manual smoke test (optional but recommended)**
```bash
# Terminal 1
cd go && go run cmd/bistro-sync/main.go --log-level debug

# Terminal 2
cd electron && pnpm start
# 1. Open a PR with review threads.
# 2. Press gu — verify threads view opens with correct lifecycle badges.
# 3. Press r on an open thread — verify it resolves without full sync.
# 4. Verify merge-readiness badge in footer updates.
# 5. Press Escape — verify return to detail mode.
```

- [ ] **Final commit message if needed (clean-up / fixups)**
```
chore(phase-2.4): final cleanup and integration fixes
```

---

## Verify commands (summary)

```bash
# Go
cd go && go vet ./... && go test -race ./internal/actions/... ./internal/provider/github/... ./internal/store/... ./internal/sync/... ./internal/ws/...

# Electron
cd electron && pnpm type-check && pnpm lint && pnpm test
```
