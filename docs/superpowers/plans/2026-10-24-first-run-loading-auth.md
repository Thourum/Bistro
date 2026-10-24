# First-run loading and sign-in Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make engine startup and GitHub sign-in explain themselves, recover cleanly, and never leave a first-time user waiting without an action.

**Architecture:** Keep process recovery, log access and crash-report redaction in Electron main; keep OAuth cancellation, timeout and callback HTML in Go. Renderer services own requests and store transitions, a pure selector derives the sign-in screen, and the existing root-mounted hotkey registrar handles every new action. UI work follows these contracts without introducing another lifecycle owner.

**Tech Stack:** Go (`github.com/itterno/bistro`), standard-library `net/http` and `html/template`; Electron, React, TypeScript, Zod, TanStack Store/Hotkeys, `@thourum/fault`, Tailwind and Vitest. Use the installed versions from `go/go.mod`, `electron/package.json` and the root `pnpm-lock.yaml`; no new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-24-first-run-loading-auth-design.md`

## Global Constraints

- `SLOW_ENGINE_MS = 10_000`; derive slow from elapsed time without a ready engine. Initialising and connecting remain unchanged until that threshold.
- `MAX_RESTARTS = 3` remains in `go-spawn.ts`; manual restart resets the counter and uses the existing spawn path. No restart action in the slow state. Hide restart for engines Electron did not spawn.
- `REPORT_LOG_LINES = 60`; log file first, captured stderr only if missing or empty.
- `MAX_ISSUE_URL_BYTES = 7_500`; drop oldest log lines until the encoded URL is **under**, not equal to, the limit.
- Export one main-process `BISTRO_REPO = 'Thourum/Bistro'`; canonical URL is `https://github.com/Thourum/Bistro`.
- `auth.CallbackTimeout = 5m`, `CallbackPort = 19284`, callback path `/callback`. Production defaults do not change.
- OAuth scopes remain exactly `repo` and `read:user`.
- `exchange: 'proxy' | 'local'` comes from `AUTH_CHECK`; absent or disconnected means neutral copy, never a guessed mode.
- Only GitHub is available. Keep GitLab and Gitea in `PROVIDERS`, disabled, labelled “Coming soon”, without key hints or active bindings.
- Never claim the token “never leaves this machine”; never promise that sync has started on the callback page.
- Main IPC uses `{ ok: T } | { err: string }`; renderer validates external data at the edge and handles faults at the UI boundary. Never introduce `try`/`catch` into Electron TS/JS.
- All screen actions join `ACTION_CATALOG`, `SHARED_KEYS`, `ACTIONS` and `isBindingEnabled`; no second hotkey hook. Display effective bindings, not hard-coded keyboard hints.
- Below `md`, the sign-in card stacks with the trust panel second; the trust panel remains in every auth state.
- Callback HTML is one embedded self-contained template, system fonts only, no external requests, all dynamic values escaped by `html/template`.
- Callback light palette literals: surface `#f8f9fa`, lowest `#fff`, container `#edeeef`, primary `#004ac6`, outline-variant `#c3c6d7`, on-surface `#191c1d`, on-surface-variant `#434655`, error `#ba1a1a`. These literals are allowed only outside the app token system, in the callback template.
- No React component tests; add only the four behavior tests specified below. Verify UI manually in the running Electron app.
- Do not edit `CLAUDE.md`, `AGENTS.md`, or historical plan notes. This plan contains the integration notes executors need.
- Out of scope: sync selection/start/progress, GitLab/Gitea OAuth, embedded Geist, first-run rail and style practice.

---

## File Structure

Paths below are relative to the worktree root. Run all commands there unless a command explicitly changes into `go/` or `electron/`.

| Boundary | Create | Modify | Responsibility |
|---|---|---|---|
| Canonical repo | — | `electron/src/main/updater.ts`, `electron/package.json`, `electron/forge.config.ts`, `README.md`, `docs/ALPHA.md`, `design/contribution-guide.md`, `website/vocs.config.ts`, `website/src/pages/docs/index.mdx`, `website/src/pages/docs/feedback.mdx` | Repo links, publishing and update target |
| OAuth lifetime | — | `go/internal/auth/callback.go`, `go/internal/server/handler/auth.go`, `go/internal/server/handler/auth_test.go`, `go/internal/server/app.go` | Shared start/cancel state, safe shutdown, exchange mode |
| Callback page | `go/internal/auth/callback_page.go`, `go/internal/auth/callback.html` | `go/internal/auth/callback.go`, `go/internal/auth/callback_test.go` | Template, escaped failure detail, mode/OS copy |
| Manual timeout | — | `go/cmd/bistro-sync/main.go` | Duration flag passed through App into auth deps |
| Main engine API | `electron/src/types/engine.ts`, `electron/src/main/engineIpc.ts`, `electron/src/main/crashReport.ts`, `electron/tests/unit/crashReport.test.ts` | `electron/src/main/go-spawn.ts`, `electron/src/main/index.ts`, `electron/src/main/preload.ts`, `electron/src/renderer/types/window.d.ts` | Typed status, lifecycle callbacks, IPC, logs and report |
| Renderer logic | `electron/src/renderer/utils/authScreenState.ts`, `electron/src/renderer/services/engine.ts`, `electron/src/renderer/services/signIn.ts`, `electron/tests/unit/authScreenState.test.ts` | `electron/src/renderer/store/ui.ts`, `electron/src/renderer/types/wsSchemas.ts`, `electron/src/renderer/services/ws/events/auth.ts`, `electron/src/renderer/services/ws/connection.ts`, `electron/src/renderer/providers/{provider,github,gitlab,gitea}.ts`, `electron/src/renderer/utils/{keybindings,keymaps,keybindingActions,keybindingActivation,legendPresets}.ts`, `electron/src/renderer/hooks/useAppHotkeys.ts`, `electron/src/renderer/components/RootShell.tsx`, `electron/src/renderer/pages/settings/ConnectionsSection.tsx` | Auth state, IPC bridge, action services, activation |
| Loading UI | `electron/src/renderer/components/EngineCrashedScreen.tsx`, `electron/src/renderer/hooks/useEngineScreenData.ts` | `electron/src/renderer/components/ConnectingScreen.tsx`, `electron/src/renderer/components/RootShell.tsx` | Timers/log lifecycle and loading/crash surfaces |
| Sign-in UI | `electron/src/renderer/pages/login/TrustPanel.tsx`, `electron/src/renderer/pages/login/SignInState.tsx`, `electron/src/renderer/hooks/useSignInScreenData.ts` | `electron/src/renderer/pages/LoginPage.tsx`, `electron/src/renderer/layouts/AuthLayout.tsx`, `electron/src/renderer/components/ProviderButton.tsx`, `electron/src/main/preload.ts`, `electron/src/renderer/types/window.d.ts` | Two-column card, state UI, OS store name and honest trust copy |

**Task mapping:** The user's UI “tasks 6 and 7” are **Task 7 (loading/crashed UI)** and **Task 8 (sign-in UI)** here. Tasks 1–6 establish their contracts. Mark UI tasks accordingly when dispatching them; do not specify a model override.

### Task 1: Correct the canonical repo and share its name

**Files:** Modify `electron/src/main/updater.ts:10,33`, `electron/package.json:8`, `electron/forge.config.ts:123-127`, `README.md:63`, `docs/ALPHA.md:33`, `design/contribution-guide.md:29`, `website/vocs.config.ts:9,11`, `website/src/pages/docs/index.mdx:29,33`, `website/src/pages/docs/feedback.mdx:12`.

**Interfaces:**
- Consumes: Existing updater/publisher configuration and public repo URLs.
- Produces: `export const BISTRO_REPO = 'Thourum/Bistro'` from `electron/src/main/updater.ts`, imported by Task 5's crash report.

- [ ] **Step 1: Replace the updater constant and its use.**

```ts
export const BISTRO_REPO = 'Thourum/Bistro';
```

Replace every `BISTRO_UPDATE_REPO` reference in this file with `BISTRO_REPO`, including `repo:` and the final enabled-log interpolation. Preserve the existing opt-in environment gate.

- [ ] **Step 2: Replace the old repo owner in the listed user-facing/config files.**

Use `https://github.com/Thourum/Bistro` in links, retaining existing `/issues`, `/releases`, `.git` and other suffixes. In Forge's GitHub publisher retain `name: 'Bistro'` and set:

```ts
owner: 'Thourum',
```

Do not rename the Go module path, rewrite historical specs, or change the licence. `design/contribution-guide.md` and Forge were found beyond the spec's list and must not keep the stale destination.

- [ ] **Step 3: Verify the live references and Electron checks.**

```bash
git diff --check
cd electron && pnpm type-check && pnpm lint && pnpm test
```

Expected: checks pass; updater still requires `BISTRO_ENABLE_AUTO_UPDATES=1`; every changed link uses `Thourum/Bistro`.

- [ ] **Step 4: Commit.**

```bash
git add electron/src/main/updater.ts electron/package.json electron/forge.config.ts README.md docs/ALPHA.md design/contribution-guide.md website/vocs.config.ts website/src/pages/docs/index.mdx website/src/pages/docs/feedback.mdx
git commit -m "fix: use canonical Bistro GitHub repository"
```

### Task 2: Make OAuth start/cancel share one safe lifetime

**Files:** Modify `go/internal/auth/callback.go:41-48,113-128`, `go/internal/server/handler/auth.go:67-77,95-266,330-336`, `go/internal/server/handler/auth_test.go`, `go/internal/server/app.go:40-56,356-363,639-668`.

**Interfaces:**
- Consumes: `auth.NewCallbackServer(cfg, state, redirectURI, resultCh) *auth.CallbackServer`, `Shutdown(context.Context) error`, and `ws.MessageHandler` (`func(context.Context, ws.InboundMessage) (interface{}, error)`).
- Produces: `NewAuthFlowHandlers(deps AuthDeps) (start, cancel ws.MessageHandler)`; `AuthDeps.CallbackTimeout time.Duration` (zero defaults to `auth.CallbackTimeout`); `(*App).SetAuthCallbackTimeout(time.Duration)`; `AUTH_CANCEL` with no direct response and `AUTH_FAILED {error:'cancelled'}` broadcast; `AUTH_CHECK_RESULT.data.exchange`.

- [ ] **Step 1: Add the cancel/restart regression to `auth_test.go`.**

Reuse its existing `fakeTokenStore`, `fakeUsernameCache` and `slog` setup. Add `sync` and the `ws` import when absent. The second flow is cancelled before the test returns, so the real callback port is not leaked.

```go
func TestAuthCancelAllowsNextStart(t *testing.T) {
	var mu sync.Mutex
	var failures []string
	deps := handler.AuthDeps{
		Tokens: &fakeTokenStore{tokens: map[string]string{}},
		Cache: &fakeUsernameCache{usernames: map[string]string{}},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetClientID: func() string { return "id" },
		Broadcast: func(message interface{}) {
			mu.Lock()
			defer mu.Unlock()
			if envelope, ok := message.(map[string]interface{}); ok {
				if data, ok := envelope["data"].(map[string]string); ok {
					failures = append(failures, data["error"])
				}
			}
		},
	}
	start, cancel := handler.NewAuthFlowHandlers(deps)
	ctx := context.Background()
	request := ws.InboundMessage{Type: "AUTH_START", Data: json.RawMessage(`{"provider":"github"}`)}
	for attempt := 0; attempt < 2; attempt++ {
		response, err := start(ctx, request)
		if err != nil { t.Fatal(err) }
		envelope, ok := response.(map[string]interface{})
		if !ok || envelope["type"] != "AUTH_REDIRECT" { t.Fatalf("start: %#v", response) }
		response, err = cancel(ctx, ws.InboundMessage{Type: "AUTH_CANCEL"})
		if err != nil || response != nil { t.Fatalf("cancel: %#v, %v", response, err) }
	}
	mu.Lock()
	defer mu.Unlock()
	if len(failures) != 2 || failures[0] != "cancelled" || failures[1] != "cancelled" {
		t.Fatalf("failures: %#v", failures)
	}
}
```

