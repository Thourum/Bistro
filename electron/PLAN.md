# Bistro Electron + React UI — Implementation Plan

## Overview

This is the desktop shell and user interface. Responsibilities:
- Spawn the Go sync engine as a child process on app startup
- Read PR data from SQLite via IPC
- Render keyboard-navigable PR list and diff viewer
- Handle keyboard shortcuts (vim-like navigation, enter to open, etc.)
- Manage OAuth browser window for initial authentication
- Stream WebSocket events from Go to invalidate TanStack Query cache
- Display CI status, comments, allow approve/comment actions

**Stack:** Electron (main + renderer), React, TanStack Query, TanStack Store, `neverthrow` for error handling, `better-sqlite3` for direct DB reads.

---

## Architecture

```
Electron App
├── main/
│   ├── index.ts (app lifecycle, spawn Go binary, handle IPC)
│   ├── ipc.ts (IPC channel handlers: read SQLite, send commands to Go)
│   ├── go-spawn.ts (spawn Go process, expose WS port, detect crashes)
│   ├── window.ts (create app window, navigation)
│   └── preload.ts (secure IPC bridge to renderer)
│
└── renderer/ (React + vite)
    ├── App.tsx (main layout: list pane + detail pane)
    ├── pages/
    │   ├── PRList.tsx (keyboard nav, search, filtered list)
    │   └── PRDetail.tsx (diff viewer, comments, actions)
    │
    ├── components/
    │   ├── DiffViewer.tsx (render unified diff with syntax highlighting)
    │   ├── CommentThread.tsx (render + edit comments on diff)
    │   ├── CIStatus.tsx (show check runs)
    │   └── KeyboardHints.tsx (display active keybindings)
    │
    ├── hooks/
    │   ├── useSQLiteQuery.ts (IPC → SQLite queries via neverthrow)
    │   ├── useWebSocket.ts (connect to Go WS, handle events)
    │   └── useKeyboard.ts (global keyboard handler + Store dispatch)
    │
    ├── queries/ (TanStack Query hooks)
    │   ├── usePRList.ts (fetch PR list from SQLite)
    │   ├── usePRDetail.ts (fetch PR + files + comments)
    │   ├── useComments.ts (fetch comments for a PR)
    │   └── useDiff.ts (fetch patch file from disk)
    │
    ├── store/ (TanStack Store for UI state)
    │   ├── index.ts (create store)
    │   ├── ui.ts (selected PR, pane focus, search query, mode)
    │   └── keyboard.ts (current key sequence, hint display)
    │
    ├── types/
    │   ├── index.ts (PR, Comment, File types — mirror Go domain model)
    │   └── ipc.ts (IPC message envelopes)
    │
    └── utils/
        ├── keyboard.ts (vim-like keybinding parser)
        ├── diff.ts (parse unified diff into line-by-line structure)
        └── highlighter.ts (syntax highlighting wrapper)
```

---

## Phase 1: Core Setup & IPC Bridge

### Tasks:

1. **Electron scaffolding** — create-electron-app or manual setup
2. **Main process** — app lifecycle, go-spawn logic, IPC handler setup
3. **Preload script** — secure IPC bridge to renderer
4. **Go spawn** — detect Go binary location in app bundle, spawn as child, extract WS port
5. **SQLite IPC channel** — main process reads SQLite, renderer requests via IPC

**Output:** Electron app starts, spawns Go binary, can send IPC query to read PR list (empty at first).

---

## Phase 2: React UI & TanStack Setup

### Tasks:

1. **React app scaffold** — vite + React 18
2. **TanStack Query setup** — create QueryClient, QueryClientProvider
3. **TanStack Store setup** — create Store for UI state (selected PR, keyboard mode)
4. **PR list page** — render PR list from TanStack Query (reading via IPC)
5. **PR detail page** — layout with diff viewer + comments pane
6. **Navigation** — switch between list/detail views

**Output:** UI can display PR list and navigate to detail view (diff/comments not yet rendered).

---

## Phase 3: Keyboard Navigation & vim-like Bindings

### Tasks:

1. **Global keyboard handler** — capture key events, don't let browser handle them
2. **Key sequence parser** — map sequences like "j" (down), "k" (up), "Enter" (open), "?" (help)
3. **Store dispatch** — keyboard events update Store (selected PR index, mode, search)
4. **Query invalidation** — when selected PR changes, TanStack Query fetches new detail
5. **Keyboard hints display** — show active keybindings in footer

**Output:** User can navigate PR list with j/k, open PRs with Enter, jump with gg/G, search with /.

---

## Phase 4: Diff Viewer & Comments

### Tasks:

1. **Diff parser** — parse unified diff into line-by-line structure with metadata
2. **Syntax highlighting** — use a library like `highlight.js` or `refractor`
3. **Line comments** — render comments inline on diff lines, allow editing
4. **Comment thread** — collapse/expand comment threads
5. **Patch file I/O** — renderer requests patch file via IPC, main process reads from disk

**Output:** Can view full diff with syntax highlighting, see comments anchored to lines.

---

## Phase 5: PR Actions & WebSocket Integration

### Tasks:

1. **WebSocket client** — connect to Go WS on startup, reconnect on disconnect
2. **Event handlers** — listen for sync progress events, invalidate TanStack Query
3. **Approve button** — send approve command via WS, wait for response
4. **Request Changes** — open comment modal, send command with comment body
5. **Post comment** — send comment to specific line or PR level
6. **CI status display** — fetch and render check runs with links

**Output:** User can approve, request changes, comment on PRs. Actions complete and reflect in UI immediately.

---

## Phase 6: Error Handling & Resilience

### Tasks:

1. **neverthrow integration** — all async functions return `ResultAsync<T, E>`
2. **Error UI** — display errors to user without crashing
3. **Retry logic** — retry failed API calls with exponential backoff
4. **Offline mode** — UI still works, shows cached data, queues actions to send when online
5. **Crash recovery** — if Go crashes, detect it, restart, resync

**Output:** App handles all error paths gracefully, no silent failures.

---

## Keyboard Bindings (Sketch)

| Key(s) | Action |
|--------|--------|
| `j` / `↓` | Next PR |
| `k` / `↑` | Previous PR |
| `gg` | Jump to top |
| `G` | Jump to bottom |
| `/` | Search |
| `Enter` | Open PR detail |
| `Esc` | Back to list |
| `a` | Approve |
| `c` | Comment |
| `r` | Request changes |
| `?` | Show help |

---

## Testing Strategy

- **Unit tests** — utility functions (diff parser, keyboard handler, Store logic)
- **Component tests** — React components, mocked SQLite data
- **Integration tests** — full user flow (open app, navigate, approve PR)
- **E2E tests** — using Playwright, click through real workflows

**Minimum coverage:** 80% on renderer + queries.

---

## Verification Checklist

- [ ] Electron app starts, Go process spawned
- [ ] PR list renders with test data (mocked SQLite)
- [ ] Keyboard navigation works (j/k, gg/G)
- [ ] Open PR detail with Enter
- [ ] Diff renders with syntax highlighting
- [ ] Comments visible on diff lines
- [ ] Approve action sends command to Go, updates UI
- [ ] WebSocket events trigger cache invalidation
- [ ] Search filters PR list
- [ ] All error paths handled (no crashes)
- [ ] Test coverage >= 80%
