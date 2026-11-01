# Tasks 1–4: Go foundation and first services

Read first: the plan [README](README.md) (Global Constraints, decisions, Service port recipe) and [inventory-engine.md](inventory-engine.md).

### Task 1: Move the Go module to the repo root and rename `server` → `engine`

**Files:**
- Move: `go/go.mod`, `go/go.sum`, `go/sqlc.yaml`, `go/internal/`, `go/cmd/` → repo root (`go.mod`, `go.sum`, `sqlc.yaml`, `internal/`, `cmd/`)
- Move: `internal/server/` → `internal/engine/` (package `engine`); `internal/server/handler/` → `internal/engine/handler/`
- Merge then delete: `go/.gitignore` → root `.gitignore`
- Delete: `go/CLAUDE.md` (context-mode boilerplate the root `CLAUDE.md` already carries)
- Modify: `.gitignore`, `.github/workflows/ci.yml` (Go job), `.github/workflows/release.yml` (`gates` job Go paths)

**Interfaces:**
- Produces: package `github.com/itterno/bistro/internal/engine` (was `.../internal/server`) and `.../internal/engine/handler`, with no API changes. Repo-root `go vet ./...` and `go test ./...` work.

- [ ] **Step 1: Move the files with history.**
  ```bash
  git mv go/go.mod go.mod && git mv go/go.sum go.sum && git mv go/sqlc.yaml sqlc.yaml
  git mv go/internal internal && git mv go/cmd cmd
  git mv internal/server internal/engine
  git rm go/CLAUDE.md
  ```
  Then merge each line of `go/.gitignore` into the root `.gitignore`, rewriting any `go/`-relative path, and `git rm go/.gitignore`. `go/dist` and `go/logs` are untracked build output; delete them from disk.
- [ ] **Step 2: Rename the package.** Run `package server` → `package engine` in every file in `internal/engine/` (not `handler/`). Rewrite the imports `github.com/itterno/bistro/internal/server` → `/internal/engine` and `/internal/server/handler` → `/internal/engine/handler` repo-wide (`gofmt -l .` must stay clean). Fix identifiers that said `server.` in `cmd/`.
- [ ] **Step 3: Keep Go out of the JS trees.** `node_modules/flatted` ships `.go` files. Add this to `go.mod` under the `go` line:
  ```
  ignore (
  	./node_modules
  	./electron
  	./frontend
  	./website
  )
  ```
  `frontend/` holds no Go and no embed target is resolved through `./...` patterns, so ignoring it is safe; `main.go`'s `//go:embed` is unaffected by `ignore`. If the toolchain rejects `ignore`, stop and report: the Go version in `go.mod` is wrong.
- [ ] **Step 4: Update `.gitignore`.** Change `go/bistro-sync` → `/bistro-sync` and `go/data/` → `/data/`. After the existing `build/` line, add `!/build/`, then add `/bin/`, `/.dev-data/` and `/frontend/dist/`.
- [ ] **Step 5: Update CI.** In `ci.yml` (Go job) and `release.yml` (`gates`): `go-version-file: go.mod`, `cache-dependency-path: go.sum`, and remove `working-directory: go`. Electron steps stay until Task 13.
- [ ] **Step 6: Verify.**
  ```bash
  go list ./... | grep -E 'node_modules|website|electron' ; echo "exit=$? (expect 1: no matches)"
  go vet ./... && go test -race ./...
  go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate && git diff --exit-code internal/store/db
  ```
  Expected: no matches, vet and tests pass, and no sqlc drift.
- [ ] **Step 7: Commit.** `git add -A && git commit -m "refactor: move Go module to repo root and rename server to engine"`

### Task 2: Services foundation

**Files:**
- Modify: `go.mod`, `go.sum` (`go get github.com/wailsapp/wails/v3@v3.0.0`)
- Create: `internal/services/errors.go`, `internal/services/guard.go`, `internal/services/emitter.go`, `internal/services/ids.go`, `internal/services/refs.go`, `internal/services/events.go`, `internal/services/set.go`
- Create: `internal/services/servicestest/recorder.go`
- Create: `internal/engine/services.go`
- Modify: `internal/engine/app.go` (add `emitter` and `panics` fields, `SetEmitter`, `SetPanicReporter`)
- Test: `internal/services/errors_test.go`, `internal/services/guard_test.go`, `internal/services/ids_test.go`