Add `context`, `encoding/json`, `io`, `log/slog`, `net`, `sync`, and `time` imports when absent. Immediately after each cancel, assert the callback port is closed:

```go
connection, dialErr := net.DialTimeout("tcp", "localhost:19284", time.Second)
if dialErr == nil {
	_ = connection.Close()
	t.Fatal("callback listener is still open after cancellation")
}
```

The test must not call `t.Parallel()` because it uses the fixed OAuth port.

- [ ] **Step 2: Run the regression red.**

```bash
cd go && go test ./internal/server/handler -run TestAuthCancelAllowsNextStart -count=1
```

Expected: compile failure for `NewAuthFlowHandlers`, before implementation.

- [ ] **Step 3: Construct the HTTP server synchronously.**

Replace `NewCallbackServer`'s direct literal return with a local `cs`, then set its server before returning. Preserve its existing field values.

```go
cs.server = &http.Server{
	Addr: fmt.Sprintf("localhost:%d", CallbackPort),
	Handler: cs.handler(),
}
return cs
```

Replace the construction in `ListenAndServe` with:

```go
func (cs *CallbackServer) ListenAndServe() error {
	return cs.server.ListenAndServe()
}
```

This is necessary for cancel-before-listen: `Shutdown` must close an already-created server, not see `nil` and leave a future listener alive. It also removes the unsynchronised write/read of `cs.server`.

- [ ] **Step 4: Introduce shared flow handlers and owner-bound cleanup.**

Rename `NewAuthStartHandler` to `NewAuthFlowHandlers`, change its return signature, and retain the current start handler's validation, configuration and success/token-persistence code. Add `cancelFlow chan struct{}` beside `callbackServer`. Add these helpers above the constructor:

```go
func clientSecret(deps AuthDeps) string {
	if deps.GetClientSecret == nil { return "" }
	return deps.GetClientSecret()
}

func exchangeMode(deps AuthDeps) string {
	if clientSecret(deps) != "" { return "local" }
	return "proxy"
}
```

Replace direct `deps.GetClientSecret()` use with `clientSecret(deps)`. Change the flow closure's arguments to:

```go
type authCodeFlow struct {
	providerName string
	server *auth.CallbackServer
	results <-chan auth.CallbackResult
	cancelled <-chan struct{}
}
runAuthCodeFlow := func(flow authCodeFlow) {
	providerName, ownServer := flow.providerName, flow.server
	resultCh, cancelCh := flow.results, flow.cancelled
```

The listening goroutine uses **ownServer**, not the shared variable. Remove `close(serverErrCh)`; a clean shutdown is not a server error. Use:

```go
serverErrCh := make(chan error, 1)
go func() {
	if err := ownServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverErrCh <- err
	}
}()
``` Use a per-flow `sync.Once` cleanup so timeout/callback errors release the port and in-progress state **before** broadcasting an outcome:

```go
var cleanupOnce sync.Once
cleanup := func() {
	cleanupOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), callbackShutdownTimeout)
		defer cancel()
		if err := ownServer.Shutdown(ctx); err != nil {
			deps.Log.Warn("callback server shutdown failed", "error", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if callbackServer == ownServer { callbackServer = nil }
	})
}
defer cleanup()
```

In the existing `select`, call `cleanup()` at the beginning of each outcome branch. Preserve existing success handling. For a callback error, broadcast the safe engine error message, not a raw token-exchange response body. Use `timeout := deps.CallbackTimeout; if timeout <= 0 { timeout = auth.CallbackTimeout }`, select on `time.After(timeout)`, and retain `AUTH_FAILED {error:"timeout"}`. Add:

```go
case <-cancelCh:
	return
```

At the end of the start handler's setup, still under `mu`, snapshot the flow into local values:

```go
resultCh := make(chan auth.CallbackResult, 1)
ownServer := auth.NewCallbackServer(cfg, state, auth.RedirectURI, resultCh)
callbackServer = ownServer
cancelFlow = make(chan struct{})
cancelCh := cancelFlow
mu.Unlock()
go runAuthCodeFlow(authCodeFlow{providerName: data.Provider, server: ownServer, results: resultCh, cancelled: cancelCh})
```

- [ ] **Step 5: Add synchronous cancellation and return both handlers.**

Assign the existing start closure to `start` instead of immediately returning it. Cancel is idempotent and serialises shutdown with new starts. Holding `mu` during the bounded shutdown prevents another client's `AUTH_START` from racing the old listener; flow cleanup shuts down outside that lock, so there is no lock inversion.

```go
cancel = func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
	mu.Lock()
	ownServer := callbackServer
	if ownServer == nil {
		mu.Unlock()
		return nil, nil
	}
	close(cancelFlow)
	shutdownCtx, stop := context.WithTimeout(context.Background(), callbackShutdownTimeout)
	err := ownServer.Shutdown(shutdownCtx)
	stop()
	callbackServer = nil
	mu.Unlock()
	if err != nil { return nil, fmt.Errorf("cancel OAuth callback: %w", err) }
	deps.Broadcast(map[string]interface{}{
		"type": "AUTH_FAILED",
		"data": map[string]string{"error": "cancelled"},
	})
	return nil, nil
}
return start, cancel
```

Check `cancelCh` non-blockingly immediately after receiving a result and before token persistence, so a cancelled flow cannot persist a late callback:

```go
select {
case <-cancelCh:
	return
default:
}
```

- [ ] **Step 6: Wire timeout and exchange mode through App.**

Add `CallbackTimeout time.Duration` to `AuthDeps`, `authCallbackTimeout time.Duration` to `App`, and:

```go
func (a *App) SetAuthCallbackTimeout(duration time.Duration) {
	a.authCallbackTimeout = duration
}
```

In `registerAuthHandlers` add `CallbackTimeout: a.authCallbackTimeout`, then:

```go
start, cancel := handler.NewAuthFlowHandlers(authDeps)
a.wsServer.RegisterHandler("AUTH_START", start)
a.wsServer.RegisterHandler("AUTH_CANCEL", cancel)
```

Use the existing registration receiver name in this function. In `NewAuthCheckHandler`'s existing response data map add:

```go
"exchange": exchangeMode(deps),
```

Search all callers of the old constructor, including tests, and change each to take the start handler from `NewAuthFlowHandlers`; no compatibility wrapper is needed.

- [ ] **Step 7: Format and verify green, including the race detector.**

```bash
cd go && gofmt -w internal/auth/callback.go internal/server/handler/auth.go internal/server/handler/auth_test.go internal/server/app.go && go vet ./... && go test -race ./...
```

Expected: cancel → closed listener → immediate next start succeeds; no race on callback server initialization; default timeout remains five minutes.

- [ ] **Step 8: Commit.**

```bash
git add go/internal/auth/callback.go go/internal/server/handler/auth.go go/internal/server/handler/auth_test.go go/internal/server/app.go
git commit -m "feat: cancel pending GitHub sign-in safely"
```

### Task 3: Serve branded, escaped OAuth callback pages

**Files:** Create `go/internal/auth/callback_page.go`, `go/internal/auth/callback.html`; modify `go/internal/auth/callback.go:65-110,225`, `go/internal/auth/callback_test.go`.

**Interfaces:**
- Consumes: `ProviderConfig.ClientSecret`, callback request query values, Task 2's owner-bound flow cleanup and existing `CallbackResult`.
- Produces: `TokenError{Code, Description string}` implementing `error`; `writeCallbackPage(http.ResponseWriter, int, callbackPage)`, used only inside auth; one embedded template with four failure branches and OS/exchange-aware success copy.

- [ ] **Step 1: Add the failure-page table regression.**

Keep the existing callback tests. Add `strings` and `time` to imports if absent. Each exchange-failure case uses a local token endpoint; no GitHub network traffic.

```go
func TestCallbackFailurePage(t *testing.T) {
	cases := []struct {
		name, query, code, detail string
		status int
	}{
		{"state", "state=wrong&code=x", "INVALID_STATE", "state mismatch", http.StatusForbidden},
		{"denied", "state=valid&error=access_denied&error_description=%3Cscript%3E", "access_denied", "&lt;script&gt;", http.StatusBadRequest},
		{"missing", "state=valid", "MISSING_CODE", "missing code", http.StatusBadRequest},
		{"exchange", "state=valid&code=x", "TOKEN_EXCHANGE_FAILED", "bad_verification_code", http.StatusInternalServerError},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"error":"bad_verification_code","error_description":"<script>"}`))
			}))
			defer tokenEndpoint.Close()
			results := make(chan CallbackResult, 1)
			cs := NewCallbackServer(ProviderConfig{TokenURL: tokenEndpoint.URL}, "valid", RedirectURI, results)
			response := httptest.NewRecorder()
			cs.handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?"+test.query, nil))
			if response.Code != test.status { t.Fatalf("status %d", response.Code) }
			body := response.Body.String()
			for _, want := range []string{test.code, test.detail, "Sign-in didn", "Nothing was saved", "description", "at"} {
				if !strings.Contains(body, want) { t.Fatalf("missing %q in %s", want, body) }
			}
			if strings.Contains(body, "<script>") { t.Fatal("unescaped detail") }
			select {
			case result := <-results:
				if result.Err == nil { t.Fatal("failure did not reach auth flow") }
			case <-time.After(time.Second):
				t.Fatal("missing callback result")
			}
		})
	}
}
```

- [ ] **Step 2: Run the table red.**

```bash
cd go && go test ./internal/auth -run TestCallbackFailurePage -count=1
```

Expected: existing unbranded `http.Error` responses fail copy/detail assertions.

- [ ] **Step 3: Add the embedded page renderer.**

Create `callback_page.go` in package `auth`, importing `bytes`, `_ "embed"`, `html/template`, `net/http`, `runtime`, `time`, and the existing logging package.

```go
//go:embed callback.html
var callbackHTML string

var callbackTemplate = template.Must(template.New("callback").Parse(callbackHTML))

type callbackPage struct {
	Success bool
	Code, Sentence, Error, Description, At string
	ExchangeStep, Store string
}

func keyringName() string {
	names := map[string]string{
		"darwin": "macOS Keychain",
		"windows": "Windows Credential Manager",
		"linux": "the Secret Service keyring on Linux",
	}
	if name, ok := names[runtime.GOOS]; ok { return name }
	return "your operating system's keyring"
}

func successPage(cfg ProviderConfig) callbackPage {
	step := "Bistro's sign-in endpoint swapped it for a token and passed it straight back. It stored nothing."
	if cfg.ClientSecret != "" { step = "Bistro swapped it for a token on this machine." }
	return callbackPage{Success: true, ExchangeStep: step, Store: keyringName()}
}

func failurePage(page callbackPage) callbackPage {
	return callbackPage{Code: page.Code, Sentence: page.Sentence, Error: page.Error,
		Description: page.Description, At: time.Now().UTC().Format(time.RFC3339)}
}

