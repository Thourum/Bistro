# Bistro API Contracts — IPC & WebSocket Schemas

**Purpose:** Define message formats between Electron (main/renderer) and Go backend.

**Audience:** Go and Electron implementers. This is the contract — changes must go through code review.

---

## Overview

Three communication channels:

1. **IPC (Electron main ↔ renderer)** — Local function calls
2. **WebSocket (Electron ↔ Go)** — Async events
3. **SQLite (local, no network)** — Shared database

---

## Channel 1: IPC (Electron Internal)

### Purpose

Main process ↔ Renderer process communication. Both run as same user, messages are unencrypted.

### Message Format

```json
{
  "id": "req-123",
  "type": "REQUEST_TYPE",
  "payload": { /* data */ },
  "timestamp": "2025-04-08T14:32:00Z"
}
```

**Response:**

```json
{
  "id": "req-123",
  "type": "RESPONSE_TYPE",
  "payload": { /* data */ },
  "error": null,
  "timestamp": "2025-04-08T14:32:00.5Z"
}
```

### Common IPC Messages

#### getPRList

**Request from renderer:**

```json
{
  "id": "req-1",
  "type": "GET_PR_LIST",
  "payload": {
    "filter": "open",
    "limit": 50,
    "offset": 0
  }
}
```

**Response from main:**

```json
{
  "id": "req-1",
  "type": "GET_PR_LIST_RESPONSE",
  "payload": {
    "prs": [
      {
        "id": "github-123",
        "provider": "github",
        "repo": "org/repo",
        "number": 123,
        "title": "Fix auth bug",
        "author": "alice",
        "state": "open",
        "review_status": "pending",
        "created_at": "2025-04-08T14:00:00Z"
      }
    ],
    "total": 42,
    "has_more": true
  }
}
```

**Error response:**

```json
{
  "id": "req-1",
  "type": "ERROR",
  "payload": null,
  "error": {
    "code": "DATABASE_ERROR",
    "message": "Failed to query PRs"
  }
}
```

#### getPRDetail

**Request:**

```json
{
  "id": "req-2",
  "type": "GET_PR_DETAIL",
  "payload": {
    "pr_id": "github-123"
  }
}
```

**Response:**

```json
{
  "id": "req-2",
  "type": "GET_PR_DETAIL_RESPONSE",
  "payload": {
    "pr": {
      "id": "github-123",
      "number": 123,
      "title": "Fix auth bug",
      "description": "...",
      "author": "alice",
      "base_branch": "main",
      "head_branch": "fix/auth",
      "state": "open",
      "review_status": "pending",
      "ci_status": "pending",
      "ci_checks": [
        { "name": "Tests", "status": "success" },
        { "name": "Lint", "status": "pending" }
      ],
      "created_at": "2025-04-08T14:00:00Z"
    },
    "diff": "--- a/file.js\n+++ b/file.js\n@@ ... @@",
    "comments": [
      {
        "id": "comment-456",
        "author": "bob",
        "body": "Looks good",
        "line": 42,
        "created_at": "2025-04-08T14:15:00Z"
      }
    ]
  }
}
```

#### approvePR

**Request from renderer:**

```json
{
  "id": "req-3",
  "type": "APPROVE_PR",
  "payload": {
    "pr_id": "github-123"
  }
}
```

**Response from main (success):**

```json
{
  "id": "req-3",
  "type": "APPROVE_PR_RESPONSE",
  "payload": {
    "pr_id": "github-123",
    "review_status": "approved",
    "approved_at": "2025-04-08T14:32:00Z"
  }
}
```

**Response from main (error):**

```json
{
  "id": "req-3",
  "type": "ERROR",
  "error": {
    "code": "RATE_LIMITED",
    "message": "Too many requests",
    "hint": "Try again in 60 seconds"
  }
}
```

#### postComment

**Request:**

```json
{
  "id": "req-4",
  "type": "POST_COMMENT",
  "payload": {
    "pr_id": "github-123",
    "line": 42,
    "body": "Great fix!"
  }
}
```

**Response (success):**