**Interfaces:**
- Produces:
  - `services.Code` and its constants `CodeNotFound`, `CodeUnauthenticated`, `CodeRateLimited`, `CodeInvalid`, `CodeCancelled`, `CodeTimeout`, `CodeInternal`
  - `type ServiceError struct{ Code Code; Message string; Reason string }`
  - `func Errorf(code Code, format string, args ...any) *ServiceError`
  - `func WithReason(code Code, reason, message string) *ServiceError`
  - `type PanicReporter interface{ ReportPanic(op string, recovered any, stack []byte) }`
  - `func guard[T any](rep PanicReporter, op string, fn func() (T, error)) (T, error)`
  - `func guardErr(rep PanicReporter, op string, fn func() error) error`
  - `type Emitter interface{ Emit(name string, data any) }`, `EmitterFunc`, `NopEmitter`
  - `func NormalizeID(string) string`, `func NormalizeIDs([]string) []string`
  - `PRRef{PRID string \`json:"pr_id"\`}`, `RepoRef{RepoID string \`json:"repo_id"\`}`
  - `services.Set` (empty struct; later tasks add fields)
  - `func (a *App) Services() *services.Set` (engine)
  - `servicestest.Recorder` with `Emit`, `Events() []servicestest.Event`, `Named(name string) []any`

- [ ] **Step 1: Add Wails.** `go get github.com/wailsapp/wails/v3@v3.0.0`. Confirm `go.mod` shows exactly `v3.0.0`.
- [ ] **Step 2: `errors.go`.**
  ```go
  package services

  import (
  	"context"
  	"errors"
  	"fmt"
  )

  // Code is the closed set of error categories the renderer maps onto tagged faults.
  type Code string

  const (
  	CodeNotFound        Code = "not_found"
  	CodeUnauthenticated Code = "unauthenticated"
  	CodeRateLimited     Code = "rate_limited"
  	CodeInvalid         Code = "invalid"
  	CodeCancelled       Code = "cancelled"
  	CodeTimeout         Code = "timeout"
  	CodeInternal        Code = "internal"
  )

  // ServiceError is the only error a service method returns. Wails marshals it
  // as the rejection cause. Reason is an optional domain code the UI may branch on.
  type ServiceError struct {
  	Code    Code   `json:"code"`
  	Message string `json:"message"`
  	Reason  string `json:"reason,omitempty"`
  }

  func (e *ServiceError) Error() string {
  	if e.Reason != "" {
  		return fmt.Sprintf("%s (%s): %s", e.Code, e.Reason, e.Message)
  	}
  	return fmt.Sprintf("%s: %s", e.Code, e.Message)
  }

  func Errorf(code Code, format string, args ...any) *ServiceError {
  	return &ServiceError{Code: code, Message: fmt.Sprintf(format, args...)}
  }

  func WithReason(code Code, reason, message string) *ServiceError {
  	return &ServiceError{Code: code, Reason: reason, Message: message}
  }

  // toServiceError returns nil for nil, otherwise a *ServiceError: one already in
  // the chain wins, then context errors, then internal. Returning error (not
  // *ServiceError) avoids a typed-nil interface.
  func toServiceError(err error) error {
  	if err == nil {
  		return nil
  	}
  	var se *ServiceError
  	if errors.As(err, &se) {
  		return se
  	}
  	switch {
  	case errors.Is(err, context.Canceled):
  		return &ServiceError{Code: CodeCancelled, Message: "cancelled"}
  	case errors.Is(err, context.DeadlineExceeded):
  		return &ServiceError{Code: CodeTimeout, Message: "timed out"}
  	}
  	return &ServiceError{Code: CodeInternal, Message: err.Error()}
  }
  ```