func writeCallbackPage(w http.ResponseWriter, status int, page callbackPage) {
	var body bytes.Buffer
	if err := callbackTemplate.Execute(&body, page); err != nil {
		http.Error(w, "Unable to display sign-in result. Return to Bistro.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(body.Bytes()); err != nil {
		logging.FromContext(nil).Warn("callback page write failed", "error", err)
	}
}
```

The logging package already exposes `logging.FromContext`; use `logging.FromContext(nil)` for the default logger in this response writer. No new logging API is needed.

- [ ] **Step 4: Add `callback.html` with the actual copy and palette.**

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bistro — GitHub sign-in</title>
<style>
:root{--surface:#f8f9fa;--lowest:#fff;--container:#edeeef;--primary:#004ac6;--outline-variant:#c3c6d7;--on-surface:#191c1d;--on-surface-variant:#434655;--error:#ba1a1a}
*{box-sizing:border-box}body{margin:0;background:var(--surface);color:var(--on-surface);font:16px/1.6 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
main{max-width:720px;margin:64px auto;padding:32px;background:var(--lowest);border:1px solid var(--outline-variant);border-radius:8px}
.brand{font-weight:700;letter-spacing:.08em}.code{font:13px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace;color:var(--primary);margin-top:32px}
.failure{color:var(--error)}h1{font-size:32px;line-height:1.2}h2{font-size:16px}section{margin-top:24px;padding:24px;background:var(--container)}li{margin:12px 0}dt{font:13px ui-monospace,SFMono-Regular,Consolas,monospace;color:var(--on-surface-variant)}dd{margin:0 0 16px;overflow-wrap:anywhere}a{color:var(--primary)}kbd{font:inherit}
@media(max-width:600px){main{margin:24px 16px;padding:24px}h1{font-size:28px}}
</style>
</head>
<body><main>
<div class="brand">BISTRO</div>
{{if .Success}}
<p class="code">AUTHORIZATION COMPLETE // GITHUB</p>
<h1>You're signed in.</h1>
<p>Close this tab and return to Bistro.</p>
<section><h2>Where the token went</h2><ol>
<li>GitHub returned a one-time authorization code.</li>
<li>{{.ExchangeStep}}</li>
<li>Bistro saved the token in {{.Store}}.</li>
</ol></section>
<p>Revoke any time: <a href="https://github.com/settings/applications">github.com/settings/applications</a>.</p>
{{else}}
<p class="code failure">{{.Code}}</p>
<h1>Sign-in didn't finish.</h1>
<p>{{.Sentence}}</p>
<p>Nothing was saved. Return to Bistro and press <kbd>G</kbd> to try again.</p>
<section><h2>Detail</h2><dl>
<dt>error</dt><dd>{{.Error}}</dd>
<dt>description</dt><dd>{{.Description}}</dd>
<dt>at</dt><dd>{{.At}}</dd>
</dl></section>
{{end}}
</main></body></html>
```

- [ ] **Step 5: Preserve upstream OAuth detail as a typed error.**

In `callback.go`, add:

```go
type TokenError struct { Code, Description string }

func (e *TokenError) Error() string { return e.Code + ": " + e.Description }
```

In `ExchangeToken`, replace `fmt.Errorf("%s: %s", tr.Error, tr.ErrorDesc)` with `&TokenError{Code: tr.Error, Description: tr.ErrorDesc}`. This preserves its human-readable error string while letting the callback extract both fields with `errors.As`.

- [ ] **Step 6: Replace callback responses in validation order.**

Keep `sendResult` for every failure. Check state **before** GitHub's `error` query, then missing code, then exchange. The page data for each branch is:

```go
failurePage(callbackPage{Code: "INVALID_STATE", Sentence: "The sign-in response did not match this attempt.",
	Error: "INVALID_STATE", Description: "state mismatch"})
failurePage(callbackPage{Code: githubError, Sentence: "GitHub did not authorize this sign-in.",
	Error: githubError, Description: r.URL.Query().Get("error_description")})
failurePage(callbackPage{Code: "MISSING_CODE", Sentence: "GitHub did not return an authorization code.",
	Error: "MISSING_CODE", Description: "missing code"})
```

Use 403, 400, 400 respectively. For exchange errors:

```go
upstreamError, description := "TOKEN_EXCHANGE_FAILED", "Unable to exchange the authorization code."
var tokenError *TokenError
if errors.As(err, &tokenError) {
	upstreamError, description = tokenError.Code, tokenError.Description
}
writeCallbackPage(w, http.StatusInternalServerError,
	failurePage(callbackPage{Code: "TOKEN_EXCHANGE_FAILED", Sentence: "Bistro could not exchange the authorization code for a token.", Error: upstreamError, Description: description}))
cs.sendResult(CallbackResult{Err: fmt.Errorf("token exchange: %w", err)})
```

On success call `writeCallbackPage(w, http.StatusOK, successPage(cs.cfg))`, then keep the existing `cs.sendResult(CallbackResult{Token: token})`. Do not insert raw query or response bodies into HTML, logs or renderer faults. Delete the old inline success HTML and any now-unused imports.

- [ ] **Step 7: Format, run table and full Go checks.**

```bash
cd go && gofmt -w internal/auth/callback.go internal/auth/callback_page.go internal/auth/callback_test.go && go vet ./... && go test -race ./...
```

Expected: all four cases return their required HTTP status and escaped detail, and send a callback error that the flow broadcasts as `AUTH_FAILED`.

- [ ] **Step 8: Commit.**

```bash
git add go/internal/auth/callback.go go/internal/auth/callback_page.go go/internal/auth/callback.html go/internal/auth/callback_test.go
git commit -m "feat: brand GitHub OAuth callback pages"
```

### Task 4: Expose a short callback timeout for manual verification

**Files:** Modify `go/cmd/bistro-sync/main.go:26-37,89`.

**Interfaces:**
- Consumes: Task 2's `(*App).SetAuthCallbackTimeout(time.Duration)` and `auth.CallbackTimeout`.
- Produces: CLI `-auth-callback-timeout` duration flag; default five minutes, manually overridable without changing production behavior.

- [ ] **Step 1: Add the duration flag and App setter.**

Import `github.com/itterno/bistro/internal/auth`, then alongside the other flags:

```go
authCallbackTimeout := flag.Duration("auth-callback-timeout", auth.CallbackTimeout,
	"OAuth callback wait duration (override for manual timeout checks)")
```

After `app.SetAutoSync`, add:

```go
app.SetAuthCallbackTimeout(*authCallbackTimeout)
```

- [ ] **Step 2: Check flag help and Go verification.**

```bash
cd go && gofmt -w cmd/bistro-sync/main.go && go run ./cmd/bistro-sync -h && go vet ./... && go test -race ./...
```

Expected: help shows `-auth-callback-timeout duration` with `(default 5m0s)`; no new one-line setter test.

- [ ] **Step 3: Commit.**

```bash
git add go/cmd/bistro-sync/main.go
git commit -m "feat: allow manual OAuth timeout checks"
```

### Task 5: Expose engine status, logs, recovery and a redacted report from main

**Files:** Create `electron/src/types/engine.ts`, `electron/src/main/engineIpc.ts`, `electron/src/main/crashReport.ts`, `electron/tests/unit/crashReport.test.ts`; modify `electron/src/main/go-spawn.ts:82-90,193-220,attachProcessLifecycle,stderr handler`, `electron/src/main/index.ts:ready handler`, `electron/src/main/preload.ts`, `electron/src/renderer/types/window.d.ts`.

**Interfaces:**
- Consumes: Task 1's `BISTRO_REPO`; `GoSpawnOptions`, `spawnGoProcess(options): Promise<number>`; `AppConfig.goProcess.logFile`; `app.getVersion()`, `shell.openPath`, `shell.openExternal`.
- Produces: `EngineStatus`/schema; `getStderrTail(): readonly string[]`; `restartGoProcess(options: GoSpawnOptions): Promise<number>`; `onStarting`, `onReady`, `onGiveUp` lifecycle callbacks; `buildCrashIssueUrl(input: Readonly<CrashReportInput>): string`; `registerEngineIPC(options: EngineIPCOptions): void`; six preload methods and two allowed push channels.

- [ ] **Step 1: Write the main redaction/URL-trim table test.**

```ts
import { describe, expect, it, vi } from 'vitest';
vi.mock('../../src/main/updater', () => ({ BISTRO_REPO: 'Thourum/Bistro' }));
import { buildCrashIssueUrl, MAX_ISSUE_URL_BYTES } from '../../src/main/crashReport';

const base = { version: '0.1.0', platform: 'darwin', arch: 'arm64', code: 1, signal: null, home: '/Users/reviewer' };

describe('buildCrashIssueUrl', () => {
  it.each([
    ['ghp_secret', '[redacted]'],
    ['gho_secret', '[redacted]'],
    ['ghu_secret', '[redacted]'],
    ['ghs_secret', '[redacted]'],
    ['ghr_secret', '[redacted]'],
    ['github_pat_secret', '[redacted]'],
    ['Authorization: Bearer private-token', 'Authorization: [redacted]'],
    ['http://localhost/callback?code=private-code&state=private-state', 'code=[redacted]&state=[redacted]'],
    ['/Users/reviewer/projects/repo', '~/projects/repo'],
  ])('redacts %s before encoding', (line, expected) => {
    const body = new URL(buildCrashIssueUrl({ ...base, logs: [line] })).searchParams.get('body');
    expect(body).toContain(expected);
    expect(body).not.toContain(line);
  });

  it('drops oldest lines and preserves readable metadata and the newest tail', () => {
    const logs = Array.from({ length: 60 }, (_, index) => `line-${index} ${'界'.repeat(150)}`);
    const url = buildCrashIssueUrl({ ...base, logs });
    expect(Buffer.byteLength(url, 'utf8')).toBeLessThan(MAX_ISSUE_URL_BYTES);
    const body = new URL(url).searchParams.get('body') ?? '';
    expect(body).toContain('0.1.0');
    expect(body).toContain('darwin / arm64');
    expect(body).not.toContain('line-0 ');
    expect(body).toContain('line-59 ');
  });

  it('can drop a single oversized line without exceeding the URL limit', () => {
    const url = buildCrashIssueUrl({ ...base, logs: ['x'.repeat(20_000)] });
    expect(Buffer.byteLength(url, 'utf8')).toBeLessThan(MAX_ISSUE_URL_BYTES);
  });
});
```

- [ ] **Step 2: Run this test red.**

```bash
cd electron && pnpm test tests/unit/crashReport.test.ts
```

Expected: missing crash-report module.

- [ ] **Step 3: Implement the report builder with redaction before URL encoding.**

```ts
import { BISTRO_REPO } from './updater';

export const REPORT_LOG_LINES = 60;
export const MAX_ISSUE_URL_BYTES = 7_500;

export interface CrashReportInput {
  readonly version: string;
  readonly platform: string;
  readonly arch: string;
  readonly code: number | null;
  readonly signal: string | null;
  readonly home: string;
  readonly logs: readonly string[];
}

function redactLog(line: string, home: string): string {
  const redacted = line
    .replace(/\b(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)\b/g, '[redacted]')
    .replace(/(Authorization:\s*)[^\r\n]+/gi, '$1[redacted]')
    .replace(/([?&](?:code|state)=)[^&#\s]*/gi, '$1[redacted]');
  return home === '' ? redacted : redacted.split(home).join('~');
}

export function buildCrashIssueUrl(input: Readonly<CrashReportInput>): string {
  const logs = input.logs.slice(-REPORT_LOG_LINES).map((line) => redactLog(line, input.home));
  const exit = input.signal ?? String(input.code ?? 'unknown');
  const makeUrl = (tail: readonly string[]): string => {
    // Tildes avoid engine output containing backticks closing the fenced block.
    const body = `Version: ${input.version}\nOS / architecture: ${input.platform} / ${input.arch}\nEngine exit: ${exit}\n\n~~~~text\n${tail.join('\n').replaceAll('~~~~', '[log fence]')}\n~~~~\n`;
    const query = new URLSearchParams({ title: 'Sync engine stopped', body });
    return `https://github.com/${BISTRO_REPO}/issues/new?${query}`;
  };
  let oldest = 0;
  let url = makeUrl(logs);
  while (Buffer.byteLength(url, 'utf8') >= MAX_ISSUE_URL_BYTES && oldest < logs.length) {
    oldest += 1;
    url = makeUrl(logs.slice(oldest));
  }
  return url;
}
```

The fixed metadata is main-owned (not user-provided); the URL limit is enforced on the encoded URL, not the decoded log. No automatic issue submission, no attachment/upload, no redaction of repo names or PR titles.

- [ ] **Step 4: Define the shared status and IPC schemas.**

Create `types/engine.ts`:

```ts
import { z } from 'zod';

