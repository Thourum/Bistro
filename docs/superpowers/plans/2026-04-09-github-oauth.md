# GitHub OAuth Device Flow — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement real GitHub OAuth device code flow end-to-end: Go handles the HTTP calls and token storage, WebSocket messages coordinate the flow, and Electron shows the device code UI and reacts to auth success/failure.

**Architecture:**
1. Electron `LoginPage` sends `AUTH_START` via WebSocket with `{ provider: "github" }`
2. Go receives `AUTH_START`, calls GitHub device code endpoint, sends `AUTH_DEVICE_CODE` back with `{ user_code, verification_uri }`
3. Electron displays the code and opens the browser
4. Go polls GitHub for token completion, stores in OS keychain
5. Go sends `AUTH_SUCCESS` back, Electron sets `isAuthenticated: true`
6. On startup, Go checks keychain for existing token — if found, skips device flow

**Tech Stack:** Go (net/http, go-keyring), Electron IPC, WebSocket, Vitest

---

## File Structure

| File | Responsibility |
|---|---|
| **Modify:** `go/internal/provider/github/auth.go` | Real HTTP calls to GitHub device code + polling endpoints |
| **Modify:** `go/internal/ws/server.go` | Add inbound message routing + AUTH_START handler |
| **Modify:** `go/internal/server/app.go` | Wire auth flow, call SetProvider on success |
| **Modify:** `go/cmd/bistro-sync/main.go` | Read GITHUB_CLIENT_ID from env, check keychain on startup, call SetProvider |
| **Modify:** `electron/src/renderer/pages/LoginPage.tsx` | Show device code UI, listen for AUTH_SUCCESS |
| **Modify:** `electron/src/renderer/hooks/useWebSocket.ts` | Handle AUTH_DEVICE_CODE and AUTH_SUCCESS events |
| **Modify:** `electron/src/renderer/store/ui.ts` | Add deviceCode/verificationUri fields, persist isAuthenticated |
| **Create:** `go/internal/ws/dispatch.go` | Message routing/dispatch for inbound WebSocket messages |
| **Create:** `electron/tests/unit/LoginPage-oauth.test.tsx` | Test device code display and auth flow |

---

### Task 1: Go — Real Device Code HTTP Call

**Files:**
- Modify: `go/internal/provider/github/auth.go`
- Test: `go/internal/provider/github/auth_test.go`

- [ ] **Step 1: Write failing test**

Create or update `go/internal/provider/github/auth_test.go`:

```go
func TestGetDeviceCode_ReturnsNonEmpty(t *testing.T) {
    // This test validates the shape of the request, not the actual API call.
    // Use httptest to mock the GitHub endpoint.
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/login/device/code" {
            t.Errorf("unexpected path: %s", r.URL.Path)
        }
        if r.Method != http.MethodPost {
            t.Errorf("unexpected method: %s", r.Method)
        }
        body, _ := io.ReadAll(r.Body)
        if !strings.Contains(string(body), "client_id=") {
            t.Error("missing client_id in body")
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "device_code":      "dc-test-123",
            "user_code":        "ABCD-1234",
            "verification_uri": "https://github.com/login/device",
            "expires_in":       900,
            "interval":         5,
        })
    }))
    defer server.Close()

    auth := NewAuthenticator("test-client-id")
    auth.baseURL = server.URL // Allow overriding base URL for tests

    code, userCode, uri, interval, err := auth.GetDeviceCode(context.Background())
    require.NoError(t, err)
    assert.Equal(t, "dc-test-123", code)
    assert.Equal(t, "ABCD-1234", userCode)
    assert.Equal(t, "https://github.com/login/device", uri)
    assert.Equal(t, 5, interval)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/provider/github/ -run TestGetDeviceCode -v`
Expected: FAIL — stub returns hardcoded values

- [ ] **Step 3: Implement real GetDeviceCode**

Replace the stub in `auth.go`:

```go
type DeviceCodeResponse struct {
    DeviceCode      string `json:"device_code"`
    UserCode        string `json:"user_code"`
    VerificationURI string `json:"verification_uri"`
    ExpiresIn       int    `json:"expires_in"`
    Interval        int    `json:"interval"`
}

func (a *Authenticator) GetDeviceCode(ctx context.Context) (deviceCode, userCode, verificationURI string, interval int, err error) {
    url := a.baseURL + "/login/device/code"

    data := fmt.Sprintf("client_id=%s&scope=repo", a.clientID)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(data))
    if err != nil {
        return "", "", "", 0, fmt.Errorf("creating request: %w", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Accept", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", "", "", 0, fmt.Errorf("device code request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return "", "", "", 0, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, string(body))
    }

    var dcResp DeviceCodeResponse
    if err := json.NewDecoder(resp.Body).Decode(&dcResp); err != nil {
        return "", "", "", 0, fmt.Errorf("decoding response: %w", err)
    }

    return dcResp.DeviceCode, dcResp.UserCode, dcResp.VerificationURI, dcResp.Interval, nil
}
```

