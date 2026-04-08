# Bistro Go Sync Engine — Implementation Plan

## Overview

This is the backend sync engine that runs as a sidecar to the Electron app. Responsibilities:
- Spawn as a WebSocket server on app startup
- Implement provider interface for GitHub (v1), GitLab & Gitea as future adapters
- Fetch PR data from GitHub API with concurrent goroutines
- Generate diffs via `go-git` shallow fetches
- Write normalized data to SQLite
- Stream sync events back to Electron via WebSocket
- Handle OS keychain for OAuth token storage

**Language:** Go 1.21+
**Key dependencies:** `go-git`, `github.com/octokit/go-octokit`, `zalando/go-keyring`, `gorilla/websocket`, `better-sqlite3` (via CGO)

---

## Architecture

```
main (cmd/bistro-sync)
  ├── startup (spawn WS server, wait for Electron connect, exit if timeout)
  ├── websocket (handle incoming commands, send events)
  └── sync orchestrator (coordinate provider fetches + git operations)

internal/
  ├── provider/
  │   ├── interface.go (Provider, PR, Comment types)
  │   ├── github/
  │   │   ├── auth.go (OAuth device flow)
  │   │   ├── prs.go (ListPRs, GetPR, GetComments)
  │   │   ├── actions.go (Approve, RequestChanges, PostComment)
  │   │   └── ci.go (GetCIStatus)
  │   └── models.go (normalized types)
  │
  ├── git/
  │   ├── clone.go (shallow fetch for base + head)
  │   ├── diff.go (generate per-file patches)
  │   └── cache.go (avoid re-cloning same repo)
  │
  ├── store/
  │   ├── db.go (SQLite connection pool)
  │   ├── prs.go (insert/update PR rows)
  │   ├── comments.go (insert/update comments)
  │   ├── files.go (insert/update file metadata + patch paths)
  │   └── migrations.go (schema setup)
  │
  ├── keychain/
  │   ├── keychain.go (wrapper around zalando/go-keyring)
  │   ├── token.go (store/retrieve OAuth tokens)
  │   └── provider.go (multi-provider token lookup)
  │
  └── ws/
      ├── server.go (WebSocket lifecycle)
      ├── handlers.go (command handlers: sync, authenticate, approve, etc.)
      └── events.go (broadcast functions for sync progress)
```

---

## Phase 1: Core Infrastructure

### Tasks:

1. **Project scaffold** (`go mod init`, gitignore, etc.)
2. **SQLite setup** — connection pool, schema migrations, helper functions for CRUD
3. **WebSocket server** — listen on random port, expose port to Electron via temp file, handle connection lifecycle
4. **OS keychain wrapper** — store/retrieve tokens from native OS keychain
5. **Graceful shutdown** — catch SIGTERM, wait for in-flight requests, close connections

**Output:** Empty app that starts, opens WS server, waits for Electron to connect, exits cleanly.

---

## Phase 2: Provider Interface & GitHub Adapter

### Tasks:

1. **Provider interface** — Define `Provider` interface with all methods
2. **GitHub OAuth** — device flow implementation, token refresh
3. **GitHub ListPRs** — fetch PR list with filters (open/merged/draft), handle pagination
4. **GitHub GetPR** — fetch single PR details
5. **GitHub GetComments** — fetch comments for a PR, normalize to domain type
6. **GitHub CI Status** — fetch check runs, normalize status

**Output:** Fully functional GitHub provider that can fetch PR data. SQLite writes not yet connected.

---

## Phase 3: Git Operations & Diff Generation

### Tasks:

1. **Shallow clone/fetch** — clone or update a repo, fetch only base+head commits
2. **Diff generation** — per-file unified diff generation using `go-git`
3. **Patch file I/O** — write patches to `data/patches/{base_sha}_{head_sha}/path/to/file.patch`
4. **Cache management** — avoid re-cloning same repo within a session

**Output:** Can fetch a PR's code diff and generate patches without network (after clone).

---

## Phase 4: Sync Orchestration

### Tasks:

1. **Sync command** — main orchestrator that coordinates everything
2. **Concurrent goroutines** — fetch PR list, then fan-out to fetch comments + CI for each
3. **SQLite writes** — write all normalized data to DB via the store package
4. **Error handling** — all functions return `error`, log failures, continue on partial success
5. **WebSocket events** — emit progress events as sync proceeds

**Output:** Full sync loop — fetch from GitHub, generate diffs, write SQLite, stream events.

---

## Phase 5: Actions & PR Modifications

### Tasks:

1. **Approve PR** — GitHub API call to approve
2. **Request Changes** — GitHub API call with comment body
3. **Post Comment** — post comment to PR or specific line
4. **Receive commands from Electron** — parse WebSocket JSON commands
5. **Update SQLite** — after action succeeds, update DB state

**Output:** User can approve/comment on PRs from the UI; changes reflect in SQLite.

---

## Testing Strategy

- **Unit tests** — Provider adapters, git operations, store functions
- **Integration tests** — Full sync flow against a test GitHub repo
- **E2E** — Start Go binary, connect Electron, simulate full user workflow

**Minimum coverage:** 80% on core paths (provider, store, git operations).

---

## Verification Checklist

- [ ] Go binary starts, opens WS, closes cleanly
- [ ] GitHub OAuth device flow completes successfully
- [ ] ListPRs fetches and writes to SQLite (confirmed via `sqlite3` CLI)
- [ ] Shallow clone works, diff patches generated on disk
- [ ] Comments fetched and stored
- [ ] CI status fetched and stored
- [ ] Approve/Request Changes actions complete
- [ ] All goroutine operations timeout gracefully (no hangs)
- [ ] Test coverage >= 80%