- [ ] **Step 3: `guard.go`.**
  ```go
  package services

  import "runtime/debug"

  // PanicReporter receives panics recovered from service calls (Sentry in production).
  type PanicReporter interface {
  	ReportPanic(op string, recovered any, stack []byte)
  }

  // guard runs the body of service method op. A panic becomes an internal
  // ServiceError and is reported, so one broken call cannot take the app down.
  func guard[T any](rep PanicReporter, op string, fn func() (T, error)) (out T, err error) {
  	defer func() {
  		if r := recover(); r != nil {
  			if rep != nil {
  				rep.ReportPanic(op, r, debug.Stack())
  			}
  			var zero T
  			out, err = zero, &ServiceError{Code: CodeInternal, Message: "internal error"}
  		}
  	}()
  	out, err = fn()
  	return out, toServiceError(err)
  }

  func guardErr(rep PanicReporter, op string, fn func() error) error {
  	_, err := guard(rep, op, func() (struct{}, error) { return struct{}{}, fn() })
  	return err
  }
  ```
- [ ] **Step 4: `emitter.go`, `refs.go`, `set.go`.**
  ```go
  // Emitter publishes push events to the renderer; production wraps app.Event.Emit.
  type Emitter interface{ Emit(name string, data any) }
  type EmitterFunc func(name string, data any)
  func (f EmitterFunc) Emit(name string, data any) { f(name, data) }
  // NopEmitter drops events (engine default until main.go sets the real one).
  type NopEmitter struct{}
  func (NopEmitter) Emit(string, any) {}
  ```
  `refs.go` holds `PRRef` and `RepoRef` from Interfaces. `set.go`:
  ```go
  // Set holds one instance of every service; main.go registers each field with Wails.
  type Set struct{}
  ```
- [ ] **Step 5: `ids.go`.** Port `lowercaseGitHubID` from `internal/ws/dispatch.go` as `NormalizeID`: it lowercases the whole ID only when it starts with `github:` in any case, and returns other IDs unchanged. `NormalizeIDs` maps over a slice and returns nil for nil. Leave `internal/ws` alone; Task 9 deletes it.
- [ ] **Step 6: `events.go` and the recorder.** `events.go` is a header comment plus an empty `func init() {}`; later tasks add `const` names, payload structs and `application.RegisterEvent[...]` lines. In `servicestest/recorder.go`, `Recorder` is a mutex-guarded slice of `Event{Name string; Data any}`. `Named(name)` returns the data of every event with that name, in order. Do not import `services`; `Recorder` satisfies `Emitter` structurally.
- [ ] **Step 7: Engine wiring.** In `app.go`, add the fields `emitter services.Emitter` (set to `services.NopEmitter{}` in `New`/`NewWithLogger`) and `panics services.PanicReporter` (nil by default), plus `SetEmitter(services.Emitter)` and `SetPanicReporter(services.PanicReporter)`. Create `internal/engine/services.go`:
  ```go
  // Services builds the service set from the engine's prepared dependencies.
  // Valid only after the stores are prepared (Run today, Start from Task 9).
  func (a *App) Services() *services.Set { return &services.Set{} }
  ```
- [ ] **Step 8: Tests.**
  - `errors_test.go`, a table over `toServiceError`: nil → nil; `fmt.Errorf("wrap: %w", WithReason(CodeInvalid, "x", "m"))` → that same error with `Reason` `"x"`; `context.Canceled` (wrapped) → `cancelled`; `context.DeadlineExceeded` → `timeout`; `errors.New("boom")` → `internal` with message `"boom"`.
  - `guard_test.go`:
    - `fn` panics: the result is the zero value plus a `CodeInternal` `ServiceError`, and a fake reporter receives `op == "X.Y"` and the recovered value.
    - `fn` returns a plain error: the result is `CodeInternal`.
    - `fn` succeeds: the value passes through with a nil error.
    - `guardErr` panics: the result is `CodeInternal`.
  - `ids_test.go`: port the identity cases from `internal/ws/dispatch_test.go` (`GitHub:Owner/Repo#1` → lowercase; non-github unchanged).
- [ ] **Step 9: Verify.** `go vet ./... && go test -race ./internal/services/... && go test -race ./...` all pass.
- [ ] **Step 10: Commit.** `git add -A && git commit -m "feat: add Wails service foundation (errors, guard, emitter, ids)"`

### Task 3: `Auth` service

**Files:**
- Create: `internal/services/auth.go`; Test: `internal/services/auth_test.go` (ported from `internal/engine/handler/auth_test.go`)
- Delete: `internal/engine/handler/auth.go`, `auth_test.go`; registration in `internal/engine/handlers.go` (`registerAuthHandlers`)
- Modify: `internal/services/events.go`, `internal/services/set.go`, `internal/engine/services.go`