export const SLOW_ENGINE_MS = 10_000;
export const engineStatusSchema = z.object({
  state: z.enum(['starting', 'ready', 'crashed']),
  startedAt: z.number().int().nonnegative(),
  isSpawned: z.boolean(),
  code: z.number().int().nullable(),
  signal: z.string().nullable(),
  error: z.string().nullable(),
});
export type EngineStatus = z.infer<typeof engineStatusSchema>;
export type EngineReply<T> = { ok: T } | { err: string };
export const engineLogTailSchema = z.array(z.string());
export const engineLogRequestSchema = z.object({ lines: z.number().int().min(1).max(60) });
```

`startedAt` is supplied by main **before spawn**, not by the renderer when its window finally opens.

- [ ] **Step 5: Extend the existing spawn lifecycle rather than adding a process manager.**

Import `SLOW_ENGINE_MS` from `../types/engine` and replace the private `GO_READY_TIMEOUT_MS = 15_000` with `GO_READY_TIMEOUT_MS = SLOW_ENGINE_MS`. Change the two “15 seconds” log messages to “10 seconds”. This only opens the slow-start window; the ready waiter remains alive and no process is stopped. Keeping 15 seconds would make the required 10-second slow screen impossible to see at first launch.

Extend `GoSpawnOptions` and export the existing `ProcessExit` type:

```ts
onStarting?: () => void;
onReady?: (port: number) => void;
onGiveUp?: (exit: ProcessExit) => void;
```

Invoke `options.onStarting?.()` immediately before a real spawn and before waiting for an external engine. In `markReady`, after `lastKnownPort = port`, invoke `options.onReady?.(port)`. Invoke it in the external-engine success path too.

In `handleProcessExit`, at `restartCount >= maxRestarts`, call `options.onGiveUp?.(exit)` before returning. In `attachProcessLifecycle`, a startup exit is not retried: call `onGiveUp` in that branch before settling the ready waiter with its fault. Failed automatic respawns must also call `onGiveUp` if they end without another retry; they must not leave status forever `starting`.

Capture stderr in a closure-local ring buffer, not another exported mutable array:

```ts
const STDERR_TAIL_LINES = 60;
const stderrTail = (() => {
  let lines: readonly string[] = [];
  return {
    append(line: string): void { lines = [...lines, line].slice(-STDERR_TAIL_LINES); },
    snapshot(): readonly string[] { return [...lines]; },
  };
})();

export function getStderrTail(): readonly string[] { return stderrTail.snapshot(); }
```

In the existing stderr line callback add `stderrTail.append(line)` without removing readiness detection. Preserve captured lines across restarts so an early crash retains context. Add:

```ts
export function restartGoProcess(options: GoSpawnOptions): Promise<number> {
  restartCount = 0;
  const config = lastKnownPort === null ? options.config : { ...options.config, wsPort: lastKnownPort };
  return spawnGoProcess({ ...options, config });
}
```

Do not kill a still-migrating process or expose this on the slow screen. The IPC guard below admits restart only in the terminal crashed state for a spawned engine.

- [ ] **Step 6: Implement log access and the six IPC handlers in `engineIpc.ts`.**

Import `readFile` from `node:fs/promises`, `homedir` from `node:os`, `dirname` from `node:path`, `Fault`, `fromPromise`, `errAsync`, `okAsync`, `ServiceError`, `ResultAsync` from `@thourum/fault`, `app`, `ipcMain`, `shell` from Electron, the engine schemas/types, report builder and `getStderrTail`.

```ts
interface EngineIPCOptions {
  readonly logFile: string;
  readonly getStatus: () => EngineStatus;
  readonly restart: () => ResultAsync<void, Fault>;
}

function engineFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause.message : String(cause))
    .withTag('ENGINE_ERROR').withCause(cause);
}

function readLogTail(logFile: string, count: number): ResultAsync<readonly string[], Fault> {
  return fromPromise(readFile(logFile, 'utf8'), (cause) => {
    if (cause instanceof Error && 'code' in cause && cause.code === 'ENOENT') {
      return ServiceError('LOG_MISSING', 'Engine log file is not present.').withCause(cause);
    }
    return engineFault(cause);
  })
    .orElse((fault) => fault.tag === 'LOG_MISSING' ? okAsync('') : errAsync(fault))
    .map((text) => {
      const lines = text.split(/\r?\n/).filter((line) => line.trim() !== '');
      return (lines.length > 0 ? lines : getStderrTail()).slice(-count);
    });
}

export function registerEngineIPC(options: EngineIPCOptions): void {
  ipcMain.handle('engine:get-status', () => ({ ok: options.getStatus() }));
  ipcMain.handle('app:get-version', () => ({ ok: app.getVersion() }));
  ipcMain.handle('engine:restart', () => {
    const status = options.getStatus();
    if (!status.isSpawned || status.state !== 'crashed') return { err: 'This engine cannot be restarted here.' };
    return options.restart().match(() => ({ ok: true }), (fault) => ({ err: fault.message }));
  });
  ipcMain.handle('engine:log-tail', (_event, input: unknown) => {
    const parsed = engineLogRequestSchema.safeParse(input);
    if (!parsed.success) return { err: 'lines must be an integer from 1 to 60.' };
    return readLogTail(options.logFile, parsed.data.lines)
      .match((lines) => ({ ok: lines }), (fault) => ({ err: fault.message }));
  });
  ipcMain.handle('engine:open-log-folder', () =>
    fromPromise(shell.openPath(dirname(options.logFile)), engineFault)
      .andThen((message) => message === '' ? okAsync(true) : errAsync(ServiceError('ENGINE_ERROR', message)))
      .match((value) => ({ ok: value }), (fault) => ({ err: fault.message })),
  );
  ipcMain.handle('engine:report-crash', () => {
    const status = options.getStatus();
    if (status.state !== 'crashed') return { err: 'Crash reporting is available after the engine stops.' };
    return readLogTail(options.logFile, REPORT_LOG_LINES)
      .map((logs) => buildCrashIssueUrl({ version: app.getVersion(), platform: process.platform,
        arch: process.arch, code: status.code, signal: status.signal, home: homedir(), logs }))
      .andThen((url) => fromPromise(shell.openExternal(url), engineFault))
      .match(() => ({ ok: true }), (fault) => ({ err: fault.message }));
  });
}
```

Import `REPORT_LOG_LINES` alongside the report builder. `EngineIPCOptions` can remain private because index's object literal is structurally checked. Missing/empty files fall back; permission/read failures surface as IPC errors, not silently empty logs. Tagged errors avoid assuming an undocumented Fault cause accessor.

- [ ] **Step 7: Integrate status and recovery in the `ready` handler in `main/index.ts`.**

Keep the existing settings/telemetry setup. Use a local status holder in the ready boundary and broadcast after every transition:

```ts
let engineStatus: EngineStatus = {
  state: 'starting', startedAt: Date.now(), isSpawned: process.env.BISTRO_SKIP_GO_SPAWN !== '1',
  code: null, signal: null, error: null,
};
const publishEngineStatus = (patch: Partial<EngineStatus>): void => {
  engineStatus = { ...engineStatus, ...patch };
  for (const win of BrowserWindow.getAllWindows()) win.webContents.send('engine:status', engineStatus);
};
```

Extract the current successful spawn branch (actual-port `runtimeConfig`, `new WebSocketClient`, connected bootstrap/settings change listener, `wsClient.connect()`) into the local `connectMainToEngine(actualWsPort: number): void`. Retain IPv4 `ws://127.0.0.1:${actualWsPort}/ws`, not `localhost`. Install settings listeners only once; a retained main WS client reconnects to the retained port on ordinary respawns.

Build a single `GoSpawnOptions` object used for initial and manual starts:

```ts
const spawnOptions: GoSpawnOptions = {
  config: appConfig.goProcess,
  logger: log,
  onStarting: () => publishEngineStatus({ state: 'starting', startedAt: Date.now(), error: null }),
  onReady: () => publishEngineStatus({ state: 'ready', code: null, signal: null, error: null }),
  onStillStarting: () => {
    fromThrowable(() => createWindow(runtimeConfig ?? appConfig, log), mainFault)()
      .orInspect((fault) => { log.error('Failed to show slow-start window', fault.message); app.quit(); });
  },
  onCrash: (code, signal) => {
    publishEngineStatus({ state: 'starting', code, signal });
    for (const win of BrowserWindow.getAllWindows()) win.webContents.send('go:crashed', { code, signal });
  },
  onGiveUp: ({ code, signal }) => publishEngineStatus({
    state: 'crashed', code, signal, error: `Engine exited: ${signal ?? code ?? 'unknown'}`,
  }),
};
```

Register the IPC once, before spawning:

```ts
registerEngineIPC({
  logFile: appConfig.goProcess.logFile,
  getStatus: () => engineStatus,
  restart: () => fromPromise(restartGoProcess(spawnOptions), mainFault).map((port) => {
    if (wsClient === null) {
      connectMainToEngine(port);
      for (const win of BrowserWindow.getAllWindows()) win.webContents.reload();
    }
  }),
});
```

Initial success calls `connectMainToEngine(port)` then the existing guarded `createWindow`. Initial failure publishes `crashed` (including the fault message, preserving exit fields), then creates the existing fallback window. For an unspawned external engine failure, `isSpawned` remains false. If manual restart fails before an exit event exists, map the fault to `crashed` too; do not leave status starting. Reload after successful recovery from an **initial** failure because that renderer was created without the real dynamic port. `createWindow` already returns an existing window, so no duplicate-window manager is needed.

- [ ] **Step 8: Expose typed methods and events through both preload declarations.**

Add `'engine:status'` and `'go:crashed'` to `VALID_EVENT_CHANNELS` and mirror them in `renderer/types/window.d.ts`. Add these methods to the exposed object:

```ts
getEngineStatus: (): Promise<EngineReply<EngineStatus>> => ipcRenderer.invoke('engine:get-status'),
getAppVersion: (): Promise<EngineReply<string>> => ipcRenderer.invoke('app:get-version'),
restartEngine: (): Promise<EngineReply<boolean>> => ipcRenderer.invoke('engine:restart'),
getEngineLogTail: (lines: number): Promise<EngineReply<readonly string[]>> => ipcRenderer.invoke('engine:log-tail', { lines }),
openLogFolder: (): Promise<EngineReply<boolean>> => ipcRenderer.invoke('engine:open-log-folder'),
reportCrash: (): Promise<EngineReply<boolean>> => ipcRenderer.invoke('engine:report-crash'),
```

Update the duplicate `declare global` in preload and `window.d.ts` with identical method signatures. Extend the existing channel-dependent callback type: `engine:status` payload is `EngineStatus`, `go:crashed` is `{code:number|null; signal:string|null}`. Preserve the old settings/sync callback types. Renderer still validates actual IPC/push data; a preload type annotation is not validation.

- [ ] **Step 9: Verify and commit.**

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

Expected: report table passes; updater behavior remains opt-in; no new thrown/caught errors, raw credentials or debug logs.

```bash
git add electron/src/types/engine.ts electron/src/main/engineIpc.ts electron/src/main/crashReport.ts electron/tests/unit/crashReport.test.ts electron/src/main/go-spawn.ts electron/src/main/index.ts electron/src/main/preload.ts electron/src/renderer/types/window.d.ts
git commit -m "feat: expose engine recovery and redacted crash reports"
```

### Task 6: Build sign-in state, services and screen-aware actions

**Files:** Create `electron/src/renderer/utils/authScreenState.ts`, `electron/src/renderer/services/engine.ts`, `electron/src/renderer/services/signIn.ts`, `electron/tests/unit/authScreenState.test.ts`; modify the renderer logic files listed in File Structure.

**Interfaces:**
- Consumes: Task 2's `AUTH_CANCEL` and optional `exchange`; Task 5's schemas/preload methods/status push; existing `wsClient.send(type: string, data: unknown): string | undefined`, `openUrlInBrowser(url): ResultAsync<void, Fault>`, `appStore`, `parseResponse`, `addToast` and settings-derived bindings.
- Produces: `AuthScreenState`; `getAuthScreenState(input: Readonly<AuthScreenInput>): AuthScreenState`; `initEngineStatus(): () => void`; engine action services; `startSignIn(provider: ProviderId): void`, `reopenSignIn(): void`, `copySignInLink(): void`, `cancelSignIn(): void`, `retrySignIn(): void`; store fields and a shared `FirstRunScreen` activation contract.

