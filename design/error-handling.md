# Bistro Error Handling Strategy

**Purpose:** Define how errors are detected, logged, and presented to users across Go and Electron.

**Audience:** Go team (primary), Electron team (secondary), maintainers debugging issues.

---

## Error Categories

| Category | Source | User Sees | Go Handles |
|----------|--------|-----------|-----------|
| **Network** | GitHub/GitLab API unreachable | "Can't reach GitHub" | Retry + exponential backoff |
| **Auth** | Token expired, revoked, insufficient scope | "Re-authenticate" | Token refresh or forced login |
| **Rate Limit** | GitHub API quota exceeded | "Too many requests, try again later" | Queue action, backoff |
| **Validation** | Malformed input from user | "Invalid input: ..." | Reject immediately |
| **Internal** | Bug in Go or DB | "Unexpected error" + error code | Log full details, retry once |
| **Offline** | No network connection | "You're offline" | Queue for later |

---

## Go Error Handling (Server-Side)

### Error Return Pattern

All Go functions that can fail return `(result, error)`:

```go
// Good
func (g *Engine) ApprovePR(ctx context.Context, prID string) (bool, error) {
    if prID == "" {
        return false, fmt.Errorf("pr_id required")
    }
    // ... implementation
    return true, nil
}

// Electron calls: GetPRApprovalStatus(id string) → (status string, code string, message string)
```

### Error Wrapping (Context)

Wrap errors with context, never lose the original:

```go
import "fmt"

// Always wrap, never lose detail
resp, err := github.GetPR(ctx, repo, number)
if err != nil {
    return fmt.Errorf("fetch PR from GitHub: %w", err)
}

// Don't do this (loses context):
// return err
```

### Error Codes (For IPC)

Every error has a code that Electron can act on:

```go
type ErrorCode string

const (
    // Network
    ErrNetworkTimeout ErrorCode = "NETWORK_TIMEOUT"
    ErrNetworkUnreachable ErrorCode = "NETWORK_UNREACHABLE"

    // Auth
    ErrAuthTokenExpired ErrorCode = "AUTH_TOKEN_EXPIRED"
    ErrAuthInsufficientScope ErrorCode = "AUTH_INSUFFICIENT_SCOPE"
    ErrAuthNotAuthenticated ErrorCode = "AUTH_NOT_AUTHENTICATED"

    // GitHub API
    ErrRateLimited ErrorCode = "RATE_LIMITED"
    ErrNotFound ErrorCode = "NOT_FOUND"
    ErrForbidden ErrorCode = "FORBIDDEN"

    // Validation
    ErrInvalidInput ErrorCode = "INVALID_INPUT"

    // Internal
    ErrInternal ErrorCode = "INTERNAL"
    ErrDatabaseError ErrorCode = "DATABASE_ERROR"
)

// IPC Response
type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   *ErrorInfo  `json:"error,omitempty"`
}

type ErrorInfo struct {
    Code    ErrorCode `json:"code"`
    Message string    `json:"message"`
    Hint    string    `json:"hint,omitempty"`
}
```

### Retry Strategy

| Error | Retry? | Max Attempts | Backoff |
|-------|--------|--------------|---------|
| Network timeout | Yes | 3 | Exponential 1s, 2s, 4s |
| Rate limited (429) | Yes | Unlimited | Until `Retry-After` header |
| Auth failed | No | - | Require manual re-auth |
| Not found (404) | No | - | PR deleted by user |
| Malformed input | No | - | User must fix |
| Internal error | Yes | 1 | Immediate retry once |

**Example:**

```go
func (g *Engine) ApprovePRWithRetry(ctx context.Context, prID string) error {
    backoff := 1 * time.Second
    maxRetries := 3

    for attempt := 0; attempt <= maxRetries; attempt++ {
        err := g.ApprovePR(ctx, prID)
        if err == nil {
            return nil
        }

        // Check if retryable
        if !isRetryable(err) {
            return err
        }

        if attempt < maxRetries {
            select {
            case <-time.After(backoff):
                backoff *= 2
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }

    return fmt.Errorf("approval failed after %d attempts", maxRetries)
}
```

### Logging (Server-Side)

**What to log:**

```go
// Always log
- Request start/finish with ID
- Errors with full context (function, error code, stack trace)
- API calls: endpoint, method, response status
- User actions: approve, comment, with result

Example log:
{
  "timestamp": "2025-04-08T14:32:00Z",
  "level": "ERROR",
  "function": "ApprovePR",
  "pr_id": "123",
  "error_code": "RATE_LIMITED",
  "error": "GitHub API rate limit exceeded",
  "retry_after_seconds": 60,
  "request_id": "req-abc123"
}
```

