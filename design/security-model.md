# Bistro Security Model

**Purpose:** Define how Bistro handles authentication, authorization, tokens, and user data.

**Audience:** Go team, security reviewers, infrastructure team.

---

## Authentication Flow

### Initial OAuth Setup

1. **User launches Bistro for the first time**
   - App shows: "Sign in with GitHub" button
   - No local token stored yet

2. **User clicks "Sign in with GitHub"**
   - Go process opens browser to GitHub OAuth URL
   - App listens on `http://localhost:3000/callback` (ephemeral port)
   - GitHub redirects browser to callback with `?code=...`

3. **Go exchanges code for token**
   - POST to GitHub: `GET https://github.com/login/oauth/access_token`
   - Payload: `{ client_id, client_secret, code }`
   - GitHub returns: `{ access_token, scopes, expires_in }`

4. **Go stores token in OS keychain**
   - macOS: `Security` framework (Keychain)
   - Windows: `Credential Manager`
   - Linux: `Secret Service` (GNOME/KDE)
   - Token is encrypted at rest by OS

5. **Electron verifies token works**
   - Go calls: `GET https://api.github.com/user`
   - If 200 OK: authenticated
   - If 401: token invalid, show "Re-authenticate"

---

## Token Management

### Storage

```
OS Keychain Entry
├── Service: "com.bistro.app"
├── Account: "github-token-${user-id}"
├── Password: [encrypted token]
└── Metadata: expires_at (if applicable)
```

**Why OS keychain?**
- Encrypted at rest by OS
- Protected by user login password
- Can't be accessed by other apps
- Survives app uninstall (user owns it)
- Standard for native apps (1Password, Slack, etc.)

### Refresh Strategy

GitHub tokens either:
- **Never expire** (default, if scope allows)
- **Expire after 8 hours** (if refresh token enabled)

```go
// Check token expiration before each API call
if tokenExpiresAt.Before(time.Now().Add(5 * time.Minute)) {
    // Token expiring soon, refresh now
    newToken, err := refreshGitHubToken(refreshToken)
    if err != nil {
        // Failed: ask user to re-auth
        return ErrAuthTokenExpired
    }
    // Store new token in keychain
    storeToken(newToken)
}
```

### Token Revocation

User can revoke token via:
1. **App settings** → "Sign out"
   - Deletes from OS keychain
   - Clears user session

2. **GitHub settings** → "Authorized Apps"
   - GitHub revokes token
   - Next API call fails with 401
   - App catches, prompts re-login

---

## Permissions Model

### OAuth Scopes Requested

```
repo:status    → Read PR status (checks, required)
read:user      → Read user profile (name, email, for display)
read:org       → Read org membership (for team PRs, optional Phase 2)
```

**NOT requested:**
```
❌ repo:write     (no push/admin)
❌ admin:org      (no org admin)
❌ delete_repo    (no deletion)
❌ repo:invite    (no collaboration)
❌ Full 'repo'    (too broad)
```

**Why minimal scopes?**
- Principle of least privilege
- If token compromised, attacker can't push code
- User trusts app more (transparent about permissions)
- GitHub shows scopes in app authorization

### GitHub App vs. OAuth Token

**Decision: Use OAuth token, not GitHub App**

| Aspect | OAuth Token | GitHub App |
|--------|-------------|-----------|
| Setup | User clicks "Sign in" | Owner installs app |
| Token per user? | Yes | App-level only |
| Can revoke? | Yes (user can) | Yes (owner can) |
| Expiration | None (or 8h with refresh) | Automatic with installation |
| Permissions granular? | Yes (scopes) | Yes (permissions) |

**Chosen: OAuth** because:
- Simpler for users (no app installation)
- Each user has own token
- User controls revocation (not app owner)
- Works for personal + org repos

---

## Local Data Security

### SQLite Database

```
Location: ~/.bistro/data.db
Permissions: 0600 (user-read/write only)
Encryption: None (unencrypted)
Assumption: User's machine is trusted
```

**Why unencrypted?**
- Threat model: attacker with physical access or compromised OS
- If OS is compromised, encryption doesn't help (attacker has keys)
- Unencrypted = faster queries, simpler code
- User can encrypt entire disk (FileVault, BitLocker, LUKS)

**What's in DB:**
```sql
-- Non-sensitive
- PR titles, numbers, branches
- Diff content (can be reconstructed from GitHub)
- Comment text (public on GitHub)

-- Sensitive (but not credential-level)
- Author names, usernames
- Repository names (internal or not)
```

**Not in DB:**
```
❌ OAuth tokens (stored in OS keychain)
❌ Personal access tokens
❌ GitHub credentials
❌ SSH keys
```

---

## Network Security

### API Communication (Electron ↔ Go)

```
IPC (Inter-Process Communication):
├── Type: Local Unix socket (macOS/Linux) or named pipe (Windows)
├── Transport: Not encrypted (same machine, OS-level isolation)
├── Messages: JSON with validation

Why not encrypted?
- Both processes run as same user
- OS already isolates processes
- Encryption adds overhead
- Renderer can't directly access Go (IPC is controlled interface)
```

### GitHub API Communication (Go ↔ GitHub)

```
https://api.github.com/...
├── Protocol: HTTPS (TLS 1.2+)
├── Certificate validation: Strict (reject self-signed)
├── Token: Sent as Authorization header
├── Timeout: 10s per request
```

**Go code:**

```go
import "crypto/tls"

client := &http.Client{
    Timeout: 10 * time.Second,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
            MinVersion:         tls.VersionTLS12,
            InsecureSkipVerify: false, // NEVER disable
        },
    },
}

// Request with token
req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
resp, _ := client.Do(req)
```

