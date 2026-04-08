# Bistro Electron UI Shell — Agent Context

## Mission

Build the **user-facing desktop shell** — the app that users interact with directly. You own rendering, keyboard navigation, WebSocket integration, and everything users see and touch. You are **not building the backend sync logic** (that's the Go team).

---

## What You Own

```
Electron + React App
├── Main process (spawn Go, read SQLite, handle IPC)
├── Renderer (React app with TanStack Query + Store)
├── Keyboard navigation (vim-like bindings, global shortcuts)
├── Diff viewer (syntax highlighting, line comments)
├── Comment threads (inline review, approve/request changes)
├── OAuth browser window (initial auth)
└── WebSocket event handling (listen for sync progress, invalidate queries)
```

**Your boundary:** UI layer only. You read from SQLite, you don't write to it. You send commands to Go, you receive events from Go.

---

## Core Concepts (Read These First)

1. **SQLite is read-only for you**
   - Main process reads SQLite via `better-sqlite3`
   - Renderer requests data via IPC (preload bridge)
   - Never write to SQLite from main or renderer
   - Go is the only writer

2. **Keyboard-first design**
   - Every action has a binding (j/k, a, c, r, ?)
   - Mouse is optional, not primary
   - No drag-and-drop workflows
   - No right-click context menus (keyboard only)

3. **Progressive rendering**
   - Open PR → diff renders first (from cached patches on disk)
   - Comments load second (from SQLite, pre-synced)
   - Actions feel instant even on slow network

4. **neverthrow error handling**
   - No try/catch blocks
   - Every async function returns `ResultAsync<T, E>`
   - Errors are handled explicitly, never silently swallowed
   - User-facing error messages are friendly, non-technical

5. **TanStack Query for data**
   - Queries read from SQLite via IPC
   - WebSocket events invalidate specific queries
   - React re-renders only changed parts
   - Refetch on focus, retry on failure

6. **TanStack Store for UI state**
   - Store tracks: selected PR index, pane focus, search mode, keyboard mode
   - Keyboard events dispatch to Store
   - Store changes trigger Query invalidation
   - No Redux, no prop drilling

---

## What NOT to Do

- ❌ Don't use try/catch (use neverthrow Result types)
- ❌ Don't write to SQLite from renderer (IPC read-only)
- ❌ Don't hardcode git provider info (it's multi-provider by design)
- ❌ Don't block the UI on long operations (use async, WebWorkers if needed)
- ❌ Don't trust Go's WebSocket messages without validation (validate schema)
- ❌ Don't make mouse required (keyboard must be primary)
- ❌ Don't show GitHub-specific UI (design for providers abstractly)
- ❌ Don't leak user tokens in logs or error messages

---

## Stack & Dependencies

| Component | Library | Why |
|-----------|---------|-----|
| Desktop shell | Electron | Cross-platform, familiar |
| UI framework | React 18+ | Fast, declarative |
| Build tool | Vite | Fast dev server, optimal builds |
| State management | TanStack Query + TanStack Store | Lightweight, composable |
| Error handling | `neverthrow` | Go-style Result types |
| Syntax highlighting | `highlight.js` or `refractor` | Zero-config, works in browser |
| Diff parsing | Custom or `diff-match-patch` | Parse unified diffs |
| WebSocket | `ws` or browser WS API | Standard, no extra deps |
| IPC bridge | Electron preload + context isolation | Secure, performant |

---

## IPC Contract with Main Process

### Renderer → Main (request data)

```typescript
// Read PR list
const result = await ipcRenderer.invoke('db:query', {
  sql: 'SELECT * FROM pull_requests WHERE provider_id = ? ORDER BY updated_at DESC',
  params: [provider_id]
});
// returns: { ok: pr[] } or { err: string }

// Read patch file
const result = await ipcRenderer.invoke('fs:read-patch', {
  path: 'patches/{base_sha}_{head_sha}/path/to/file.patch'
});
// returns: { ok: string } or { err: string }
```

### Renderer → Main (send commands to Go)

```typescript
const result = await ipcRenderer.invoke('go:command', {
  type: 'approve_pr',
  pr_id: 'github:owner/repo:123'
});
// returns: { ok: true } or { err: string }
```

---

## WebSocket Event Handling

