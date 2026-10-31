# Wails v3 migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: subagent-driven-development task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Electron and the localhost WebSocket with one Wails v3 process that embeds the sync engine, serves typed Go services and events to the React renderer, and auto-updates from GitHub releases.

**Architecture:** The Go module moves to the repo root; `internal/server` becomes `internal/engine` (no WS). Every WS handler is rewritten as a method on a per-domain struct in `internal/services`. Electron's main process is ported to `internal/shell` (settings, editor launch, telemetry, updater). `main.go` builds the Wails app, and the renderer moves to `frontend/` and calls the generated bindings in `frontend/bindings/` through one `call.ts` helper.

**Tech Stack:** Go 1.26, `github.com/wailsapp/wails/v3 v3.0.0`, `@wailsio/runtime` 3.0.0, React 19 + Vite + Vitest + Biome (pnpm), `@thourum/fault`, `sentry-go`, `posthog-go`, `@sentry/browser`.

**Spec:** [`docs/superpowers/specs/2026-10-28-wails3-migration-design.md`](../../specs/2026-10-28-wails3-migration-design.md). Read it before your task.

**Branch:** `experiment/wails3-migration`, cut from `experiment/wails3`.

## Global Constraints

- Pin `github.com/wailsapp/wails/v3 v3.0.0` (Go), `@wailsio/runtime` `3.0.0` (npm). The CLI comes from `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`. Wails' built-in task runner is `wails3 task <name>`, so no separate `task` binary is needed.
- The Go module path stays `github.com/itterno/bistro`. Do not assert it from memory; read `go.mod`.
- Platforms: macOS (WKWebView), Windows (WebView2), Linux (WebKitGTK).
- Data continuity is nice to have, not required. Write no migration code. Settings start fresh. The DB and keychain token are reused only if their paths and service names resolve the same.
- The WebSocket is deleted entirely, including `cmd/bistro-sync`, the port config and the origin checks.
- Generated Wails types replace zod for engine data. zod remains only for genuinely external input (settings files, URLs).
- Service errors are `*ServiceError{Code, Message}` and `Code` is a closed set: `not_found`, `unauthenticated`, `rate_limited`, `invalid`, `cancelled`, `timeout`, `internal`.
- Renderer call timeout: 30 s by default, none for `Auth.Start` and the dev-folder scan. On timeout it calls `promise.cancel()` and returns a `timeout` fault.
- Updater: `github.New(github.Config{Repository: "Thourum/Bistro", Prerelease: true, ChecksumAsset: "SHA256SUMS"})`, `CheckInterval: 6 * time.Hour`. Assets: `bistro-darwin-arm64.zip`, `bistro-windows-amd64.zip`, `bistro-linux-amd64.tar.gz`, `SHA256SUMS`. First-install artifacts are named `*-installer.*`. Disabled in dev and untagged builds.
- Done limits on the packaged macOS app: idle `footprint` over the process tree ≤ 173 MiB, first-install DMG ≤ 40.5 MB.
- Repo conventions (`CLAUDE.md`): in TS, use `@thourum/fault` and never `try`/`catch`. Go owns all SQLite writes. Keybindings are pure config. Write tests only where they would fail if the logic broke; no tests for trivial wrappers, and none that mock the thing under test.
- The pre-commit hook blocks staged files over 20,000 bytes (bytes/4 > 5,000 tokens). It skips sqlc output and files marked generated. Split big authored files, and never use `--no-verify`. Enable it with `git config core.hooksPath .githooks`.
- Commit per task as `type: description`. Do not push.

## Plan decisions beyond the spec

The spec leaves these open. Each one is fixed here so tasks agree.