- [ ] **Step 1: Write the auth-screen selector table test.**

```ts
import { expect, it } from 'vitest';
import { getAuthScreenState, type AuthScreenInput } from '../../src/renderer/utils/authScreenState';

const idle: AuthScreenInput = {
  isConnected: true, authLoading: false, authUrl: null, authError: null,
  authTimedOut: false, authInterrupted: false,
};
it.each([
  [idle, 'idle'],
  [{ ...idle, authLoading: true, authUrl: 'https://github.com/login/oauth/authorize' }, 'waiting'],
  [{ ...idle, authTimedOut: true }, 'timedOut'],
  [{ ...idle, authError: 'denied' }, 'failed'],
  [{ ...idle, authInterrupted: true }, 'interrupted'],
  [{ ...idle, isConnected: false }, 'engineDown'],
  [{ ...idle, isConnected: false, authLoading: true, authUrl: 'https://github.com/login/oauth/authorize' }, 'interrupted'],
])('maps %j to %s', (input, expected) => {
  expect(getAuthScreenState(input)).toBe(expected);
});
```

- [ ] **Step 2: Run red and implement the pure selector.**

```bash
cd electron && pnpm test tests/unit/authScreenState.test.ts
```

Expected: missing selector module. Create it:

```ts
export type AuthScreenState = 'idle' | 'waiting' | 'timedOut' | 'failed' | 'interrupted' | 'engineDown';
export type FirstRunScreen = AuthScreenState | 'connecting' | 'slow' | 'crashed';
export interface AuthScreenInput {
  readonly isConnected: boolean;
  readonly authLoading: boolean;
  readonly authUrl: string | null;
  readonly authError: string | null;
  readonly authTimedOut: boolean;
  readonly authInterrupted: boolean;
}

export function getAuthScreenState(input: Readonly<AuthScreenInput>): AuthScreenState {
  const isWaiting = input.authLoading || input.authUrl !== null;
  if (input.authInterrupted || (!input.isConnected && isWaiting)) return 'interrupted';
  if (!input.isConnected) return 'engineDown';
  if (input.authTimedOut) return 'timedOut';
  if (isWaiting) return 'waiting';
  if (input.authError !== null) return 'failed';
  return 'idle';
}
```

- [ ] **Step 3: Extend the auth/engine store and WS schema.**

In `UIState` and initial state add:

```ts
authUrl: string | null;                 // initially null
authStartedAt: number | null;           // initially null
authTimedOut: boolean;                 // initially false
authInterrupted: boolean;              // initially false
authExchange: 'proxy' | 'local' | null; // initially null
engineStatus: EngineStatus | null;     // initially null
appVersion: string | null;             // initially null
```

In `types/wsSchemas.ts`, add to the existing `AUTH_CHECK_RESULT` object:

```ts
exchange: z.enum(['proxy', 'local']).optional(),
```

Search `types/index.ts` for a separate auth-check response schema and add the same optional field if that schema exists. Old engines without the field must still parse and show neutral copy. Derive types from schemas; no unvalidated casts.

- [ ] **Step 4: Preserve redirect data and map auth outcomes in `events/auth.ts`.**

Use the existing `openUrlInBrowser` instead of reimplementing browser IPC. On `AUTH_REDIRECT`, set the store **before** launching the browser:

```ts
appStore.setState((state) => ({
  ...state, authUrl: data.url, authStartedAt: Date.now(), authLoading: true,
  authError: null, authTimedOut: false, authInterrupted: false,
}));
void openUrlInBrowser(data.url).match(
  () => undefined,
  (fault) => addToast({ type: 'error', message: fault.message, dismissible: true }),
);
```

Browser-launch failure retains the URL/waiting state so the user can copy or reopen it. It must not strand an engine flow while returning the app to idle.

Replace `handleAuthFailed`'s state update and toast with:

```ts
const isCancelled = data.error === 'cancelled';
const isTimeout = data.error === 'timeout';
appStore.setState((state) => ({
  ...state, authLoading: false, authUrl: null, authStartedAt: null,
  authTimedOut: isTimeout, authInterrupted: false,
  authError: isCancelled || isTimeout ? null : data.error,
}));
if (!isCancelled && !isTimeout) {
  addToast({ type: 'error', message: data.error, dismissible: true });
}
```

In `handleAuthCheckResult`, remove the unconditional `authLoading: false`; the rest becomes:

```ts
appStore.setState((state) => ({ ...state, authChecked: true,
  isAuthenticated: data.authenticated, connectedProviders: data.providers ?? [],
  authExchange: data.exchange ?? null,
}));
```

Do not erase `authInterrupted` on reconnect or reset an active sign-in from an unrelated auth-check reply. In `handleAuthSuccess`'s existing update add/replace:

```ts
authLoading: false,
authUrl: null,
authStartedAt: null,
authTimedOut: false,
authInterrupted: false,
authError: null,
```

Retain its existing authenticated/provider updates. The existing expiry/disconnect flows continue to work.

- [ ] **Step 5: Mark a waiting sign-in interrupted on every WS close.**

In `services/ws/connection.ts`'s existing `onClose` store update, add:

```ts
authInterrupted: state.authInterrupted || state.authLoading || state.authUrl !== null,
authLoading: false,
authUrl: null,
authStartedAt: null,
authTimedOut: false,
authExchange: null,
```

Use the updater callback's existing parameter name `previous` in this block (or rename the parameter consistently to `state`). Keep `authChecked` and reconnect/auth bootstrap semantics unchanged. The alert/status block is based on actual WS connectivity, even when the primary auth state is interrupted. `wsClient` has no public `isConnected()` method; add this selector to `authScreenState.ts` and use it in services/hooks:

```ts
export function isEngineConnected(status: string): boolean {
  return ['connected', 'ready', 'syncing', 'rate_limited'].includes(status);
}
```

`disconnected`, `connecting` and `error` are disconnected. This uses the current transport's connection-state contract, not a second socket owner.

- [ ] **Step 6: Add renderer engine IPC services and a one-time root subscription.**

In `services/engine.ts`, import `ResultAsync`, `Fault`, `ServiceError`, `err`, `ok`, `errAsync`, `fromThrowable` from `@thourum/fault`, `z` from `zod`, `engineStatusSchema` and `engineLogTailSchema` from `../../types/engine`, `parseResponse` from `../utils/error`, `appStore` from `../store` and `addToast` from `../components/ToastContainer`. The complete IPC boundary/subscription code is:

```ts
function engineRequest<T>(request: () => Promise<unknown>, schema: z.ZodType<T>): ResultAsync<T, Fault> {
  return fromThrowable(request, engineIpcFault)()
    .asyncAndThen((promise) => ResultAsync.fromPromise(promise, engineIpcFault))
    .andThen((response) => {
      const envelope = z.union([z.object({ ok: schema }), z.object({ err: z.string() })]).safeParse(response);
      if (!envelope.success) return err(ServiceError('ENGINE_IPC_ERROR', 'Invalid engine response.'));
      return 'err' in envelope.data ? err(ServiceError('ENGINE_IPC_ERROR', envelope.data.err)) : ok(envelope.data.ok);
    });
}

function engineIpcFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause.message : String(cause))
    .withTag('ENGINE_IPC_ERROR').withCause(cause);
}

function showEngineError(fault: Fault): void {
  addToast({ type: 'error', message: fault.message, dismissible: true });
}

export function getEngineLogTail(lines: number): ResultAsync<readonly string[], Fault> {
  const api = window.electron;
  if (api === undefined) return errAsync(ServiceError('ENGINE_IPC_ERROR', 'Engine controls are unavailable.'));
  return engineRequest(() => api.getEngineLogTail(lines), engineLogTailSchema);
}

export function restartEngine(): void {
  const api = window.electron;
  if (api === undefined) { showEngineError(ServiceError('ENGINE_IPC_ERROR', 'Engine controls are unavailable.')); return; }
  void engineRequest(() => api.restartEngine(), z.boolean()).match(() => undefined, showEngineError);
}

export function openEngineLogFolder(): void {
  const api = window.electron;
  if (api === undefined) { showEngineError(ServiceError('ENGINE_IPC_ERROR', 'Engine controls are unavailable.')); return; }
  void engineRequest(() => api.openLogFolder(), z.boolean()).match(() => undefined, showEngineError);
}

export function reportEngineCrash(): void {
  const api = window.electron;
  if (api === undefined) { showEngineError(ServiceError('ENGINE_IPC_ERROR', 'Engine controls are unavailable.')); return; }
  void engineRequest(() => api.reportCrash(), z.boolean()).match(() => undefined, showEngineError);
}

export function initEngineStatus(): () => void {
  const api = window.electron;
  if (api === undefined) return () => undefined;
  let isActive = true;
  let receivedPush = false;
  const unsubscribe = api.on('engine:status', (payload) => {
    receivedPush = true;
    parseResponse(engineStatusSchema, payload).match(
      (engineStatus) => appStore.setState((state) => ({ ...state, engineStatus })),
      showEngineError,
    );
  });
  void engineRequest(() => api.getEngineStatus(), engineStatusSchema).match(
    (engineStatus) => {
      if (isActive && !receivedPush) appStore.setState((state) => ({ ...state, engineStatus }));
    },
    (fault) => { if (isActive) showEngineError(fault); },
  );
  void engineRequest(() => api.getAppVersion(), z.string()).match(
    (appVersion) => { if (isActive) appStore.setState((state) => ({ ...state, appVersion })); },
    (fault) => { if (isActive) showEngineError(fault); },
  );
  return () => { isActive = false; unsubscribe(); };
}
```

Export `getEngineLogTail(lines: number): ResultAsync<readonly string[], Fault>` using `engineLogTailSchema`; export void `restartEngine`, `openEngineLogFolder`, `reportEngineCrash` which call their preload method through this adapter and `.match` at the service's toast boundary. Missing Electron methods return a tagged `ENGINE_IPC_ERROR` with “Engine controls are unavailable.” rather than asserting non-null.

Implement `initEngineStatus(): () => void`: subscribe to `'engine:status'` first, then fetch `getEngineStatus` and `getAppVersion` once. Validate push status with `parseResponse(engineStatusSchema, payload)`, store it on success, toast on invalid data. Snapshot/status ordering must not overwrite a newer push: retain a closure-local `receivedPush` boolean and ignore a snapshot after the first push. Return the unsubscribe function. In RootShell, call it in a mount effect:

```ts
useEffect(() => initEngineStatus(), []);
```

Do not couple this subscription to the WS; crash actions must remain available while it is down. Allowing `go:crashed` in preload is compatibility; `engine:status` is the terminal-state authority, so an ordinary crash notification must not immediately show the crashed screen while automatic retries remain.

- [ ] **Step 7: Add sign-in services used by buttons and key actions.**

`services/signIn.ts` imports `PROVIDERS`, `ProviderId`, `appStore`, `wsClient`, `openUrlInBrowser`, `isEngineConnected`, `ResultAsync`/`Fault`, and `addToast`. Export:

```ts
export function startSignIn(provider: ProviderId): void {
  const definition = PROVIDERS.find((entry) => entry.id === provider);
  if (!definition?.isAvailable || !isEngineConnected(appStore.state.syncStatus) || appStore.state.authLoading) return;
  appStore.setState((state) => ({ ...state, authLoading: true, authError: null,
    authTimedOut: false, authInterrupted: false, authUrl: null, authStartedAt: null }));
  const sentId = wsClient.send('AUTH_START', { provider });
  if (sentId === undefined) {
    appStore.setState((state) => ({ ...state, authLoading: false, authError: 'Cannot connect to sync engine.' }));
  }
}

export function reopenSignIn(): void {
  const url = appStore.state.authUrl;
  if (url === null) return;
  void openUrlInBrowser(url).match(() => undefined,
    (fault) => addToast({ type: 'error', message: fault.message, dismissible: true }));
}

export function copySignInLink(): void {
  const url = appStore.state.authUrl;
  if (url === null) return;
  void ResultAsync.fromPromise(navigator.clipboard.writeText(url), (cause) =>
    new Fault(cause instanceof Error ? cause.message : String(cause)).withTag('CLIPBOARD_ERROR').withCause(cause),
  ).match(() => undefined,
    (fault) => addToast({ type: 'error', message: fault.message, dismissible: true }));
}

export function cancelSignIn(): void {
  if (appStore.state.authLoading || appStore.state.authUrl !== null) {
    if (!isEngineConnected(appStore.state.syncStatus)) return;
    const sentId = wsClient.send('AUTH_CANCEL', {});
    if (sentId === undefined) {
      addToast({ type: 'error', message: 'Cannot cancel while the engine is disconnected.', dismissible: true });
    }
    return;
  }
  appStore.setState((state) => ({ ...state, authTimedOut: false, authInterrupted: false, authError: null }));
}

export function retrySignIn(): void { startSignIn('github'); }
```