```typescript
// Main process listens to Go WebSocket, broadcasts to renderer
ws.on('message', (data) => {
  const event = JSON.parse(data);
  mainWindow.webContents.send('go:event', event);
});

// Renderer listens and invalidates queries
ipcRenderer.on('go:event', (event) => {
  if (event.type === 'pr_updated') {
    queryClient.invalidateQueries({ queryKey: ['pr', event.pr_id] });
  }
  if (event.type === 'diff_ready') {
    queryClient.invalidateQueries({ queryKey: ['diff', event.pr_id] });
  }
});
```

---

## Error Handling Philosophy

No exceptions. Every async operation returns a Result:

```typescript
// Good
const result = await usePRList.query();
if (result.ok) {
  const prs = result.value;
  // render
} else {
  // show error message (never throw)
  showToast(result.error.message);
}

// Bad
try {
  const prs = await fetch('/pr-list');  // might fail
  setPRs(prs);
} catch (e) {
  // crash or silent fail
}
```

---

## Keyboard Bindings (Reference)

| Key(s) | Action | Handler |
|--------|--------|---------|
| `j` / `↓` | Next PR | Store dispatch, invalidate |
| `k` / `↑` | Previous PR | Store dispatch, invalidate |
| `gg` | Jump to top | Store dispatch |
| `G` | Jump to bottom | Store dispatch |
| `/` | Search mode | Store.setMode('search') |
| `Enter` (in search) | Filter PRs | Query filter |
| `Esc` | Exit search / Back to list | Store.setMode('list') |
| `Enter` (on PR) | Open detail | Route change |
| `a` | Approve PR | Send command to Go |
| `c` | Comment | Store.setMode('comment') |
| `r` | Request changes | Send command to Go |
| `?` | Show help | Store.setShowHelp(true) |

---

## Testing

- **Unit tests** for utilities (diff parser, keyboard handler, error formatting)
- **Component tests** with mocked SQLite data, mocked WebSocket events
- **Integration tests** opening/closing PRs, keyboard navigation
- **E2E tests** with Playwright (click buttons, type, verify UI updates)
- **Minimum coverage:** 80% on queries + components

Run tests: `npm test`

---

## Development Workflow

1. Read `PLAN.md` (6 phases in order)
2. Start with Phase 1 (IPC bridge, preload, spawn Go)
3. Don't build UI without IPC working first
4. Phase 3 (keyboard) before Phase 4 (visual features)
5. Error handling **must** be done as you code (neverthrow from the start)
6. Commit frequently with descriptive messages

---

## Key Files You'll Create

```
electron/
  package.json
  vite.config.ts
  tsconfig.json
  src/
    main/
      index.ts              ← App lifecycle
      ipc.ts                ← IPC handlers
      go-spawn.ts           ← Spawn Go binary
      preload.ts            ← Secure IPC bridge
    renderer/
      App.tsx               ← Main layout
      pages/
        PRList.tsx
        PRDetail.tsx
      components/
        DiffViewer.tsx
        CommentThread.tsx
        CIStatus.tsx
      hooks/
        useSQLiteQuery.ts    ← IPC + neverthrow
        useWebSocket.ts
        useKeyboard.ts
      queries/              ← TanStack Query hooks
        usePRList.ts
        usePRDetail.ts
      store/                ← TanStack Store
        index.ts
        ui.ts
      types/
        index.ts            ← Mirror Go domain types
        ipc.ts
      utils/
        keyboard.ts
        diff.ts
```

---

## Questions? Check Here

- **How do I read from SQLite?** → Invoke IPC call to main process, which reads via `better-sqlite3`
- **How do I handle slow network?** → TanStack Query handles retries/caching; UI renders cached data first
- **How do I parse a unified diff?** → See `utils/diff.ts`, or use `diff-match-patch` library
- **How do I send a command to Go?** → Invoke IPC call to main, which sends WebSocket message to Go
- **How do I style the diff viewer?** → CSS modules or Tailwind, syntax highlighting via `highlight.js`
- **How do I test without Go running?** → Mock IPC, return fake data from `usePRList`
- **What if the user has multiple GitHub accounts?** → Provider ID is `github:{username}`, UI filters by active provider

---

## Success Criteria

✅ Electron app starts, spawns Go, connects to SQLite
✅ PR list renders with real data (reads from SQLite via IPC)
✅ Keyboard navigation works (j/k, gg/G, Enter)
✅ Open PR → diff renders instantly (from cached patches)
✅ Comments visible on diff lines
✅ Approve/comment actions send commands to Go, update UI
✅ WebSocket events trigger cache invalidation
✅ All errors handled with neverthrow (no try/catch, no crashes)
✅ Search filters PR list (FTS5 via IPC)
✅ Test coverage >= 80%