**What NOT to log:**

```go
// Never log
- Full OAuth tokens (log first 8 chars only)
- User email addresses (use anonymized ID)
- Full request/response bodies (log first 500 chars)
- Password hashes or secrets
```

### Rate Limit Handling

GitHub API returns `X-RateLimit-Remaining` and `X-RateLimit-Reset` headers:

```go
// Check before making request
if g.rateLimitRemaining < 10 {
    waitUntil := time.Unix(g.rateLimitReset, 0)
    return fmt.Errorf("rate limited until %v", waitUntil)
}

// After request
if resp.StatusCode == 429 {
    resetTime := resp.Header.Get("X-RateLimit-Reset")
    // Convert to Unix timestamp
    g.rateLimitReset = parseUnixTimestamp(resetTime)
    return &ErrorInfo{
        Code: ErrRateLimited,
        Hint: fmt.Sprintf("Try again in %d seconds", g.rateLimitReset - now),
    }
}
```

---

## Electron Error Handling (Client-Side)

### Result Type Pattern (hand-rolled, defined in `electron/src/main/types.ts`)

All async operations return `Result<T, E>`:

```typescript
// Result type definition (electron/src/main/types.ts)
type Result<T, E> =
    | { success: true; data: T }
    | { success: false; error: E }

// Signature
async function approvePR(prID: string): Promise<Result<boolean, ErrorInfo>>

// Usage
const result = await approvePR('123')

if (!result.success) {
    // Handle error
    handleError(result.error)
} else {
    // Use success value
    const approved = result.data
    console.log('Approved:', approved)
}
```

### Error Handling at UI Boundaries

**Rule:** Only catch errors at UI layer, never in business logic.

```typescript
// Good: business logic throws
export function filterPRs(prs: PR[]): PR[] {
    return prs.filter(pr => pr.state === 'open')
}

// Good: UI layer catches
export function displayPRs(prs: PR[]) {
    try {
        const filtered = filterPRs(prs)
        render(filtered)
    } catch (error) {
        showErrorModal('Failed to display PRs')
    }
}

// Bad: business logic catches
export function filterPRs(prs: PR[]): PR[] {
    try {
        return prs.filter(pr => pr.state === 'open')
    } catch (error) {
        // Why catch here?
    }
}
```

### User-Facing Error Messages

**Pattern:**

```
Problem: "This PR couldn't be approved"
Reason: "GitHub API is temporarily unavailable"
Action: "[Try again]"
```

**Examples:**

| Go Error | User Sees |
|----------|-----------|
| NETWORK_TIMEOUT | "Couldn't reach GitHub (timeout). Check your internet." |
| AUTH_TOKEN_EXPIRED | "Your login expired. Re-authenticate via settings." |
| RATE_LIMITED | "Too many requests. Try again in 60 seconds." |
| NOT_FOUND | "This PR was deleted or moved." |
| INVALID_INPUT | "Invalid input: PR number required." |
| INTERNAL | "Unexpected error (code: INT-5f2a). Contact support." |

**DO NOT show:**

```
❌ Stack traces
❌ JSON payloads
❌ Internal IP addresses
❌ Full file paths
❌ Database queries
```

**DO show:**

```
✓ What happened (past tense)
✓ Why it happened (user-friendly)
✓ What to do next (action)
✓ Support link if unclear
```

### IPC Error Response Handling

When Go returns an error via IPC:

```typescript
interface ErrorInfo {
    code: ErrorCode
    message: string // User-friendly
    hint?: string   // Developer hint
}

function handleIpcError(error: ErrorInfo): void {
    switch (error.code) {
        case 'NETWORK_TIMEOUT':
            showNotification('Timeout. Retrying...')
            retry()
            break

        case 'AUTH_TOKEN_EXPIRED':
            showModal('Please re-authenticate')
            navigateTo('/login')
            break

        case 'RATE_LIMITED':
            showNotification(error.hint) // "Try again in 60s"
            disableActionsUntil(Date.now() + 60000)
            break

        case 'INTERNAL':
            showErrorModal(
                `Unexpected error: ${error.message}`,
                `Report code: ${error.code}`
            )
            break

        default:
            showNotification(error.message)
    }
}
```

---