Cancel while waiting awaits the engine's cancelled event; timeout Escape is purely local back-to-hosts. Update `ConnectionsSection.tsx` to call `startSignIn` so unavailable providers and stale auth state cannot bypass the shared boundary. Its existing click/confirming-action routes should use that service, not duplicate `AUTH_START`.

- [ ] **Step 8: Mark provider availability and remove unavailable actions.**

Add `readonly isAvailable: boolean` to `ProviderDef`; set `isAvailable: true` in `github.ts`, false in `gitlab.ts` and `gitea.ts`. Keep all three registry entries. Filter by `.isAvailable` in `LOGIN_BINDINGS`, provider-generated `SHARED_KEYS`, and the `getBindingAction` provider lookup. This prevents old overrides from activating disabled providers.

- [ ] **Step 9: Register keys and handlers in the existing catalog.**

Add `'connecting'` to `RouteBase` and `ALL_ROUTES`; add `connecting: []` to `legendPresets.ts`'s exhaustive route table. Append to login catalog and create the connecting group:

```ts
const SIGN_IN_ACTIONS: readonly BindingDefinition[] = [
  { actionId: 'login.reopen', description: 'Reopen browser', activation: { routes: ['login'] } },
  { actionId: 'login.copyLink', description: 'Copy sign-in link', activation: { routes: ['login'] } },
  { actionId: 'login.cancel', description: 'Cancel / Back to hosts', activation: { routes: ['login'] } },
  { actionId: 'login.retry', description: 'Try again', activation: { routes: ['login'] } },
];
const CONNECTING_BINDINGS: readonly BindingDefinition[] = [
  { actionId: 'connecting.restart', description: 'Restart engine', activation: { routes: ['connecting'] } },
  { actionId: 'connecting.openLogs', description: 'Open log folder', activation: { routes: ['connecting'] } },
  { actionId: 'connecting.report', description: 'Report on GitHub', activation: { routes: ['connecting'] } },
];
```

Spread both into `ACTION_CATALOG`. Add these exact entries to `SHARED_KEYS`:

```ts
'connecting.restart': 'r',
'connecting.openLogs': 'l',
'connecting.report': 'i',
'login.reopen': 'r',
'login.copyLink': 'c',
'login.cancel': 'Escape',
'login.retry': 'Enter',
```

Add to `ACTIONS` in `keybindingActions.ts` using the Task 6 services:

```ts
'connecting.restart': () => restartEngine(),
'connecting.openLogs': () => openEngineLogFolder(),
'connecting.report': () => reportEngineCrash(),
'login.reopen': () => reopenSignIn(),
'login.copyLink': () => copySignInLink(),
'login.cancel': () => cancelSignIn(),
'login.retry': () => retrySignIn(),
```

The existing provider action may directly call `startSignIn(provider.id)` instead of writing `confirmingAction`; update LoginPage in Task 8 accordingly so keyboard and mouse share one boundary. Preserve other confirming-action consumers.

- [ ] **Step 10: Gate actions on the derived screen, not just the URL.**

Extend `isBindingEnabled`'s context with optional `screen?: FirstRunScreen` and `isSpawned?: boolean` to preserve unrelated callers. Before the existing onboarding sandbox logic, apply this table:

```ts
const FIRST_RUN_ACTION_STATES: Readonly<Record<string, readonly FirstRunScreen[]>> = {
  'login.provider.github': ['idle', 'failed', 'interrupted', 'timedOut'],
  'login.reopen': ['waiting'],
  'login.copyLink': ['waiting'],
  'login.cancel': ['waiting', 'timedOut'],
  'login.retry': ['timedOut'],
  'connecting.restart': ['crashed'],
  'connecting.openLogs': ['slow', 'crashed'],
  'connecting.report': ['crashed'],
};
```

If a `login.*`/`connecting.*` action has no table entry or its `screen` is absent/not in its allowed list, return false. For `connecting.restart`, also require `isSpawned === true`. Require actual connectivity for all provider/retry actions even if `screen === 'interrupted'`. Preserve overlay suppression by calling the existing `isActive` first.

In `useAppHotkeys`, select auth fields, `authChecked`, `syncStatus`, and `engineStatus`. Derive `authScreen` with `getAuthScreenState`. Derive the effective route/screen:

- terminal `engineStatus.state === 'crashed'` → route `'connecting'`, screen `'crashed'` on every URL;
- `!authChecked` (root/login or route pending) → route `'connecting'`, screen `'slow'` when starting elapsed ≥ `SLOW_ENGINE_MS`, else `'connecting'`;
- otherwise use `routeBaseFromPath(pathname)`, with `authScreen` for login.

Use a renderer timer in this hook while startup is pending so the L binding becomes enabled at the threshold even without a store push. Clean it up on ready/unmount; no duplicate registrations. Pass `screen`, `isSpawned`, and `isConnected` into **both** ordinary-hotkey and sequence enabled checks and add them to both memo dependency arrays. Update the activation context type for `isConnected?: boolean`. Hotkey callbacks retain the ref-based fresh store/context behavior.

Add this first-run gate before the existing early return for non-onboarding routes, keeping the sandbox branch unchanged:

```ts
if (binding.actionId.startsWith('login.') || binding.actionId.startsWith('connecting.')) {
  if (!isActive(binding, real)) return false;
  const allowed = FIRST_RUN_ACTION_STATES[binding.actionId];
  if (screen === undefined || allowed === undefined || !allowed.includes(screen)) return false;
  if (binding.actionId === 'connecting.restart' && isSpawned !== true) return false;
  if ((binding.actionId.startsWith('login.provider.') || binding.actionId === 'login.retry') && isConnected !== true) return false;
  return true;
}
```

In `useAppHotkeys`, use this concrete derivation (rename the existing path-derived variable to `pathRouteBase`):

```ts
const [now, setNow] = useState(Date.now);
useEffect(() => {
  if (authChecked || engineStatus?.state !== 'starting') return;
  const interval = window.setInterval(() => setNow(Date.now()), 1_000);
  return () => window.clearInterval(interval);
}, [authChecked, engineStatus?.state]);
const isConnected = isEngineConnected(syncStatus);
const authScreen = getAuthScreenState({ isConnected, authLoading, authUrl, authError, authTimedOut, authInterrupted });
const isCrashed = engineStatus?.state === 'crashed';
const routeBase = isCrashed || !authChecked ? 'connecting' : pathRouteBase;
const screen: FirstRunScreen = isCrashed ? 'crashed' : !authChecked
  ? engineStatus?.state === 'starting' && now - engineStatus.startedAt >= SLOW_ENGINE_MS ? 'slow' : 'connecting'
  : authScreen;
const isSpawned = engineStatus?.isSpawned === true;
```

Name the interval literal `TIMER_TICK_MS = 1_000` and use that constant. Include imports `useState`, `useEffect`, `SLOW_ENGINE_MS`, `FirstRunScreen`, `getAuthScreenState` and `isEngineConnected`. In both existing `isBindingEnabled` calls use `{routeBase, activeOverlay, sandboxPane, screen, isSpawned, isConnected}`. Both memo arrays and the current `routeBaseRef` must use this effective route. `HotkeyRegistrar` already lives in RootShell above Outlet and pending routes. Do not move it into any new screen or gate it on auth/WS.

- [ ] **Step 11: Verify and commit.**

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

Expected: selector table and existing keyboard tests pass; hooks remain registered before WS startup; unavailable provider overrides cannot fire; waiting and timeout actions do not collide with engine actions.

```bash
git add electron/src/renderer electron/tests/unit/authScreenState.test.ts
git commit -m "feat: model cancellable sign-in and first-run actions"
```

### Task 7: Slow and crashed engine surfaces [UI]

**Files:** Create `electron/src/renderer/hooks/useEngineScreenData.ts`, `electron/src/renderer/components/EngineCrashedScreen.tsx`; modify `electron/src/renderer/components/ConnectingScreen.tsx`, `electron/src/renderer/components/RootShell.tsx`.

**Interfaces:**
- Consumes: Tasks 5–6's `engineStatus`, `SLOW_ENGINE_MS`, `getEngineLogTail`, `restartEngine`, `openEngineLogFolder`, `reportEngineCrash`; `useEffectiveBindings(): KeyBinding[]` from `hooks/useEffectiveBindings.ts`, `findActionKeys(bindings: readonly KeyBinding[], actionIds: readonly string[]): string | undefined` from `utils/legendPresets.ts`, and `AuthLayout`.
- Produces: `useEngineScreenData(lines: number)` returning `{status, elapsed, isSlow, logs}`; `EngineCrashedScreen(): React.ReactElement`; slow ConnectingScreen; root-level terminal crash replacement. No new hotkey registrations.

- [ ] **Step 1: Add timer/log-fetch lifecycle to `useEngineScreenData`.**

Create the hook with the following code. Keep requests in services and state/effects here. Fetching the log while slow is allowed; engine **status** is never polled. One read fault is toasted and stops that interval, rather than generating one toast per second.

```ts
import { useEffect, useState } from 'react';
import { SLOW_ENGINE_MS, type EngineStatus } from '../../types/engine';
import { addToast } from '../components/ToastContainer';
import { getEngineLogTail } from '../services/engine';
import { appStore } from '../store';
import { useStore } from './useStore';

const TIMER_TICK_MS = 1_000;
const SECONDS_PER_MINUTE = 60;
interface EngineScreenData {
  readonly status: EngineStatus | null;
  readonly elapsed: string;
  readonly isSlow: boolean;
  readonly logs: readonly string[];
}

export function useEngineScreenData(lines: number): EngineScreenData {
  const status = useStore(appStore, (state) => state.engineStatus);
  const [now, setNow] = useState(Date.now);
  const [logs, setLogs] = useState<readonly string[]>([]);
  useEffect(() => {
    if (status?.state !== 'starting') return;
    const interval = window.setInterval(() => setNow(Date.now()), TIMER_TICK_MS);
    return () => window.clearInterval(interval);
  }, [status?.state]);
  const elapsedSeconds = Math.max(0, Math.floor((now - (status?.startedAt ?? now)) / TIMER_TICK_MS));
  const elapsed = `${String(Math.floor(elapsedSeconds / SECONDS_PER_MINUTE)).padStart(2, '0')}:${String(elapsedSeconds % SECONDS_PER_MINUTE).padStart(2, '0')}`;
  const isSlow = status?.state === 'starting' && now - status.startedAt >= SLOW_ENGINE_MS;
  useEffect(() => {
    if (!isSlow && status?.state !== 'crashed') return;
    let isActive = true;
    let hasFailed = false;
    const refresh = (): void => {
      if (hasFailed) return;
      void getEngineLogTail(lines).match(
        (tail) => { if (isActive) setLogs(tail); },
        (fault) => {
          if (!isActive || hasFailed) return;
          hasFailed = true;
          addToast({ type: 'error', message: fault.message, dismissible: true });
        },
      );
    };
    refresh();
    const interval = isSlow ? window.setInterval(refresh, TIMER_TICK_MS) : null;
    return () => { isActive = false; if (interval !== null) window.clearInterval(interval); };
  }, [isSlow, status?.state, lines]);
  return { status, elapsed, isSlow, logs };
}
```