**Interfaces:**
- Consumes: Task 2's foundation.
- Produces:
  - `type Auth struct{ Log *slog.Logger; Tokens TokenStore; Cache UsernameCache; Events Emitter; SyncTrigger chan<- string; SetProvider func(provider.Provider, string); GetClientID, GetClientSecret func() string; CallbackTimeout time.Duration; CurrentUserWriter CurrentUserWriter; CurrentUserClearer CurrentUserClearer; Panics PanicReporter }`. The interface types move from the handler package with the same method sets.
  - `func (s *Auth) Start(ctx context.Context, req AuthStartRequest) (AuthIdentity, error)`
  - `func (s *Auth) Check(ctx context.Context) (AuthStatus, error)`
  - `func (s *Auth) Disconnect(ctx context.Context, req AuthDisconnectRequest) (AuthStatus, error)`
  - Event `EventAuthRedirect = "auth:redirect"` with `AuthRedirect{Provider string \`json:"provider"\`; URL string \`json:"url"\`}`
  - `Set.Auth *Auth`

- [ ] **Step 1: Port `Check` and `Disconnect`.** Follow the recipe. `AuthStatus` has the json fields `authenticated`, `providers` (`[]AuthIdentity`), `auth_status`, `auth_reason`, `requires_auth` and `exchange`. `Disconnect` keeps the old order: delete the token, the cache entry and the current user, call `SetProvider(nil, name)`, then return `Check`.
- [ ] **Step 2: Port the flow as a blocking `Start`.** Keep the single-flight `owner` mutex. Map each pre-flight failure using the inventory's auth table. Once the callback listener is serving, emit `EventAuthRedirect` with the authorize URL. Then `select` on four cases:
  - the callback result: success → persist the token, write the current user, call `triggerSync`, return the identity
  - `time.After(s.CallbackTimeout)` → `Errorf(CodeTimeout, "authentication timed out")`
  - `ctx.Done()` → `Errorf(CodeCancelled, "authentication cancelled")`
  - failure → `WithReason(CodeUnauthenticated, "authentication_failed", …)`

  Always shut down the callback server and release `owner` in a `defer`. `AUTH_CANCEL` has no replacement: renderer cancellation cancels `ctx`.
- [ ] **Step 3: Register the event.** In `events.go`, add `const EventAuthRedirect = "auth:redirect"`, the `AuthRedirect` struct, and `application.RegisterEvent[AuthRedirect](EventAuthRedirect)` in `init`.
- [ ] **Step 4: Wire it.** Add `Auth *Auth` to `Set`. In `engine/services.go`, build it from the same values `registerAuthHandlers` used, with `Events: a.emitter` and `Panics: a.panics`. Delete `registerAuthHandlers` and its call.
- [ ] **Step 5: Tests.** Port the existing auth tests to direct calls. Add two new behaviour tests:
  - **Cancel frees the slot.** Start in a goroutine with a cancellable ctx and a fake callback that never answers. Wait until the recorder has `auth:redirect`, then cancel. Assert `CodeCancelled`, then assert a second `Start` is **not** rejected with Reason `auth_already_in_progress`; cancel it too.
  - **Concurrent Start is rejected.** While one Start is pending, a second returns `CodeInvalid` with Reason `auth_already_in_progress`.

  Use the existing test seams for the callback listener; if the old tests bound the real port 19284, keep doing that.
- [ ] **Step 6: Verify.** `go vet ./... && go test -race ./internal/services/... ./internal/engine/... && go test -race ./...`
- [ ] **Step 7: Commit.** `git commit -am "feat: port auth handlers to the Auth service"` (plus `git add` for the new files).

### Task 4: `PR`, `Sync`, `Repos`, `ReviewQueue` services

**Files:**
- Create: `internal/services/{pr,sync,repos,review_queue}.go` with matching `_test.go` files, ported from `handler/{pr,sync,repo,review_queue}_test.go`
- Delete: `handler/{pr,sync,repo,review_queue}.go` and their tests; their lines in `registerReviewHandlers`
- Modify: `internal/engine/services.go`, `set.go`, `internal/engine/app.go` (add `ApplySyncSettings`), `internal/engine/app.go` `Run` (drop `waitReviewQueue`)