The `Authenticator` struct needs a `baseURL` field (default `"https://github.com"`):

```go
type Authenticator struct {
    clientID string
    baseURL  string
}

func NewAuthenticator(clientID string) *Authenticator {
    return &Authenticator{clientID: clientID, baseURL: "https://github.com"}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go && go test ./internal/provider/github/ -run TestGetDeviceCode -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/internal/provider/github/auth.go go/internal/provider/github/auth_test.go
git commit -m "feat: implement real GitHub device code HTTP call in auth.go"
```

---

### Task 2: Go — Token Polling

**Files:**
- Modify: `go/internal/provider/github/auth.go`
- Test: `go/internal/provider/github/auth_test.go`

- [ ] **Step 1: Write failing test for PollForToken**

```go
func TestPollForToken_Success(t *testing.T) {
    callCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        w.Header().Set("Content-Type", "application/json")
        if callCount < 3 {
            // First 2 calls: authorization_pending
            json.NewEncoder(w).Encode(map[string]string{
                "error": "authorization_pending",
            })
            return
        }
        // 3rd call: success
        json.NewEncoder(w).Encode(map[string]string{
            "access_token": "gho_test_token_123",
            "token_type":   "bearer",
            "scope":        "repo",
        })
    }))
    defer server.Close()

    auth := NewAuthenticator("test-client-id")
    auth.baseURL = server.URL

    token, err := auth.PollForToken(context.Background(), "dc-test-123", 1) // 1 second interval for fast test
    require.NoError(t, err)
    assert.Equal(t, "gho_test_token_123", token)
    assert.GreaterOrEqual(t, callCount, 3)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/provider/github/ -run TestPollForToken -v`

- [ ] **Step 3: Implement PollForToken**

```go
type TokenResponse struct {
    AccessToken string `json:"access_token"`
    TokenType   string `json:"token_type"`
    Scope       string `json:"scope"`
    Error       string `json:"error"`
}

func (a *Authenticator) PollForToken(ctx context.Context, deviceCode string, interval int) (string, error) {
    url := a.baseURL + "/login/oauth/access_token"

    for {
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        case <-time.After(time.Duration(interval) * time.Second):
        }

        data := fmt.Sprintf("client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code", a.clientID, deviceCode)
        req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(data))
        if err != nil {
            return "", fmt.Errorf("creating poll request: %w", err)
        }
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        req.Header.Set("Accept", "application/json")

        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            return "", fmt.Errorf("poll request: %w", err)
        }

        var tokenResp TokenResponse
        if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
            resp.Body.Close()
            return "", fmt.Errorf("decoding poll response: %w", err)
        }
        resp.Body.Close()

        switch tokenResp.Error {
        case "":
            // Success
            return tokenResp.AccessToken, nil
        case "authorization_pending":
            continue
        case "slow_down":
            interval += 5
            continue
        case "expired_token":
            return "", fmt.Errorf("device code expired — please restart the login flow")
        case "access_denied":
            return "", fmt.Errorf("access denied by user")
        default:
            return "", fmt.Errorf("unexpected error: %s", tokenResp.Error)
        }
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go && go test ./internal/provider/github/ -run TestPollForToken -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/internal/provider/github/auth.go go/internal/provider/github/auth_test.go
git commit -m "feat: implement GitHub OAuth token polling with exponential backoff"
```

---

### Task 3: Go — WebSocket Message Dispatch

**Files:**
- Create: `go/internal/ws/dispatch.go`
- Modify: `go/internal/ws/server.go`

- [ ] **Step 1: Create message dispatch infrastructure**

Create `go/internal/ws/dispatch.go`:

```go
package ws

import (
    "encoding/json"
    "fmt"
    "log/slog"
)

// InboundMessage represents a message received from Electron.
type InboundMessage struct {
    ID        string          `json:"id"`
    Type      string          `json:"type"`
    Data      json.RawMessage `json:"data"`
    Timestamp string          `json:"timestamp"`
}

// MessageHandler processes an inbound WebSocket message and returns an optional response.
type MessageHandler func(msg InboundMessage) (interface{}, error)

// Dispatcher routes inbound WebSocket messages to registered handlers.
type Dispatcher struct {
    handlers map[string]MessageHandler
    logger   *slog.Logger
}

func NewDispatcher(logger *slog.Logger) *Dispatcher {
    return &Dispatcher{
        handlers: make(map[string]MessageHandler),
        logger:   logger,
    }
}

func (d *Dispatcher) Register(messageType string, handler MessageHandler) {
    d.handlers[messageType] = handler
}

func (d *Dispatcher) Dispatch(raw []byte) (interface{}, error) {
    var msg InboundMessage
    if err := json.Unmarshal(raw, &msg); err != nil {
        return nil, fmt.Errorf("invalid message format: %w", err)
    }

    handler, ok := d.handlers[msg.Type]
    if !ok {
        d.logger.Warn("unhandled message type", "type", msg.Type)
        return nil, nil
    }

    d.logger.Info("dispatching message", "type", msg.Type, "id", msg.ID)
    return handler(msg)
}
```

- [ ] **Step 2: Wire dispatcher into ws/server.go handleWebSocket**

In the message read loop, replace the existing logic with:

```go
_, raw, err := conn.ReadMessage()
if err != nil {
    break
}
response, err := s.dispatcher.Dispatch(raw)
if err != nil {
    s.logger.Error("dispatch error", "error", err)
    continue
}
if response != nil {
    responseBytes, _ := json.Marshal(response)
    conn.WriteMessage(websocket.TextMessage, responseBytes)
}
```

- [ ] **Step 3: Run Go tests**

Run: `cd go && go test ./internal/ws/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/internal/ws/dispatch.go go/internal/ws/server.go
git commit -m "feat: add WebSocket message dispatcher with handler registration"
```

---

### Task 4: Go — AUTH_START Handler + Keychain Integration

**Files:**
- Modify: `go/internal/server/app.go`
- Modify: `go/cmd/bistro-sync/main.go`

- [ ] **Step 1: Register AUTH_START handler in app.go**

In `app.go`, add a method that handles `AUTH_START`:

```go
func (a *App) handleAuthStart(msg ws.InboundMessage) (interface{}, error) {
    var data struct {
        Provider string `json:"provider"`
    }
    if err := json.Unmarshal(msg.Data, &data); err != nil {
        return nil, fmt.Errorf("invalid AUTH_START data: %w", err)
    }

    if data.Provider != "github" {
        return map[string]interface{}{
            "id": msg.ID, "type": "AUTH_FAILED",
            "data": map[string]string{"error": "unsupported provider"},
        }, nil
    }

    // Start device flow in background
    go a.runDeviceFlow(msg.ID)
    return nil, nil
}

func (a *App) runDeviceFlow(requestID string) {
    ctx := context.Background()
    auth := github.NewAuthenticator(a.githubClientID)

    deviceCode, userCode, verificationURI, interval, err := auth.GetDeviceCode(ctx)
    if err != nil {
        a.ws.Broadcast(map[string]interface{}{
            "type": "AUTH_FAILED", "data": map[string]string{"error": err.Error()},
        })
        return
    }

    // Send device code to Electron
    a.ws.Broadcast(map[string]interface{}{
        "type": "AUTH_DEVICE_CODE",
        "data": map[string]interface{}{
            "user_code":        userCode,
            "verification_uri": verificationURI,
        },
    })

    // Poll for token
    token, err := auth.PollForToken(ctx, deviceCode, interval)
    if err != nil {
        a.ws.Broadcast(map[string]interface{}{
            "type": "AUTH_FAILED", "data": map[string]string{"error": err.Error()},
        })
        return
    }

    // Store token in keychain
    tm := keychain.NewTokenManager()
    if err := tm.StoreGitHubToken(token); err != nil {
        a.logger.Error("failed to store token in keychain", "error", err)
    }

    // Wire up GitHub provider
    client := github.NewClient(token)
    a.SetProvider(client)

    // Notify Electron
    a.ws.Broadcast(map[string]interface{}{
        "type": "AUTH_SUCCESS",
        "data": map[string]string{"provider": "github"},
    })
}
```

- [ ] **Step 2: Update main.go to check keychain on startup**

In `main.go`, before starting the server:

```go
    // Check keychain for existing GitHub token
    clientID := os.Getenv("GITHUB_CLIENT_ID")
    if clientID == "" {
        clientID = "Ov23liXXXXXXXXXXXXXX" // Default public OAuth app client ID
    }

    tm := keychain.NewTokenManager()
    existingToken, err := tm.GetGitHubToken()
    if err == nil && existingToken != "" {
        logger.Info("Found existing GitHub token in keychain")
        client := github.NewClient(existingToken)
        app.SetProvider(client)
    } else {
        logger.Info("No GitHub token found — waiting for OAuth flow")
    }
```

- [ ] **Step 3: Run Go tests**

Run: `cd go && go test ./... 2>&1 | tail -20`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add go/internal/server/app.go go/cmd/bistro-sync/main.go
git commit -m "feat: wire AUTH_START handler with device flow and keychain storage"
```

---

### Task 5: Electron — Auth WebSocket Events + LoginPage UI

**Files:**
- Modify: `electron/src/renderer/hooks/useWebSocket.ts`
- Modify: `electron/src/renderer/store/ui.ts`
- Modify: `electron/src/renderer/pages/LoginPage.tsx`

- [ ] **Step 1: Add auth fields to store**

In `ui.ts`, add to UIState:

```typescript
  deviceCode: string | null;
  verificationUri: string | null;
  authLoading: boolean;
  authError: string | null;
```

Initialize all to `null`/`false`.

- [ ] **Step 2: Add WebSocket handlers for AUTH_DEVICE_CODE and AUTH_SUCCESS**

In `useWebSocket.ts`, add to the switch:

```typescript
          case 'AUTH_DEVICE_CODE': {
            const payload = message.data as { user_code: string; verification_uri: string };
            try {
              appStore.setState((s) => ({
                ...s,
                deviceCode: payload.user_code,
                verificationUri: payload.verification_uri,
                authLoading: true,
                authError: null,
              }));
            } catch { /* test env */ }
            break;
          }
          case 'AUTH_SUCCESS': {
            try {
              appStore.setState((s) => ({
                ...s,
                isAuthenticated: true,
                authLoading: false,
                deviceCode: null,
                verificationUri: null,
                authError: null,
              }));
            } catch { /* test env */ }
            break;
          }
          case 'AUTH_FAILED': {
            const payload = message.data as { error: string };
            try {
              appStore.setState((s) => ({
                ...s,
                authLoading: false,
                authError: payload.error,
                deviceCode: null,
                verificationUri: null,
              }));
            } catch { /* test env */ }
            break;
          }
```

- [ ] **Step 3: Update LoginPage to show device code flow**

Replace the optimistic auth logic in LoginPage with:

```tsx
// Read auth state from store
const deviceCode = useStore(appStore, (s) => s.deviceCode);
const verificationUri = useStore(appStore, (s) => s.verificationUri);
const authLoading = useStore(appStore, (s) => s.authLoading);
const authError = useStore(appStore, (s) => s.authError);

// When device code is available, show it
{deviceCode !== null && verificationUri !== null && (
  <div className="text-center space-y-4">
    <p className="text-sm text-on-surface-variant">
      Enter this code at GitHub:
    </p>
    <p className="text-3xl font-mono font-bold text-primary tracking-widest">
      {deviceCode}
    </p>
    <a
      href={verificationUri}
      target="_blank"
      rel="noopener noreferrer"
      className="text-sm text-primary hover:underline"
    >
      Open {verificationUri}
    </a>
    <p className="text-xs text-on-surface-variant animate-pulse">
      Waiting for authorization...
    </p>
  </div>
)}

{authError !== null && (
  <p className="text-sm text-error">{authError}</p>
)}
```

Remove the optimistic `isAuthenticated: true` set — the real flow handles it.

- [ ] **Step 4: Run Electron tests**

Run: `cd electron && pnpm test`
Expected: All tests pass (may need to update LoginPage tests)

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/hooks/useWebSocket.ts electron/src/renderer/store/ui.ts electron/src/renderer/pages/LoginPage.tsx
git commit -m "feat: implement real OAuth device code flow in Electron UI"
```

---

### Task 6: Build Verification

- [ ] **Step 1: Run Go tests**

Run: `cd go && go test ./...`
Expected: All pass

- [ ] **Step 2: Run Electron tests + build**

Run: `cd electron && pnpm test && pnpm build`
Expected: All pass, clean build

- [ ] **Step 3: Commit any remaining changes**

```bash
git add -A && git commit -m "feat: GitHub OAuth device flow — build verification"
```