1. **`ServiceError.Reason`.** An optional `Reason string` (`json:"reason,omitempty"`) carries domain sub-codes the UI branches on, such as `clone_failed`, `repo_not_local` or `auth_already_in_progress`. This keeps `Code` closed.
2. **No `ok`/`error` fields in responses.** `data.error` strings, `ok:false` + `reason`, `success:false` + `error`, and `LocationErrorPayload` all become `ServiceError`s. Data *states* stay in the response: `Interdiff` `available`/`reason`, `Expertise` `status: "unavailable"`, `Triage` `state`/`reason`.
3. **Two extra domains.** `repo.go` (`REQUEST_REPO_LIST`, `TOGGLE_WATCH_REPO`) becomes a `Repos` service. `SUBMIT_ACTION` lives in `threads.go`, so it becomes `Threads.SubmitAction`.
4. **Engine-internal, not exposed.** `SET_SYNC_SETTINGS` (only Electron main sent it) becomes `engine.App.ApplySyncSettings`. Settings drive both it and `ApplyClassificationSettings` from Go. `Classification.SetSettings` stays exposed because `bench/keys.ts` calls it.
5. **Auth.Start.** It emits the progress event `auth:redirect` once the callback listener is up, blocks until the flow ends, and returns `AuthIdentity`. `ctx` cancellation replaces `AUTH_CANCEL`.
6. **Event names.** See [inventory-engine.md § Events](inventory-engine.md#events). The orchestrator's lowercase event types (`action_complete`, `rate_limited`, `pr_approved`) never matched the renderer's uppercase handlers, so those handlers were dead. The migration wires them on purpose. Events nobody consumes are not registered: `sync_started`, `sync_progress`, the orchestrator's `sync_complete`/`sync_error`, and `changes_requested`. `AUTH_TOKEN_EXPIRED` and `LOCATION_ERROR` have no Go emitter, so their renderer handlers are deleted rather than given events.
7. **No engine process.** The engine-status, restart and crash-report UI is deleted. That covers `services/engine.ts`, `EngineCrashedScreen.tsx`, `useEngineScreenData.ts`, `engineStatus`, and the `connecting.restart`/`connecting.report` actions. `connecting.openLogs` stays and calls `Shell.OpenLogFolder`.
8. **Data dir.** `os.UserConfigDir()/Bistro` matches Electron's `userData` on all three OSes, so `sync.db` and `settings.json` may be picked up again. This reconciles two spec statements, "Settings start fresh" (Decisions) and the settings path in §3. There is no migration code and no compatibility promise. A file already at that path goes through the same per-leaf validation as any other, and an unreadable one falls back to defaults. `BISTRO_DATA_DIR` overrides it, and `wails3 task dev` sets `./.dev-data`. Logs go to `<dataDir>/logs/bistro.log`. The keychain service stays `bistro-sync`.
9. **Settings writes are whole-object.** The renderer sends full `Settings`; Go validates per leaf and keeps the current value for an invalid leaf. `window` is shell-owned: `Set` always keeps the stored bounds and ignores the renderer's copy. External edits are picked up by polling the file's mtime every 2 s, which adds no dependency.
10. **Bench safety.** `call()` is for reads, and `callWrite()` refuses with `BENCH_WRITE_BLOCKED` while the keys bench is active. The keys bench is enabled by the URL `bench_keys=1` only (the Electron argv flag is gone).
11. **Dead paths are dropped.** `REQUEST_CI_CHECKS` (no Go handler) is removed from `usePRDetailData.ts`. The dead `window.electron` APIs (`db`, `fs`, `go`, `syncAction`, `onGoEvent`, `send`) go too. So do the `review_action_requested` and `fs_patch_read` telemetry events, which only those dead APIs fired.
12. **Renderer Sentry respects consent** (`privacy.telemetryEnabled`), like Go. This fixes a gap in the Electron build.
13. **Version.** Set by `-ldflags "-X main.version=<tag without v>"`, default `dev`. The frontend gets it through `BISTRO_VERSION` at build time. The GitHub client id moves to `-X main.embeddedGitHubClientID`, and the Sentry DSN to `-X main.sentryDSN`, overridable by `SENTRY_DSN`.
14. **Go tooling at the root.** `go.mod` gains an `ignore` directive for `node_modules`, `frontend` and `website`, because `node_modules/flatted` ships `.go` files. CI creates a placeholder `frontend/dist/index.html` before `go vet` so `//go:embed` resolves.

## Task index

| # | Task | File |
|---|---|---|
| 1 | Move Go module to root; rename `server` → `engine` | [tasks-01-04](tasks-01-04-go-foundation.md) |
| 2 | Services foundation: `ServiceError`, `guard`, `Emitter`, IDs, `Set` | tasks-01-04 |
| 3 | `Auth` service | tasks-01-04 |
| 4 | `PR`, `Sync`, `Repos`, `ReviewQueue` services | tasks-01-04 |
| 5 | `Diff`, `Patchsets`, `Triage`, `Interdiff`, `Viewed`, `ReadState` | [tasks-05-08](tasks-05-08-go-services.md) |
| 6 | `Comments`, `Dismiss`, `Threads`, `Drafts` | tasks-05-08 |
| 7 | `RepoLocation`, `DevFolderScan` | tasks-05-08 |
| 8 | `Expertise`, `Classification`, `Editor` | tasks-05-08 |
| 9 | Engine without WS: typed pushes, supervised loops, `Start`/`Stop` | [tasks-09-10](tasks-09-10-engine-settings.md) |
| 10 | `shell` settings store | tasks-09-10 |
| 11 | `shell` editor launch + `Shell` service | [tasks-11-12](tasks-11-12-shell.md) |
| 12 | Telemetry (Sentry, PostHog) + updater | tasks-11-12 |
| 13 | Move renderer to `frontend/`, delete `electron/` | [tasks-13-14](tasks-13-14-app.md) |
| 14 | `main.go`, `Taskfile.yml`, `build/`, bindings, `--smoke` | tasks-13-14 |
| 15 | Renderer `call.ts`, fault mapping, event subscription | [task-15](task-15-renderer-transport.md) |
| 16 | Renderer: auth, sync, PR list, repos, onboarding | [tasks-16-18](tasks-16-18-renderer.md) |
| 17 | Renderer: diff, review, comments, drafts, threads | tasks-16-18 |
| 18 | Renderer: repo location, expertise, classification, editor, Shell, settings; delete WS layer | tasks-16-18 |
| 19 | CI, release workflow, `scripts/smoke.sh` | [tasks-19-20](tasks-19-20-ship.md) |
| 20 | Docs and ADR 0001 amendment | tasks-19-20 |

Reference inventories, which are snapshots of the pre-migration code, so verify against the source:
- [inventory-engine.md](inventory-engine.md): every WS message → service method, and every push → event.
- [inventory-renderer.md](inventory-renderer.md): renderer call sites, event handlers, `window.electron` uses.
- [inventory-wails.md](inventory-wails.md): the Wails v3.0.0 APIs this plan relies on, checked in the module cache.

## Service port recipe (Tasks 3–8)

Every handler port follows this recipe. A task lists only what differs.

1. **Types.** Put request/response structs in `internal/services/<domain>.go`, exported, with the **same json tags** as the old payload, so renderer field names do not change. Shared request shapes are `PRRef{PRID string \`json:"pr_id"\`}` and `RepoRef{RepoID string \`json:"repo_id"\`}`, both from Task 2.
2. **Struct.** The service struct holds the old `handler.*Deps` fields directly, plus `Panics PanicReporter` and `Log *slog.Logger`. Wails binds **every exported method**, so helpers must be unexported, and params/returns must be JSON-serialisable concrete types (never `any`).
3. **Method shape:**
   ```go
   func (s *Diff) Get(ctx context.Context, req PRRef) (DiffResponse, error) {
   	return guard(s.Panics, "Diff.Get", func() (DiffResponse, error) {
   		prID := NormalizeID(req.PRID)
   		// old handler body, minus json.Unmarshal and the reply envelope
   	})
   }
   ```
   Use `guardErr` for methods that return only `error`. Call `NormalizeID`/`NormalizeIDs` on every `pr_id`, `repo_id` and `pr_ids` field first, because the WS dispatcher used to lowercase `github:` IDs.
4. **Errors.** Return `Errorf(code, ...)` or `WithReason(code, reason, msg)`. A plain Go error becomes `internal`, `context.Canceled` becomes `cancelled`, and `context.DeadlineExceeded` becomes `timeout` (in `guard`).
5. **Events.** Add a constant, a payload struct and an `application.RegisterEvent[Payload](name)` line to `internal/services/events.go`'s `init`. Emit **values, not pointers**, because Wails type-checks registered events exactly: `s.Events.Emit(EventDraftsChanged, DraftsChanged{PRID: id})`.
6. **Tests.** Move `internal/engine/handler/<x>_test.go` to `internal/services/<x>_test.go`. Call the methods directly; assert on the returned struct, on `ServiceError.Code`/`Reason` (`var se *ServiceError; errors.As(err, &se)`), and on `servicestest.Recorder`. Delete assertions about envelopes and request ids, since that behaviour is gone.
7. **Delete and wire.** Delete `handler/<x>.go` and its test, and remove its `RegisterHandler` lines from `internal/engine/handlers.go` (or from `expertise.go`/`repo_location.go`). Add a field to `services.Set` and build it in `internal/engine/services.go` from the same values the registration used.
8. **Shared helpers.** If a helper interface in `handler/` (for example `currentUserReader` or `diffReader`) is still used by an unported handler, **copy** it into `internal/services/deps.go` and leave the original. Task 9 deletes the package.
9. **Verify:** `go vet ./... && go test -race ./internal/services/... ./internal/engine/...`, then `go test -race ./...` before committing.

## Manual verification (owner, after Task 20)

These are Done criteria 2–4. They are not implementer tasks.
- **macOS:** sign in, sync, and open the kubernetes and google-cloud-go fixture PRs. Review with vim, emacs and standard navigation. Write drafts and submit them, then quit and relaunch with the drafts intact. Update from one tagged pre-release to the next through the built-in window. Check Ctrl+Tab in the standard keymap (known WKWebView risk).
- **Windows and Linux:** sign in, sync, open a diff, submit a comment.
- **Packaged macOS app:** idle `footprint` summed over the process tree ≤ 173 MiB; DMG ≤ 40.5 MB.