Use `useEngineScreenData(3)` in ConnectingScreen and `useEngineScreenData(60)` in EngineCrashedScreen. Extract the effect body into a feature-local hook helper in the same file if formatting pushes the function far past 50 lines; no standalone formatting module.

- [ ] **Step 2: Extend ConnectingScreen after the existing initial presentation.**

Retain the current logo/connecting content when `!isSlow`. In the slow branch inside the same token-based layout, render:

```tsx
<p className="font-mono text-sm text-primary">CONNECTING… {elapsed}</p>
<p className="text-sm text-on-surface-variant">
  Taking longer than usual. This can happen after an update or on first launch.
</p>
<pre className="overflow-x-auto rounded-lg bg-surface-container p-4 font-mono text-xs text-on-surface-variant">
  {logs.join('\n') || 'Waiting for engine log output…'}
</pre>
<button type="button" onClick={openEngineLogFolder} className="rounded-lg border border-outline-variant px-4 py-2 text-sm text-on-surface">
  Open log folder
</button>
```

Attach the effective hint for `connecting.openLogs` using the existing binding-hint pattern in neighboring pages; do not write a literal `L`. Do not render a restart/report button in this branch.

- [ ] **Step 3: Create the crashed screen and its action row.**

Use `useEngineScreenData(60)` and the existing AuthLayout/title/brand primitives. Render the terminal exit, 60-line tail and hint exactly:

```tsx
<h1 className="text-2xl font-bold text-on-surface">The sync engine stopped</h1>
<p className="font-mono text-sm text-error">
  {status?.signal !== null && status?.signal !== undefined ? `Signal: ${status.signal}` : `Exit code: ${status?.code ?? 'unknown'}`}
</p>
<pre className="max-h-64 overflow-auto rounded-lg bg-surface-container p-4 font-mono text-xs text-on-surface-variant">
  {logs.join('\n') || 'No engine log output was captured.'}
</pre>
```

Action buttons call the services directly. Use an array of `{actionId, label, onClick}` records for the three actions, filtering restart unless `status?.isSpawned`. Render effective hints from the shared bindings. Report's description is always visible, not tooltip-only:

```tsx
<p className="text-xs text-on-surface-variant">
  Opens a public GitHub issue with the last 60 log lines. Check them before you submit.
</p>
```

Do not promise repo names/PR titles are redacted. Disable the restart button after starting until status changes; the main IPC terminal-state guard prevents repeated clicks/keys from spawning parallel engines.

- [ ] **Step 4: Replace Outlet only after retries are exhausted.**

In `RootShellInner`, select `engineStatus` and replace its current Outlet with:

```tsx
{engineStatus?.state === 'crashed' ? <EngineCrashedScreen /> : <Outlet />}
```

Keep `HotkeyRegistrar`, OverlayProvider, title bar and ToastContainer mounted. Ordinary `go:crashed` does not trigger this replacement. The terminal screen must replace inbox, settings, onboarding or login alike.

- [ ] **Step 5: Verify UI manually and run Electron checks.**

```bash
cd electron && pnpm start
```

For a slow-start check, use a temporary local debug delay before Go's ready signal while Electron **spawns** it, then remove that delay before commit. Do not use external-engine skip-spawn mode for this check: its existing bounded port wait can fail at 10 seconds, which would test crashed rather than slow. At ≥10 s verify elapsed time and last three lines, L works with no WS, and there is **no restart**. On normal startup the existing initial screen remains.

For a crash check, repeatedly kill the spawned engine after readiness until three automatic restarts are exhausted. Verify final screen replaces the current route, R resets the counter and reconnects, L opens the log directory, I opens an editable public issue with redacted log data. For an external engine, verify restart is hidden and its key is disabled.

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

No component tests or debug-delay code in the commit.

- [ ] **Step 6: Commit.**

```bash
git add electron/src/renderer/hooks/useEngineScreenData.ts electron/src/renderer/components/ConnectingScreen.tsx electron/src/renderer/components/EngineCrashedScreen.tsx electron/src/renderer/components/RootShell.tsx
git commit -m "feat: show slow startup and engine crash recovery"
```

### Task 8: Two-column sign-in and always-visible trust panel [UI]

**Files:** Create `electron/src/renderer/pages/login/TrustPanel.tsx`, `electron/src/renderer/pages/login/SignInState.tsx`, `electron/src/renderer/hooks/useSignInScreenData.ts`; modify `electron/src/renderer/pages/LoginPage.tsx`, `electron/src/renderer/layouts/AuthLayout.tsx`, `electron/src/renderer/components/ProviderButton.tsx`, `electron/src/main/preload.ts`, `electron/src/renderer/types/window.d.ts`.

**Interfaces:**
- Consumes: Task 6's selector/store/actions, `PROVIDERS.isAvailable`, Task 5's IPC-derived `appVersion`, optional `authExchange`; `useEffectiveBindings(): KeyBinding[]` and `findActionKeys(bindings: readonly KeyBinding[], actionIds: readonly string[]): string | undefined`; read-only `window.electron.platform` added here.
- Produces: `TrustPanel({exchange, isConnected, platform})`, `SignInState({screen, elapsed, error, isConnected, status, syncError, lastSyncAt, version})` with named readonly prop interfaces; `useSignInScreenData()` reactive view data; LoginPage's two-column layout. No direct WS calls or new hotkey registrations in these components.

- [ ] **Step 1: Extend AuthLayout without widening unrelated screens.**

Add `isWide?: boolean` to `AuthLayoutProps`, destructure `isWide = false` alongside `children`, and import `cn` from `../lib/utils`. Switch only the card width:

```tsx
className={cn('w-full bg-surface-container-lowest border border-outline-variant/40 shadow-ambient', isWide ? 'max-w-4xl' : 'max-w-md')}
```

The stacked trust panel can be taller than the window. For the outer AuthLayout element replace its alignment/overflow classes with:

```tsx
className={cn('relative w-full min-h-0 flex-1 flex justify-center px-6 bg-surface text-on-surface',
  isWide ? 'items-start overflow-y-auto py-6' : 'items-center overflow-hidden')}
```

Give the wide card `my-auto shrink-0` so it centers when it fits and remains reachable from the top when it does not. Retain existing crosshair decoration. LoginPage calls `<AuthLayout isWide>`; connecting/crashed screens keep the narrow default. No boolean positional function arguments.

- [ ] **Step 2: Add a hook for the sign-in timer and view data.**

The current preload has no platform field. Add `platform: process.platform` to its exposed object and `readonly platform: string` to both preload/window declarations in this task. Include those two files in this task's commit. Do not guess from user-agent.

In `useSignInScreenData.ts`, import `useEffect`, `useState`, `useStore`, `appStore`, `getAuthScreenState` and `isEngineConnected`. Add the actual view-data hook:

```ts
const TIMER_TICK_MS = 1_000;
const SECONDS_PER_MINUTE = 60;

interface SignInScreenData {
  readonly screen: AuthScreenState;
  readonly elapsed: string;
  readonly isConnected: boolean;
  readonly authLoading: boolean;
  readonly authError: string | null;
  readonly authExchange: 'proxy' | 'local' | null;
  readonly engineStatus: EngineStatus | null;
  readonly appVersion: string | null;
  readonly lastSyncAt: string | null;
  readonly syncError: string | null;
  readonly platform: string;
}

export function useSignInScreenData(): SignInScreenData {
  const authLoading = useStore(appStore, (state) => state.authLoading);
  const authUrl = useStore(appStore, (state) => state.authUrl);
  const authStartedAt = useStore(appStore, (state) => state.authStartedAt);
  const authError = useStore(appStore, (state) => state.authError);
  const authTimedOut = useStore(appStore, (state) => state.authTimedOut);
  const authInterrupted = useStore(appStore, (state) => state.authInterrupted);
  const authExchange = useStore(appStore, (state) => state.authExchange);
  const engineStatus = useStore(appStore, (state) => state.engineStatus);
  const appVersion = useStore(appStore, (state) => state.appVersion);
  const lastSyncAt = useStore(appStore, (state) => state.lastSyncAt);
  const syncError = useStore(appStore, (state) => state.syncError);
  const syncStatus = useStore(appStore, (state) => state.syncStatus);
  const [now, setNow] = useState(Date.now);
  const isConnected = isEngineConnected(syncStatus);
  const screen = getAuthScreenState({ isConnected, authLoading, authUrl, authError, authTimedOut, authInterrupted });
  useEffect(() => {
    if (screen !== 'waiting' || authStartedAt === null) return;
    const interval = window.setInterval(() => setNow(Date.now()), TIMER_TICK_MS);
    return () => window.clearInterval(interval);
  }, [screen, authStartedAt]);
  const seconds = authStartedAt === null ? 0 : Math.max(0, Math.floor((now - authStartedAt) / TIMER_TICK_MS));
  const elapsed = `${String(Math.floor(seconds / SECONDS_PER_MINUTE)).padStart(2, '0')}:${String(seconds % SECONDS_PER_MINUTE).padStart(2, '0')}`;
  return { screen, elapsed, isConnected, authLoading, authError, authExchange,
    engineStatus, appVersion, lastSyncAt, syncError, platform: window.electron?.platform ?? 'unknown' };
}
```

Give the exported hook an explicit `SignInScreenData` return interface containing those fields, using `AuthScreenState` and `EngineStatus | null` rather than redeclaring their unions. If a request has not returned its URL yet, it shows `00:00` without starting the auth timer. Timer starts at `AUTH_REDIRECT`, not click time.

- [ ] **Step 3: Implement the trust panel's exact content.**

Import React's type, `openUrlInBrowser` from `../../utils/openUrl` and `addToast` from `../../components/ToastContainer`. Use:

```ts
interface TrustPanelProps {
  readonly exchange: 'proxy' | 'local' | null;
  readonly isConnected: boolean;
  readonly platform: string;
}

function openSource(): void {
  void openUrlInBrowser('https://github.com/Thourum/Bistro').match(
    () => undefined,
    (fault) => addToast({ type: 'error', message: fault.message, dismissible: true }),
  );
}
```

Export `TrustPanel({exchange, isConnected, platform}: TrustPanelProps): React.ReactElement`, computing the following local values and returning the JSX below. Step 2 selects neutral while disconnected, even when an old exchange mode exists:

```ts
const EXCHANGE_COPY = {
  proxy: "Bistro's sign-in endpoint swaps it for a token and passes it straight back. It stores nothing.",
  local: 'Bistro swaps it for a token on this machine.',
} satisfies Record<'proxy' | 'local', string>;
const KEYRING_COPY: Readonly<Record<string, string>> = {
  darwin: 'macOS Keychain',
  win32: 'Windows Credential Manager',
  linux: 'the Secret Service keyring on Linux',
};
const exchangeStep = isConnected && exchange !== null ? EXCHANGE_COPY[exchange] : 'Bistro swaps it for a token.';
const keyring = KEYRING_COPY[platform] ?? "your operating system's keyring";
```

Render these sections using app tokens, no raw palette literals:

```tsx
<section className="space-y-6 bg-surface-container p-8 text-sm text-on-surface-variant">
  <div>
    <h2 className="font-bold text-on-surface">What GitHub will ask you to approve</h2>
    <dl className="mt-4 space-y-4">
      <div><dt className="font-mono text-primary">repo</dt><dd>GitHub labels this 'full control of private repositories'. Bistro needs it to read the private PRs you review and to post your reviews.</dd></div>
      <div><dt className="font-mono text-primary">read:user</dt><dd>Your username, so Bistro knows which review requests are yours.</dd></div>
    </dl>
  </div>
  <div><h2 className="font-bold text-on-surface">Where the token goes</h2>
    <ol className="mt-4 list-decimal space-y-3 pl-5">
      <li>GitHub returns a one-time authorization code.</li>
      <li>{exchangeStep}</li>
      <li>Bistro saves the token in {keyring}.</li>
    </ol>
  </div>
  <dl className="space-y-3">
    <div><dt className="font-bold text-on-surface">Telemetry</dt><dd>Off by default.</dd></div>
    <div><dt className="font-bold text-on-surface">Auto-update</dt><dd>Off by default.</dd></div>
    <div><dt className="font-bold text-on-surface">Licence</dt><dd>GPLv3 · <button type="button" className="text-primary underline" onClick={openSource}>read the source</button></dd></div>
    <div><dt className="font-bold text-on-surface">Revoke</dt><dd>GitHub → Settings → Applications</dd></div>
  </dl>
</section>
```