**Interfaces:**
- Consumes: Task 2.
- Produces:
  - `func (s *PR) List(ctx, PRListRequest) (PRListResponse, error)`
  - `func (s *PR) Sync(ctx, PRRef) (PRRef, error)`
  - `func (s *Sync) Force(ctx, ForceSyncRequest) (SyncTriggered, error)`
  - `func (s *Repos) List(ctx, RepoListRequest) (RepoListResponse, error)`
  - `func (s *Repos) SetWatched(ctx, SetWatchedRequest) (WatchResult, error)`
  - `func (s *ReviewQueue) Get(ctx) (provider.ReviewQueue, error)`
  - engine: `func (a *App) ApplySyncSettings(autoSync bool, interval time.Duration)`
  - `Set.PR`, `Set.Sync`, `Set.Repos`, `Set.ReviewQueue`

  Field names and json tags are in [inventory-engine.md](inventory-engine.md). `PRListResponse.PRs` is `[]PRRow` with the old `prRow` tags.

- [ ] **Step 1: Port `PR.List` and `PR.Sync`.** `List` keeps the defaults (filter `open`, limit 100, max 200) and the expertise `touches_you` enrichment. On a DB error it returns `CodeInternal` instead of an empty list with `error`. `PR.Sync` calls `SyncPR` and blocks; it maps a missing provider to `CodeUnauthenticated`.
- [ ] **Step 2: Port `Sync.Force`.** Same queueing via `trigger chan<- string`; `queued` is false when the channel is full. Delete the `SET_SYNC_SETTINGS` handler and add the engine method below, which sends `SyncSettingsMsg` on the existing `a.syncSettings` channel. The 30–3600 s clamp moves here from Electron's `index.ts`:
  ```go
  // ApplySyncSettings forwards the user's sync preferences to the sync loop.
  // interval is clamped to 30s–3600s, as the Electron shell did.
  func (a *App) ApplySyncSettings(autoSync bool, interval time.Duration) {
  	interval = min(max(interval, 30*time.Second), time.Hour)
  	select {
  	case a.syncSettings <- SyncSettingsMsg{AutoSync: autoSync, Interval: interval}:
  	default:
  		a.log.Warn("sync settings update dropped; channel full")
  	}
  }
  ```
  Adapt the `SyncSettingsMsg` field names to the existing type; read `syncloop.go`. Port the clamp into an `app_test.go` table only if the existing loop test seam makes it one call; otherwise skip the test, since it is a two-line clamp.
- [ ] **Step 3: Port `Repos`.** `List(cache_only)` keeps the 30 s provider timeout and the cache-only, provider-free path. A missing provider when `!cache_only` → `CodeUnauthenticated`; a provider failure → `CodeInternal`, message `unable to load repositories`. `SetWatched` keeps `WatchSyncTrigger`.
- [ ] **Step 4: Make `ReviewQueue.Get` blocking.** Validation and not-authenticated cases return errors (`CodeUnauthenticated`, or `CodeInvalid` for an unsupported provider). Otherwise it runs the fetch inline under `context.WithTimeout(ctx, 30*time.Second)`. Failure → `CodeInternal` with message `Unable to read review requests. Try again later.` The returned `wait` func and its defer in `Run` go away.
- [ ] **Step 5: Wire them.** Add the four fields to `Set`, construct them in `services.go`, and remove their `RegisterHandler` lines.
- [ ] **Step 6: Tests.** Port the existing tests. They must cover:
  - list defaults and the limit cap
  - `Force` with a full channel → `queued:false`
  - `Repos.List` unauthenticated → `CodeUnauthenticated`
  - the review queue happy path
  - the review queue provider error → `CodeInternal`
  - the review queue timeout (a fake provider that blocks on `ctx`) → `CodeTimeout`, through `toServiceError`'s `DeadlineExceeded` mapping
- [ ] **Step 7: Verify.** `go vet ./... && go test -race ./...`
- [ ] **Step 8: Commit.** `git add -A && git commit -m "feat: port PR, sync, repo and review-queue handlers to services"`