### WebSocket Communication (Go ↔ Electron)

```
ws://localhost:{PORT}
├── Protocol: WebSocket (unencrypted over localhost)
├── Why not wss://? No external network exposure
├── Authentication: Token in message header
├── Timeout: 30s ping/pong interval
```

**Why localhost WebSocket?**
- Not exposed to network (firewall blocks external)
- Faster than HTTPS
- Simpler to debug
- Standard for local desktop apps

---

## Input Validation

### User Input (From Electron to Go)

All user input is validated server-side:

```go
// Example: Comment validation
func ValidateComment(body string) error {
    if len(body) == 0 {
        return errors.New("comment body required")
    }
    if len(body) > 65536 {
        return errors.New("comment too long (max 64KB)")
    }
    // Don't validate content (user can say anything)
    return nil
}
```

### API Responses (From GitHub to Go)

Never trust GitHub API responses:

```go
type PR struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    State  string `json:"state"`
}

func (g *Engine) ProcessGitHubPR(data []byte) (*PR, error) {
    var pr PR
    if err := json.Unmarshal(data, &pr); err != nil {
        return nil, fmt.Errorf("malformed PR JSON: %w", err)
    }

    // Validate after unmarshaling
    if pr.Number <= 0 {
        return nil, errors.New("invalid PR number")
    }
    if len(pr.Title) == 0 || len(pr.Title) > 1000 {
        return nil, errors.New("invalid PR title length")
    }
    if pr.State != "open" && pr.State != "closed" {
        return nil, errors.New("invalid PR state")
    }

    return &pr, nil
}
```

---

## Audit & Logging

### What Gets Logged

```
✓ User login/logout (timestamp, user ID)
✓ PR approval (PR ID, timestamp)
✓ Comments posted (comment ID, line number)
✓ API errors (error code, not details)
✓ Rate limit hits (when, until when)
```

### What NOT Logged

```
❌ Full OAuth tokens
❌ PR content (titles, diffs are public but too verbose)
❌ Comment bodies (if they contain sensitive info)
❌ API responses (could contain sensitive data)
```

**Log retention:**

```
- Logs stored in ~/.bistro/logs/
- Rotated daily
- Keep 7 days (automatic cleanup)
- User can manually delete at any time
```

### Example Log Entry

```json
{
  "timestamp": "2025-04-08T14:32:00Z",
  "event": "approve_pr",
  "pr_id": "123",
  "result": "success",
  "duration_ms": 2500
}
```

---

## Threat Model

### Threats We Protect Against

| Threat | Mitigation |
|--------|-----------|
| **Malicious Go process** | Electron validates IPC messages |
| **Token theft (keychain)** | OS keychain encryption + file permissions |
| **MITM on GitHub API** | TLS certificate validation |
| **Typosquatting** | Only official bistro.app domain + GitHub OAuth |
| **Local privilege escalation** | OS responsible (app runs as user) |

### Threats Out of Scope

| Threat | Why |
|--------|-----|
| **Compromised OS** | User's machine security responsibility |
| **Compromised GitHub account** | GitHub 2FA responsibility |
| **Physical theft** | Disk encryption (FileVault) responsibility |
| **Supply chain attack** (e.g., malicious Go dependency) | Dependency scanning (Phase 2) |

---

## SecondFactor Authentication

**Phase 1:** Not implemented
**Phase 2:** Optional

If user has GitHub 2FA:
1. OAuth flow handles 2FA (user enters code in browser)
2. Bistro receives token without needing to know 2FA code
3. Token inherits user's permission level

---

## Secrets Management

### Development

```
Environment variables (.env.local, .gitignored):
- GITHUB_CLIENT_ID
- GITHUB_CLIENT_SECRET
- LOG_LEVEL

Never committed to git
```

### Production

```
Binary is open-source, client_id/secret embedded in release:
├── client_id: Public (can be reverse-engineered)
├── client_secret: Handled by OAuth redirect
└── Result: Each installation uses same credentials (acceptable)

Alternative (Phase 2):
- Distribute different credentials per installation
- Requires backend service (out of scope)
```

---

## Security Checklist (Before Release)

- [ ] Token stored in OS keychain (not disk)
- [ ] HTTPS certificate validation enabled
- [ ] All user input validated server-side
- [ ] No hardcoded secrets in code
- [ ] Keychain access permissions set correctly
- [ ] Minimal GitHub OAuth scopes requested
- [ ] Rate limiting headers handled
- [ ] Logs don't contain sensitive data
- [ ] SQLite DB permissions are 0600
- [ ] WebSocket localhost-only binding
- [ ] TLS 1.2+ enforced
- [ ] Error messages don't leak implementation details

---

## Questions?

**Q: Should we use GitHub App instead of OAuth?**
A: OAuth simpler for users. GitHub App better for org-level control. Chosen OAuth for initial version.

**Q: What if someone steals the client secret?**
A: OAuth secret can't steal user tokens (it's used to authenticate the app, not the user). At worst, attacker can impersonate the app, not users.

**Q: Can you use Bistro on a shared computer?**
A: No. Each user should have their own OS user account. Bistro stores token per-user in OS keychain.

**Q: Should we encrypt the SQLite database?**
A: Not needed if user encrypts their disk (FileVault, BitLocker). Adds complexity without real benefit for local-only data.

**Q: What about API rate limits?**
A: Go tracks `X-RateLimit-Remaining` header. If < 10 remaining, waits before next request. User sees "Rate limited, try again in Xs".

---

**Last updated:** 2025-04-08