```json
{
  "id": "req-4",
  "type": "POST_COMMENT_RESPONSE",
  "payload": {
    "comment_id": "comment-789",
    "pr_id": "github-123",
    "author": "you",
    "body": "Great fix!",
    "line": 42,
    "created_at": "2025-04-08T14:32:00Z"
  }
}
```

---

## Channel 2: WebSocket (Electron ↔ Go)

### Purpose

Real-time events from Go to Electron. Go is the server, Electron connects as client.

### Connection

```
ws://localhost:8080
Headers: { Authorization: "Bearer <token>" }
```

### Message Format

```json
{
  "id": "evt-1",
  "type": "EVENT_TYPE",
  "data": { /* payload */ },
  "timestamp": "2025-04-08T14:32:00Z"
}
```

### Events from Go to Electron

#### syncComplete

**When:** Background sync finishes

```json
{
  "id": "evt-sync-001",
  "type": "SYNC_COMPLETE",
  "data": {
    "pr_count": 42,
    "new_prs": 3,
    "updated_prs": 5,
    "deleted_prs": 0,
    "duration_ms": 2300
  },
  "timestamp": "2025-04-08T14:32:05Z"
}
```

#### prApproved

**When:** PR is approved (by user or auto-approved)

```json
{
  "id": "evt-approve-001",
  "type": "PR_APPROVED",
  "data": {
    "pr_id": "github-123",
    "repo": "org/repo",
    "number": 123,
    "approved_by": "alice",
    "timestamp": "2025-04-08T14:32:00Z"
  }
}
```

#### commentPosted

**When:** Comment is posted

```json
{
  "id": "evt-comment-001",
  "type": "COMMENT_POSTED",
  "data": {
    "pr_id": "github-123",
    "comment_id": "comment-789",
    "author": "alice",
    "body": "Great fix!",
    "line": 42,
    "created_at": "2025-04-08T14:32:00Z"
  }
}
```

#### syncError

**When:** Background sync fails

```json
{
  "id": "evt-error-001",
  "type": "SYNC_ERROR",
  "data": {
    "error_code": "NETWORK_TIMEOUT",
    "message": "Failed to reach GitHub",
    "provider": "github",
    "will_retry": true,
    "retry_after_ms": 5000
  }
}
```

#### authTokenExpired

**When:** OAuth token expires or revoked

```json
{
  "id": "evt-auth-001",
  "type": "AUTH_TOKEN_EXPIRED",
  "data": {
    "provider": "github",
    "message": "Please re-authenticate"
  }
}
```

#### rateLimited

**When:** GitHub API rate limit hit

```json
{
  "id": "evt-ratelimit-001",
  "type": "RATE_LIMITED",
  "data": {
    "provider": "github",
    "reset_at": "2025-04-08T15:00:00Z",
    "reset_in_seconds": 1800
  }
}
```

### Requests from Electron to Go (via WebSocket)

#### requestDiff

**Electron sends:**

```json
{
  "id": "req-diff-001",
  "type": "REQUEST_DIFF",
  "data": {
    "pr_id": "github-123",
    "repo": "org/repo"
  }
}
```

**Go responds with event:**

```json
{
  "id": "req-diff-001",
  "type": "DIFF_READY",
  "data": {
    "pr_id": "github-123",
    "diff": "--- a/file.js\n+++ b/file.js\n@@ ... @@",
    "size_bytes": 2048
  }
}
```

#### requestComments

**Electron sends:**

```json
{
  "id": "req-comments-001",
  "type": "REQUEST_COMMENTS",
  "data": {
    "pr_id": "github-123"
  }
}
```

**Go responds:**

```json
{
  "id": "req-comments-001",
  "type": "COMMENTS_READY",
  "data": {
    "pr_id": "github-123",
    "comments": [
      {
        "id": "comment-456",
        "author": "bob",
        "body": "Good catch",
        "line": 42,
        "created_at": "2025-04-08T14:15:00Z"
      }
    ]
  }
}
```

---

## Error Codes (Complete List)