`openSource` uses `openUrlInBrowser('https://github.com/Thourum/Bistro').match(...)` at the UI boundary and a toast on failure. No inline raw Promise handling. Confirm the telemetry default in settings and updater's opt-in gate; do not imply the user cannot enable them.

- [ ] **Step 4: Let ProviderButton suppress key hints and show availability.**

Change its `keyHint` prop to optional and add `availabilityLabel?: string`. Render the hint container only when a non-empty key hint exists; render “Coming soon” instead for unavailable providers. Keep its native `disabled` attribute, with no click or keyboard action while disabled. Focus GitHub on the idle sign-in screen using the existing focus/ref pattern or a named `autoFocus?: boolean` prop on the native button.

- [ ] **Step 5: Implement the left-column state component.**

`SignInState` renders waiting and timedOut instead of hosts; idle/failed/interrupted/engineDown render hosts in LoginPage and this component's inline state area. Use actual state copy:

| State | Exact copy/actions |
|---|---|
| waiting | “Waiting for authorization… GITHUB // {elapsed}”; “Complete the sign-in in your browser, then return here.”; Reopen browser, Copy sign-in link, Cancel |
| timedOut | “No response after 5:00”; “The browser hasn't reported back, and Bistro stopped waiting. Start again to get a fresh sign-in.”; Try again, Back to hosts |
| failed | Inline `authError` text (React-escaped), hosts remain |
| interrupted | “Sign-in was interrupted because the sync engine restarted. Start again.”, hosts remain |
| disconnected | Alert “Cannot connect to sync engine. Bistro is restarting it.” and status block, hosts remain but disabled |

Render the disconnected block based on `!isConnected`, not just `screen === 'engineDown'`, so disconnect-while-waiting shows both the interrupted explanation and engine diagnostics. Terminal crashed status is replaced by RootShell and therefore never shows a false restart promise here. The status block labels are Engine, Error, Last sync, Version; values come from `engineStatus.state`, `syncError ?? engineStatus.error`, `lastSyncAt` and `appVersion`, with readable unavailable values. No hard-coded `v0.1.0-stable`.

Use service actions in a typed array and effective binding hints:

```ts
const WAITING_ACTIONS = [
  { actionId: 'login.reopen', label: 'Reopen browser', run: reopenSignIn },
  { actionId: 'login.copyLink', label: 'Copy sign-in link', run: copySignInLink },
  { actionId: 'login.cancel', label: 'Cancel', run: cancelSignIn },
] as const;
const TIMEOUT_ACTIONS = [
  { actionId: 'login.retry', label: 'Try again', run: retrySignIn },
  { actionId: 'login.cancel', label: 'Back to hosts', run: cancelSignIn },
] as const;
```

Implement the state area with this component, importing `React`, the shared `AuthScreenState`/`EngineStatus` types, `useEffectiveBindings`, `findActionKeys`, and the five sign-in services. Keep `WAITING_ACTIONS` and `TIMEOUT_ACTIONS` above it in the same file.

```tsx
interface SignInStateProps {
  readonly screen: AuthScreenState;
  readonly elapsed: string;
  readonly error: string | null;
  readonly isConnected: boolean;
  readonly status: EngineStatus | null;
  readonly syncError: string | null;
  readonly lastSyncAt: string | null;
  readonly version: string | null;
}

export function SignInState(props: SignInStateProps): React.ReactElement {
  const bindings = useEffectiveBindings();
  const actions = props.screen === 'waiting' ? WAITING_ACTIONS : props.screen === 'timedOut' ? TIMEOUT_ACTIONS : [];
  return <div className="space-y-4">
    {props.screen === 'waiting' && <div role="status" className="space-y-3">
      <p className="font-mono text-sm text-primary">Waiting for authorization… GITHUB // {props.elapsed}</p>
      <p className="text-sm text-on-surface-variant">Complete the sign-in in your browser, then return here.</p>
    </div>}
    {props.screen === 'timedOut' && <div role="status" className="space-y-3">
      <h2 className="font-bold text-on-surface">No response after 5:00</h2>
      <p className="text-sm text-on-surface-variant">The browser hasn't reported back, and Bistro stopped waiting. Start again to get a fresh sign-in.</p>
    </div>}
    {actions.length > 0 && <div className="flex flex-wrap gap-3">
      {actions.map((action) => {
        const hint = findActionKeys(bindings, [action.actionId]);
        return <button key={action.actionId} type="button" onClick={action.run}
          className="rounded-lg border border-outline-variant px-4 py-2 text-sm text-on-surface">
          {action.label}{hint !== undefined && <kbd className="ml-2 font-mono text-xs text-on-surface-variant">{hint}</kbd>}
        </button>;
      })}
    </div>}
    {props.screen === 'failed' && props.error !== null && <p role="alert" className="text-sm text-error">{props.error}</p>}
    {props.screen === 'interrupted' && <p role="alert" className="text-sm text-error">Sign-in was interrupted because the sync engine restarted. Start again.</p>}
    {!props.isConnected && <div role="alert" className="space-y-4 rounded-lg bg-surface-container p-4">
      <p className="text-sm text-error">Cannot connect to sync engine. Bistro is restarting it.</p>
      <dl className="space-y-2 font-mono text-xs text-on-surface-variant">
        <div><dt>Engine</dt><dd>{props.status?.state ?? 'Unavailable'}</dd></div>
        <div><dt>Error</dt><dd>{props.syncError ?? props.status?.error ?? 'No error detail available.'}</dd></div>
        <div><dt>Last sync</dt><dd>{props.lastSyncAt ?? 'Not yet synced'}</dd></div>
        <div><dt>Version</dt><dd>{props.version ?? 'Unavailable'}</dd></div>
      </dl>
    </div>}
  </div>;
}
```

The GitHub provider key G also retries from timedOut through the catalog; Enter is the visible Try again action. Do not recreate timers or auth transitions inside this component.

- [ ] **Step 6: Assemble LoginPage around the services and two-column grid.**

Delete its duplicated `handleProviderClick`/direct `AUTH_START`, browser-wait implementation and hard-coded version footer. Preserve routing/auth guard ownership. Render:

```tsx
<AuthLayout isWide>
  <div className="grid md:grid-cols-2">
    <div className="space-y-6 p-8">
      <p className="text-4xl font-extrabold uppercase tracking-tighter text-on-background">BISTRO</p>
      <h1 className="text-2xl font-bold text-on-surface">Connect your platform to begin reviewing code</h1>
      {screen !== 'waiting' && screen !== 'timedOut' && (
        <div className="space-y-3">
          {PROVIDERS.map((provider) => (
            <ProviderButton key={provider.id} provider={provider.id} icon={provider.icon}
              label={provider.label} disabled={!provider.isAvailable || !isConnected || authLoading}
              keyHint={provider.isAvailable ? githubHint : undefined}
              availabilityLabel={provider.isAvailable ? undefined : 'Coming soon'}
              onClick={() => startSignIn(provider.id)} />
          ))}
        </div>
      )}
      <SignInState screen={screen} elapsed={elapsed} error={authError} isConnected={isConnected}
        status={engineStatus} syncError={syncError} lastSyncAt={lastSyncAt} version={appVersion} />
      <p className="text-xs text-on-surface-variant">Sign-in opens GitHub in your browser. Bistro never sees your password.</p>
    </div>
    <TrustPanel exchange={authExchange} isConnected={isConnected} platform={platform} />
  </div>
</AuthLayout>
```

This reuses LoginPage's current BISTRO wordmark; no new logo wrapper is needed. Import `useEffectiveBindings` and `findActionKeys` from their existing modules, then define:

```ts
const bindings = useEffectiveBindings();
const githubHint = findActionKeys(bindings, ['login.provider.github']);
```

Use the same two existing helpers in ConnectingScreen, EngineCrashedScreen and SignInState; `findActionKeys(bindings, [actionId])` returns `undefined` for an unbound action so its key chip is omitted. Keep the trust panel outside every state condition and second in DOM order. GitHub focus applies only on the host screen, not after every timer tick.

- [ ] **Step 7: Complete the end-to-end manual matrix.**

```bash
cd electron && pnpm start
```

Check idle/waiting/timedOut/failed/interrupted/engineDown at desktop and below `md`; the trust panel stays visible and mode-neutral before auth-check or while disconnected. Verify GitLab/Gitea ignore clicks and have no hint; G starts GitHub; R reopens the identical saved URL; C copies it; Escape cancels and a new G starts immediately; Enter/G retry timeout; Escape timeout returns to hosts. Change a key override before WS readiness and verify screen hints/actions follow it.

For timeout, run the engine standalone with the new flag:

```bash
cd go && go run ./cmd/bistro-sync -auth-callback-timeout=5s
```

Launch Electron against that engine using its existing `BISTRO_SKIP_GO_SPAWN=1`/WS-port configuration. Start sign-in without authorizing, verify timedOut and that retry gets a fresh URL. The screen's “5:00” copy remains the production copy during this test-only shortened duration. Cancel/timeout late browser callbacks correctly receive the browser connection error because the listener is gone.

Deny authorization at GitHub and inspect the branded escaped failure page; authorize successfully and inspect success copy (correct OS store and exchange mode, no sync-start claim). Disconnect while waiting: saved URL disappears, interrupted message appears, engine-down diagnostics exist only while disconnected, and reconnect does not silently resume the dead sign-in. Kill the engine through all retries and verify terminal screen supersedes sign-in.

- [ ] **Step 8: Run all authoritative checks and inspect the final diff.**

```bash
cd go && go vet ./... && go test -race ./...
cd ../electron && pnpm type-check && pnpm lint && pnpm test
```

```bash
git diff --check
git diff --stat
```

Expected: all checks pass; four focused behavior regressions are present, no component suites, no credentials or temporary debug delays, and no out-of-scope sync changes. Review all changed code before committing; use the appropriate code/security reviewer for OAuth, main IPC and input-validation changes, without model overrides.

- [ ] **Step 9: Commit.**

```bash
git add electron/src/renderer/pages/LoginPage.tsx electron/src/renderer/pages/login/TrustPanel.tsx electron/src/renderer/pages/login/SignInState.tsx electron/src/renderer/hooks/useSignInScreenData.ts electron/src/renderer/layouts/AuthLayout.tsx electron/src/renderer/components/ProviderButton.tsx electron/src/main/preload.ts electron/src/renderer/types/window.d.ts
git commit -m "feat: explain GitHub sign-in and trust at first run"
```

## Execution notes

- Each task inherits Global Constraints. Read both this plan and the spec before implementation; code snippets are scoped additions/replacements, not permission to erase unrelated code. Run each task's checks and code/security review gate before its commit; if checks fail, report the exact failure and fix task-local defects before proceeding.
- The callback port is fixed, so tests that actually start a callback server must run serially and release it. Handler cleanup must only clear the server it owns; a completed old flow must never close/clear a new one.
- `AUTH_CANCEL` returns `nil, nil` because the WS dispatcher sends no response for nil; cancellation completion is the `AUTH_FAILED cancelled` broadcast.
- Startup exits are terminal today, unlike post-ready exits. `onGiveUp` covers both startup failure and exhausted automatic restarts.
- `engine:get-status` and `app:get-version` are additional snapshot channels needed to survive pushes before window creation; the status push remains the state-change source.
- Settings already load from main IPC and RootShell already owns hotkeys. Preserve these independent lifetimes; no WS readiness dependency for recovery keys.
- The connecting route base is screen-derived (including pending root/deep-link routes), not a new navigable URL. Keep `routeBaseFromPath`'s existing route matching for normal screens.
- No generic retry framework, new dependencies, settings redesign, documentation regeneration or sync orchestration is needed.