## Recovery Paths

### Network Error → Offline Mode

```
User tries to approve PR
    ↓
Go sends to GitHub, times out (5s)
    ↓
Go returns: { code: "NETWORK_TIMEOUT", message: "..." }
    ↓
Electron shows: "⚠ Can't reach GitHub. Using cached data."
    ↓
If Phase 2 action queue enabled:
  - Store action locally
  - Show: "Queued for sync"
  - Retry when online
Otherwise:
  - Show: "Try again when online"
```

### Auth Expired → Force Re-Login

```
Go detects: Token returns 401 Unauthorized
    ↓
Go returns: { code: "AUTH_TOKEN_EXPIRED", message: "..." }
    ↓
Electron shows modal: "Your login expired. Re-authenticate?"
    ↓
User clicks [Re-authenticate]
    ↓
Open OAuth flow (GitHub / GitLab)
    ↓
New token stored in OS keychain
    ↓
Resume operation
```

### Rate Limited → Backoff

```
Go hits GitHub API rate limit (429)
    ↓
Go reads X-RateLimit-Reset header: 1712599920 (Unix timestamp)
    ↓
Go returns: {
    code: "RATE_LIMITED",
    message: "Too many requests",
    hint: "Try again in 60 seconds"
}
    ↓
Electron:
  - Shows: "Rate limited. Retrying in 60s..."
  - Disables action buttons
  - Starts countdown timer
  - Auto-retries after 60s
```

### Database Corruption → Resync

```
SQLite query fails: "database disk image is malformed"
    ↓
Go catches error in ApprovalPR function
    ↓
Go returns: { code: "DATABASE_ERROR", message: "..." }
    ↓
Electron shows: "Local cache corrupted. Resyncing..."
    ↓
Go backs up old DB: ~/.bistro/data.db.backup-2025-04-08
    ↓
Go deletes corrupted DB
    ↓
Go starts fresh sync from GitHub
    ↓
Electron re-fetches PR list
```

---

## Error Logging Infrastructure

### Go Logging (Server-Side)

```go
import "log/slog"

// Structured logging
slog.Error("approval failed",
    slog.String("pr_id", prID),
    slog.String("error_code", string(ErrRateLimited)),
    slog.String("error", err.Error()),
    slog.Int("retry_after", 60),
)

// Output (JSON):
{
  "time": "2025-04-08T14:32:00Z",
  "level": "ERROR",
  "msg": "approval failed",
  "pr_id": "123",
  "error_code": "RATE_LIMITED",
  "error": "GitHub API rate limit exceeded",
  "retry_after": 60
}
```

### Electron Logging (Client-Side)

```typescript
import { logger } from '@/utils/logger'

// Log with context
logger.error('Failed to approve PR', {
    prId: '123',
    errorCode: error.code,
    errorMessage: error.message,
    timestamp: new Date().toISOString(),
})

// Output:
{
  "timestamp": "2025-04-08T14:32:00Z",
  "level": "ERROR",
  "message": "Failed to approve PR",
  "context": {
    "prId": "123",
    "errorCode": "RATE_LIMITED",
    "errorMessage": "Too many requests"
  }
}
```

### Privacy & Retention

- Logs stored locally in `~/.bistro/logs/`
- Rotated daily, keep 7 days
- Never uploaded unless user explicitly requests
- User can view logs via settings (Phase 2)
- Option to disable logging (off by default only for error logs)

---

## Telemetry (What Gets Reported)

**Sent to telemetry service (Phase 2):**

```
- App version
- OS (macOS, Linux, Windows)
- Number of PRs synced
- Sync duration (seconds)
- Error types (not details) and frequency
- Feature usage (e.g., "searched 5 times")
```

**Never sent:**

```
- PR content
- User names or emails
- Organization names
- Repository names
- Personal settings
```

---

## Questions?

**Q: Why no try/catch in business logic?**
A: Catch at the boundary (UI layer). Business logic should fail loudly if something's wrong.

**Q: How long should we retry?**
A: Network errors: 3 times. Rate limits: up to the `Retry-After` header (usually < 1 min). Auth errors: never auto-retry.

**Q: Should we log to a remote service?**
A: Phase 2. Phase 1: local logs only. User can opt-in to telemetry later.

**Q: What if both Go and Electron fail?**
A: Go fails first (IPC returns error), Electron shows modal. If Electron crashes, user restarts app. Go will retry on next startup.

---

**Last updated:** 2025-04-08