Used in both IPC and WebSocket error responses.

```
NETWORK_TIMEOUT
NETWORK_UNREACHABLE
AUTH_TOKEN_EXPIRED
AUTH_INSUFFICIENT_SCOPE
AUTH_NOT_AUTHENTICATED
RATE_LIMITED
NOT_FOUND
FORBIDDEN
INVALID_INPUT
DATABASE_ERROR
INTERNAL
```

---

## Data Type Definitions

### PR

```typescript
interface PR {
  id: string;                 // provider-number (e.g., "github-123")
  provider: 'github' | 'gitlab' | 'gitea';
  repo: string;               // org/repo
  number: number;             // PR number in that repo
  title: string;
  description?: string;
  author: string;             // username
  state: 'open' | 'closed' | 'merged';
  review_status: 'pending' | 'approved' | 'changes_requested';
  base_branch: string;
  head_branch: string;
  created_at: string;         // ISO 8601
  updated_at: string;
  merged_at?: string;
  ci_status?: 'pending' | 'success' | 'failure';
}
```

### Comment

```typescript
interface Comment {
  id: string;
  pr_id: string;
  author: string;
  body: string;
  line?: number;              // null if not on specific line
  created_at: string;
  updated_at: string;
  resolved?: boolean;
}
```

### Error

```typescript
interface ErrorInfo {
  code: ErrorCode;
  message: string;            // User-friendly
  hint?: string;              // Developer hint
  details?: Record<string, any>;  // Debug info
}
```

---

## Rate Limiting (WebSocket)

### Client behavior

```typescript
// Electron should not send > 1 message per 100ms
// Go will close connection if client spams
const delay = 100; // ms between messages
```

### Server behavior

```go
// Go closes connection if client sends > 100 messages/sec
// Or > 1000 messages in 10 seconds
```

---

## Versioning

### API Version Header

```json
{
  "type": "HANDSHAKE",
  "api_version": "1.0",
  "min_version": "1.0"
}
```

When APIs change:
- Increment major: breaking changes
- Increment minor: new fields, backwards compatible

---

## Examples

### Complete Approval Flow

**Electron (renderer) → IPC (main):**

```json
{
  "id": "approve-1",
  "type": "APPROVE_PR",
  "payload": { "pr_id": "github-123" }
}
```

**IPC (main) → Go (WebSocket):**

```go
// Internal call, not JSON message
go.engine.ApprovePR(ctx, "github-123")
```

**Go processes, updates GitHub, then sends WebSocket:**

```json
{
  "id": "evt-approve-1",
  "type": "PR_APPROVED",
  "data": {
    "pr_id": "github-123",
    "approved_by": "you"
  }
}
```

**Electron (main) receives event, sends IPC to renderer:**

```json
{
  "id": "notify-1",
  "type": "PR_APPROVED",
  "payload": {
    "pr_id": "github-123",
    "status": "approved"
  }
}
```

**Renderer updates UI:** ✓ APPROVED (green)

---

## Backwards Compatibility

### Adding fields (safe)

```json
// Old response
{ "pr_id": "123", "status": "approved" }

// New response (old clients ignore new_field)
{ "pr_id": "123", "status": "approved", "new_field": "value" }
```

### Removing fields (BREAKING)

```json
// Old response
{ "pr_id": "123", "status": "approved", "old_field": "value" }

// New response (old clients crash)
{ "pr_id": "123", "status": "approved" }
// ❌ Bump major version, requires coordinated release
```

---

## Questions?

**Q: Why not REST for all communication?**
A: REST is request/response only. WebSocket enables real-time push events (sync complete, new comments, etc.).

**Q: Why not use gRPC?**
A: Added complexity, slower for local IPC, Electron doesn't have great gRPC support.

**Q: What if message size exceeds limit?**
A: Go closes connection with error. Electron should chunk large data (diffs > 10MB).

**Q: Can I add a new message type?**
A: Yes, but document it here first. Coordinate with both teams.

---

**Last updated:** 2025-04-08

**Breaking change policy:** Bump API version, require both teams to update simultaneously.
