# Bistro Design & Documentation — Plan

## Overview

This folder contains all design artifacts, specifications, and architectural documentation. It's the source of truth for how Bistro works at a high level.

---

## Current Documents

- **`ui-ux-principles.md`** — Design language, layout strategy, keyboard navigation map (for Google Stitch mockups)
- **`architecture.md`** — Full core system architecture (in root as master plan)

---

## Design Documents to Create

### 1. `data-flow.md`

**Purpose:** Detailed walkthrough of how data moves through the system.

**Sections:**
- User opens app → SQLite reads → instant render
- User opens a PR → diff rendering flow (local git vs API fallback)
- Background sync → WebSocket events → cache invalidation
- User approves PR → action sent to Go → SQLite update → UI refresh
- Multi-provider scenario (GitHub + GitLab accounts in same DB)

---

### 2. `keyboard-bindings.md`

**Purpose:** Complete keyboard reference for end users and developers.

**Sections:**
- Navigation (j/k, gg/G, Page Up/Down)
- Searching & filtering (/, type, enter, esc)
- PR actions (a=approve, c=comment, r=request changes)
- Diff navigation (scroll, jump to file, next/prev hunk)
- Help & settings (?, :set, etc.)
- Vim philosophy: discoverable but power-user optimized

**Constraints:**
- Every action must have a keyboard binding
- Mouse-only workflows should be slow (not impossible, just slow)

---

### 3. `error-handling.md`

**Purpose:** Define error handling strategy across Go + Electron.

**Sections:**
- Error types (network, auth, validation, internal)
- Go side: explicit error returns, logging, retry strategy
- Electron side: neverthrow Result types, error UI patterns
- User-facing error messages (localize, helpful, non-technical)
- Logging & telemetry (what to log, what not to log, privacy)
- Recovery paths (offline mode, reconnect, resync)

---

### 4. `performance-targets.md`

**Purpose:** Define acceptable latencies and resource constraints.

**Sections:**
- App startup: < 2s from click to first render
- PR list load: < 500ms from app start
- Open PR detail: < 100ms (cached diff) or 2s (new diff, needs clone)
- Approve/comment action: < 3s round-trip
- Search: < 50ms for FTS5 query (SQLite in-memory)
- Memory: < 200MB baseline, < 500MB with 50 PRs open in browser
- Background sync doesn't slow down UI interaction

---

### 5. `security-model.md`

**Purpose:** Define security model for tokens, permissions, user data.

**Sections:**
- OAuth token storage: OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- Token refresh: background, transparent, no user intervention
- Permissions requested: PR review scope only (no push, no admin)
- Local data: SQLite unencrypted (user's machine, assumed trusted)
- Network: HTTPS to GitHub API, localhost WebSocket (no encryption needed, not exposed)
- Audit trail: what gets logged, how long logs kept

---

### 6. `future-roadmap.md`

**Purpose:** Document planned features and extensibility points.

**Sections:**
- Phase 2: GitLab support, Gitea support
- Phase 3: Notifications, scheduled background sync
- Phase 4: CLI version, VSCode extension
- Phase 5: AI-assisted code review suggestions
- Extensibility: plugin system, custom keyboard layouts, themes

---

### 7. `contribution-guide.md`

**Purpose:** Developer guide for contributing to Bistro.

**Sections:**
- Development setup (Go 1.21+, Node 18+, Electron tooling)
- Architecture overview (pointer back to architecture.md)
- Running locally: `go run ./cmd/bistro-sync` + `npm start`
- Testing: unit/integration/E2E patterns
- Code style: Go idioms, React patterns, no try/catch
- PR process: testing checklist before submitting

---

## Design Review Checklist

Each document should be reviewed for:

- [ ] Clarity: a new contributor can understand without asking questions
- [ ] Completeness: no ambiguous "TBD" sections
- [ ] Consistency: terminology matches across documents
- [ ] Feasibility: technically achievable with chosen stack
- [ ] User-centric: explains the "why" not just the "what"

---

## Document Ownership

| Document | Owner | Status |
|----------|-------|--------|
| ui-ux-principles.md | Product / Design | ✅ Done (ready for Stitch) |
| architecture.md | Architecture review | ✅ Done (plan approved) |
| data-flow.md | Electron + Go teams | ⏳ Pending |
| keyboard-bindings.md | Product/Design | ⏳ Pending |
| error-handling.md | Go team (primary), Electron (secondary) | ⏳ Pending |
| performance-targets.md | Lead architect | ⏳ Pending |
| security-model.md | Go team + security review | ⏳ Pending |
| future-roadmap.md | Product/Strategy | ⏳ Pending |
| contribution-guide.md | DevEx / Onboarding | ⏳ Pending |

---

## Process

1. As each document is created, add to this index
2. Documents evolve with implementation feedback
3. When a decision changes, update the relevant document and link to the commit
4. Quarterly review: ensure documents stay current
