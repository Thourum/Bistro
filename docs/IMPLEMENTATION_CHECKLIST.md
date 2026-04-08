# Bistro Implementation Checklist — Phases 3-6

**Purpose:** Track implementation of each Phase against design requirements in `design/`.

**Last Updated:** 2026-04-08

---

## Phase 3: Keyboard Navigation & vim-like Bindings

**Design Reference:** `design/keyboard-bindings.md`

### Global Bindings

- [ ] `?` — Open help overlay showing all bindings for current context
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Component:** Help modal in `src/renderer/components/HelpOverlay.tsx` (new)
  - **Test:** `tests/unit/keyboard.test.ts`
  - **Design Details:** Page 1, "Quick Reference" table
  - **Verification:** Help modal displays and closes with Esc

- [ ] `esc` — Close dialog / cancel current input / go back
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** Store.setMode('list'), close any modals
  - **Test:** Modal closes on Esc, search cancels, back to list works
  - **Design Details:** Page 2, "Global" section

### PR List Pane (Left Side)

- [ ] `j` / `↓` — Move down one PR
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(index + 1)`
  - **Test:** Selection increments, scrolls into view
  - **Design Details:** Page 3, "Navigation" subsection

- [ ] `k` / `↑` — Move up one PR
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(index - 1)`
  - **Test:** Selection decrements, scrolls into view
  - **Design Details:** Page 3, "Navigation" subsection

- [ ] `ctrl+d` — Page down (5 PRs at a time)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(index + 5)` with bounds check
  - **Test:** Jumps exactly 5, doesn't overshoot list end
  - **Design Details:** Page 3, "Navigation" subsection

- [ ] `ctrl+u` — Page up (5 PRs at a time)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(Math.max(0, index - 5))`
  - **Test:** Jumps up exactly 5, doesn't go below 0
  - **Design Details:** Page 3, "Navigation" subsection

- [ ] `gg` — Jump to first PR
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(0)`
  - **Test:** Selection goes to 0 regardless of current position
  - **Design Details:** Page 3, "Navigation" subsection

- [ ] `G` — Jump to last PR
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(prs.length - 1)`
  - **Test:** Selection goes to last item, requires PR count in scope
  - **Design Details:** Page 3, "Navigation" subsection

- [ ] `1-9` — Jump to Nth PR (e.g., `3` jumps to PR #3 in list)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedPRIndex(n - 1)` where `n` is 1-9
  - **Test:** `1` → index 0, `5` → index 4, `9` → index 8 (if exists)
  - **Design Details:** Page 3, "Navigation" subsection
  - **Note:** Only valid if n <= list length

### PR List Filtering & Search

- [ ] `/` — Focus search box and enter search mode
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setMode('search')`, focus search input
  - **Component:** Update `CommandBar.tsx` to auto-focus on mode change
  - **Test:** Search input receives focus, cursor visible
  - **Design Details:** Page 3, "Filtering & Search" subsection

- [ ] `enter` (in search) — Apply search filter
  - **File:** `src/renderer/queries/usePRList.ts`
  - **Store Action:** Filter PR list by searchQuery
  - **Implementation:** Use FTS5 query on PR title/body/author/repo
  - **Test:** Search "auth" returns only PRs matching keyword
  - **Design Details:** Page 3, "Filtering & Search" subsection

- [ ] `esc` (in search) — Exit search, clear query
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setMode('list')`, `setSearchQuery('')`
  - **Test:** Search clears, mode returns to list
  - **Design Details:** Page 4, "Exit search" row

- [ ] `f` — Toggle filter menu (Open/Draft/Merged)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Component:** Filter popup in `src/renderer/components/FilterMenu.tsx` (new)
  - **Store Action:** Toggle visibility of filter menu
  - **Test:** Menu appears on `f`, keyboard navigation in menu works
  - **Design Details:** Page 3, "Filtering & Search" subsection

### PR Detail View

- [ ] `enter` (on PR in list) — Open selected PR detail view
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setMode('detail')`
  - **Test:** Switches from list to detail view
  - **Design Details:** Page 4, "PR Detail View" subsection

- [ ] `hjkl` — Scroll in detail view (h=left, j=down, k=up, l=right)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** Scroll diff viewer horizontally/vertically
  - **Component:** `src/renderer/components/DiffViewer.tsx` (Phase 4)
  - **Test:** Diff scrolls smoothly, h/l scroll horizontal, j/k scroll vertical
  - **Design Details:** Page 4, "Detail View" subsection
  - **Note:** Implement in Phase 4 when diff viewer exists

- [ ] `n` — Next file in diff
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedFileIndex(index + 1)` (new store field)
  - **Test:** Cycles through files in PR
  - **Design Details:** Page 4, "Detail View" subsection

- [ ] `p` — Previous file in diff
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setSelectedFileIndex(index - 1)` with bounds check
  - **Test:** Cycles backward through files
  - **Design Details:** Page 4, "Detail View" subsection

- [ ] `a` — Approve PR (double-tap to confirm)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** First `a` shows "Press 'a' again to confirm", second `a` sends approval
  - **IPC Call:** `window.electron.go({ type: 'approve_pr', pr_id: ... })`
  - **Store:** Track pending confirmation state
  - **Test:** Single `a` shows hint, second `a` triggers approval, UI updates
  - **Design Details:** Page 4, "Detail View" subsection; also `design/ui-ux-principles.md` page 11 "Approving a PR"

- [ ] `c` — Comment on current line (in diff)
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Component:** Comment input box appears at current line
  - **Store Action:** `setMode('comment')`
  - **Test:** Comment box opens focused at correct line number
  - **Design Details:** Page 4, "Detail View" subsection; also `design/ui-ux-principles.md` page 12 "Commenting on a Line"

- [ ] `r` — Reply to existing comment
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** If cursor on comment thread, open reply box
  - **Store Action:** `setMode('comment')`
  - **Test:** Reply box opens below selected comment
  - **Design Details:** Page 4, "Detail View" subsection

- [ ] `esc` (in detail) — Back to PR list
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Store Action:** `setMode('list')`
  - **Test:** Returns to list view
  - **Design Details:** Page 4, "Detail View" subsection

### macOS / Windows / Linux Modifiers

- [ ] `cmd+/` (macOS) — Toggle search box
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** Platform-specific check `process.platform === 'darwin'`
  - **Test:** Cmd+/ on macOS focuses search, Ctrl+/ on others doesn't
  - **Design Details:** Page 4, "Global" subsection

- [ ] `ctrl+/` (Windows/Linux) — Toggle search box
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** Platform-specific check for non-macOS
  - **Test:** Ctrl+/ on Windows/Linux focuses search, Cmd+/ on macOS doesn't
  - **Design Details:** Page 4, "Global" subsection

### Phase 3 Summary

**Acceptance Criteria:**
- [ ] All 25 keyboard bindings implemented
- [ ] Bindings match `design/keyboard-bindings.md` exactly
- [ ] Help overlay (`?`) shows correct bindings for current context
- [ ] Platform-specific modifiers work correctly
- [ ] Tests cover all bindings (unit + integration)
- [ ] No hardcoded sequences (all in utility module)

**Files to Create/Modify:**
- Create: `src/renderer/hooks/useKeyboard.ts` (main handler)
- Create: `src/renderer/components/HelpOverlay.tsx`
- Create: `src/renderer/components/FilterMenu.tsx`
- Modify: `src/renderer/store/ui.ts` (add selectedFileIndex, confirmingAction fields)
- Create: `tests/unit/keyboard.test.ts` (comprehensive binding tests)

---

## Phase 4: Diff Viewer & Comments

**Design Reference:** `design/ui-ux-principles.md` (pages 11-12), `design/data-flow.md`

### Diff Parser

- [ ] Parse unified diff format into line-by-line structure
  - **File:** `src/renderer/utils/diff.ts`
  - **Input:** Raw unified diff string from `window.electron.fs()`
  - **Output:** `{ file: string, lines: Array<{ num, type, content, lineNum }> }`
  - **Types:** 'add', 'remove', 'context'
  - **Test:** Parse sample diffs, handle edge cases (no newline at EOF, binary files)
  - **Design Details:** `design/data-flow.md` "Diff Rendering" section

- [ ] Handle multiple files in single diff
  - **File:** `src/renderer/utils/diff.ts`
  - **Implementation:** Group lines by file header
  - **Test:** Multi-file diff parses correctly with file boundaries
  - **Design Details:** Phase 4 task description

### Syntax Highlighting

- [ ] Integrate highlight.js for syntax highlighting
  - **File:** `src/renderer/utils/highlighter.ts`
  - **Implementation:** Detect language from file extension, highlight code
  - **Dependencies:** `highlight.js` (already in package.json)
  - **Test:** Highlight JavaScript, Python, Go code correctly
  - **Design Details:** `design/ui-ux-principles.md` page 11 "Detail Pane: Diff Viewer"

- [ ] Fallback for unknown languages
  - **File:** `src/renderer/utils/highlighter.ts`
  - **Implementation:** No highlighting but preserve formatting
  - **Test:** Unknown file types don't crash, render as plaintext
  - **Design Details:** Phase 4 task description

### Diff Viewer Component

- [ ] Create DiffViewer component
  - **File:** `src/renderer/components/DiffViewer.tsx`
  - **Props:** `diff: ParsedDiff, selectedFile: string`
  - **Rendering:** File headers, line numbers, diff markers, syntax-highlighted code
  - **Test:** Diff renders correctly, lines numbered, colors applied
  - **Design Details:** `design/ui-ux-principles.md` page 11

- [ ] Line numbers (left margin, muted)
  - **File:** `src/renderer/components/DiffViewer.tsx`
  - **Styling:** Color `var(--color-text-secondary)`, font `var(--font-family-mono)`
  - **Test:** Line numbers visible, correct count
  - **Design Details:** `design/ui-ux-principles.md` page 11 "File Header"

- [ ] Diff markers (+, -, space) with proper coloring
  - **File:** `src/renderer/components/DiffViewer.tsx`
  - **Styling:** `+` green (`var(--color-success)`), `-` red (`var(--color-error)`), ` ` default
  - **Test:** Colors apply correctly to diff markers
  - **Design Details:** `design/ui-ux-principles.md` page 11 "Diff content"

- [ ] File navigation (n/p keys from Phase 3)
  - **File:** `src/renderer/components/DiffViewer.tsx`
  - **Implementation:** Tab through files, highlight current file
  - **Test:** `n` and `p` cycle through files correctly
  - **Design Details:** Requires Phase 3 keyboard integration

### Comment Threads

- [ ] Render inline comments on diff lines
  - **File:** `src/renderer/components/CommentThread.tsx`
  - **Data Source:** `comments` from usePRDetail query
  - **Display:** Author avatar, name (bold), body, timestamp (muted)
  - **Test:** Comments render at correct line numbers
  - **Design Details:** `design/ui-ux-principles.md` page 12 "Comment Threads"

- [ ] Comment thread collapse/expand
  - **File:** `src/renderer/components/CommentThread.tsx`
  - **Implementation:** Toggle visibility of replies (keep root comment visible)
  - **Test:** Click thread header to collapse/expand
  - **Design Details:** Phase 4 task description

- [ ] Reply box (comment input)
  - **File:** `src/renderer/components/CommentThread.tsx`
  - **Placeholder:** "Add comment (Ctrl+Enter to send, Esc to cancel)"
  - **Keybindings:** Ctrl+Enter = send, Esc = cancel
  - **Test:** Reply box appears, keyboard shortcuts work
  - **Design Details:** `design/ui-ux-principles.md` page 12

- [ ] Edit and delete existing comments (for author)
  - **File:** `src/renderer/components/CommentThread.tsx`
  - **Implementation:** Show edit/delete buttons only for comment author
  - **IPC Call:** `window.electron.go({ type: 'edit_comment', ... })` / `delete_comment`
  - **Test:** Edit/delete buttons visible for own comments, hidden for others
  - **Design Details:** `design/ui-ux-principles.md` page 12

### Patch File I/O

- [ ] Fetch patch file via IPC from main process
  - **File:** `src/renderer/queries/useDiff.ts` (new TanStack Query hook)
  - **IPC Call:** `window.electron.fs({ path: 'patches/{base_sha}_{head_sha}/...' })`
  - **Error Handling:** Catch and display "Failed to load diff" (neverthrow)
  - **Test:** Patch loads successfully, errors handled
  - **Design Details:** `design/api-contracts.md` "fs:read-patch" section

### Phase 4 Summary

**Acceptance Criteria:**
- [ ] Diff parser handles all unified diff formats
- [ ] Syntax highlighting works for common languages
- [ ] DiffViewer renders with correct styling (colors, typography, line numbers)
- [ ] Comments display inline on correct lines
- [ ] Reply/edit/delete functionality works
- [ ] All errors handled with user-friendly messages
- [ ] Tests cover all parsing, rendering, and interaction scenarios

**Files to Create/Modify:**
- Create: `src/renderer/utils/diff.ts` (parser)
- Create: `src/renderer/utils/highlighter.ts` (syntax highlighting wrapper)
- Create: `src/renderer/components/DiffViewer.tsx`
- Create: `src/renderer/components/CommentThread.tsx`
- Create: `src/renderer/queries/useDiff.ts` (TanStack Query hook)
- Create: `tests/unit/diff.test.ts` (parser tests)

---

## Phase 5: PR Actions & WebSocket Integration

**Design Reference:** `design/api-contracts.md` (WebSocket section), `design/error-handling.md`

### WebSocket Connection

- [ ] Connect to Go WebSocket on app startup
  - **File:** `src/main/index.ts` (or new `src/main/websocket.ts`)
  - **URL:** `ws://localhost:{WS_PORT}` (from env)
  - **Implementation:** Reconnect with exponential backoff on disconnect
  - **Test:** Connection established, logs show "WebSocket connected"
  - **Design Details:** `design/api-contracts.md` "Channel 2: WebSocket"

- [ ] Listen for WebSocket events from Go
  - **File:** `src/renderer/hooks/useWebSocket.ts`
  - **Implementation:** Main process broadcasts events to renderer via IPC
  - **Events:** `pr_updated`, `diff_ready`, `sync_progress`, `error`
  - **Test:** Events received and logged
  - **Design Details:** `design/api-contracts.md` WebSocket event types

- [ ] Handle disconnect and reconnect
  - **File:** `src/main/websocket.ts`
  - **Implementation:** Exponential backoff (1s, 2s, 4s, 8s, max 30s)
  - **UI Indicator:** Show "Reconnecting..." in status bar when disconnected
  - **Test:** Disconnect triggers reconnect, backoff delays increase
  - **Design Details:** `design/error-handling.md` "Offline" section

### Query Invalidation

- [ ] Invalidate PR list on `pr_updated` event
  - **File:** `src/renderer/hooks/useWebSocket.ts`
  - **Implementation:** `queryClient.invalidateQueries({ queryKey: ['prs'] })`
  - **Test:** PR list refetches after event
  - **Design Details:** `design/data-flow.md` "Query Invalidation" section

- [ ] Invalidate PR detail on `pr_updated` event for specific PR
  - **File:** `src/renderer/hooks/useWebSocket.ts`
  - **Implementation:** `queryClient.invalidateQueries({ queryKey: ['pr', prId] })`
  - **Test:** Detail view updates immediately after event
  - **Design Details:** `design/data-flow.md`

- [ ] Invalidate diff on `diff_ready` event
  - **File:** `src/renderer/hooks/useWebSocket.ts`
  - **Implementation:** `queryClient.invalidateQueries({ queryKey: ['diff', prId] })`
  - **Test:** Diff viewer refreshes when new patch ready
  - **Design Details:** `design/data-flow.md`

### Approve PR Action

- [ ] Send approve command via WebSocket
  - **File:** `src/renderer/hooks/useKeyboard.ts` (a key handler)
  - **Implementation:** `window.electron.go({ type: 'approve_pr', pr_id: selectedPR.id })`
  - **Response:** Wait for confirmation or error
  - **Test:** Approval command sent, response received
  - **Design Details:** `design/ui-ux-principles.md` page 11 "Approving a PR"

- [ ] Confirmation: double-tap `a` to confirm approval
  - **File:** `src/renderer/hooks/useKeyboard.ts`
  - **Implementation:** First `a` shows "Press 'a' again to confirm", second `a` sends
  - **Timeout:** Confirmation hint disappears after 2 seconds
  - **Test:** Single `a` shows hint, second `a` sends, timeout works
  - **Design Details:** `design/ui-ux-principles.md` page 11

- [ ] Show "Sending..." state while approval processes
  - **File:** `src/renderer/components/StatusBar.tsx` (or PR detail header)
  - **Implementation:** Show spinner, timeout after 3 seconds with error
  - **Test:** Spinner appears, disappears after success/error
  - **Design Details:** `design/ui-ux-principles.md` page 11

- [ ] Update UI to show "✓ APPROVED" on success
  - **File:** `src/renderer/components/StatusSidebar.tsx` (new, Phase 4+)
  - **State:** Pull from PR detail query, reacts to query invalidation
  - **Test:** Status updates immediately after approval
  - **Design Details:** `design/ui-ux-principles.md` page 10 "Status Pane"

### Request Changes Action

- [ ] Send request-changes command with comment
  - **File:** `src/renderer/hooks/useKeyboard.ts` (r key handler)
  - **Implementation:** Open comment modal, capture text, send via `window.electron.go()`
  - **IPC:** `{ type: 'request_changes', pr_id, comment }`
  - **Test:** Command sent with comment text
  - **Design Details:** `design/api-contracts.md` "go:command" section

- [ ] Comment modal with keyboard support
  - **File:** `src/renderer/components/CommentModal.tsx` (new)
  - **Keybindings:** Ctrl+Enter = send, Esc = cancel
  - **Test:** Modal appears on `r`, keyboard shortcuts work
  - **Design Details:** `design/ui-ux-principles.md` page 12

### Post Comment Action

- [ ] Send comment to specific diff line
  - **File:** `src/renderer/components/CommentThread.tsx` (reply box)
  - **Implementation:** Capture line number, comment text, send via `window.electron.go()`
  - **IPC:** `{ type: 'post_comment', pr_id, line, file, comment }`
  - **Test:** Comment sent with correct metadata
  - **Design Details:** `design/api-contracts.md` "go:command" section

- [ ] Optimistic update: show comment immediately
  - **File:** `src/renderer/components/CommentThread.tsx`
  - **Implementation:** Add comment to local state before server confirmation
  - **Rollback:** Remove if server returns error
  - **Test:** Comment appears instantly, stays on success, removes on error
  - **Design Details:** `design/ui-ux-principles.md` page 12 "Commenting on a Line"

### CI Status Display

- [ ] Fetch CI check runs from Go
  - **File:** `src/renderer/queries/usePRDetail.ts`
  - **Data:** Included in PR detail response or separate query
  - **Fields:** `name, state ('pending'|'success'|'failure'), description, url`
  - **Test:** CI status data loads
  - **Design Details:** `design/ui-ux-principles.md` page 10 "CI Status"

- [ ] Render CI status in sidebar
  - **File:** `src/renderer/components/StatusSidebar.tsx`
  - **Display:** ✓ / ✗ / ⧗ icons with check name and status
  - **Styling:** Green for pass, red for fail, orange for pending
  - **Test:** Statuses render with correct colors and icons
  - **Design Details:** `design/ui-ux-principles.md` page 10

- [ ] Clickable CI status links
  - **File:** `src/renderer/components/StatusSidebar.tsx`
  - **Implementation:** Click to open check details (if `url` provided)
  - **Test:** Click opens link
  - **Design Details:** `design/ui-ux-principles.md` page 10

### Phase 5 Summary

**Acceptance Criteria:**
- [ ] WebSocket connects on startup, reconnects on disconnect
- [ ] Events received and logged correctly
- [ ] Query invalidation works for all event types
- [ ] Approve/request-changes/comment actions send correct IPC commands
- [ ] Optimistic updates show changes immediately
- [ ] CI status displays with correct colors and icons
- [ ] All errors handled per `design/error-handling.md`
- [ ] Tests cover connection, events, actions, and error scenarios

**Files to Create/Modify:**
- Create: `src/main/websocket.ts` (WebSocket client)
- Create: `src/renderer/hooks/useWebSocket.ts` (event listener + query invalidation)
- Create: `src/renderer/components/StatusSidebar.tsx`
- Create: `src/renderer/components/CommentModal.tsx`
- Modify: `src/renderer/hooks/useKeyboard.ts` (add action handlers)
- Create: `tests/integration/websocket.test.ts`

---

## Phase 6: Error Handling & Resilience

**Design Reference:** `design/error-handling.md`, `design/ui-ux-principles.md` (pages 18-19)

### Error Handling with neverthrow

- [ ] All IPC calls wrapped in neverthrow
  - **File:** All query files and action handlers
  - **Pattern:** `ResultAsync.fromPromise(..., (error) => ({ code, message }))`
  - **Test:** Errors caught and returned, not thrown
  - **Design Details:** `design/error-handling.md` "Electron Error Handling"

- [ ] Error codes from Go (AUTH_TOKEN_EXPIRED, RATE_LIMITED, etc.)
  - **File:** `src/types/ipc.ts` (add error code types)
  - **Implementation:** Parse error response, extract code
  - **Test:** Error codes propagate correctly
  - **Design Details:** `design/error-handling.md` "Error Codes (For IPC)"

- [ ] User-friendly error messages
  - **File:** `src/renderer/utils/error.ts` (message formatter)
  - **Mapping:** Error code → human-readable message
  - **Examples:**
    - `AUTH_TOKEN_EXPIRED` → "Your authentication expired. Please re-login."
    - `RATE_LIMITED` → "Too many requests. Try again in a few minutes."
  - **Test:** All error codes have friendly messages
  - **Design Details:** `design/error-handling.md` "User-Facing Messages"

### Error UI Display

- [ ] Error toast/banner component
  - **File:** `src/renderer/components/ErrorBanner.tsx` (new)
  - **Styling:** Red border, error icon, user-friendly message
  - **Auto-dismiss:** After 5 seconds or on close button click
  - **Test:** Error appears and dismisses
  - **Design Details:** `design/ui-ux-principles.md` page 18 "Error States"

- [ ] Toast queue for multiple errors
  - **File:** `src/renderer/components/ToastContainer.tsx` (new)
  - **Implementation:** Stack errors, dismiss oldest when new arrives (max 3)
  - **Test:** Multiple errors queue and dismiss
  - **Design Details:** Phase 6 task description

- [ ] No crash on error (graceful degradation)
  - **File:** All components using try/catch replacement with neverthrow
  - **Implementation:** Render error state, not component crash
  - **Test:** Error doesn't unmount component tree
  - **Design Details:** `design/error-handling.md`

### Retry Logic

- [ ] Exponential backoff for failed queries
  - **File:** `src/renderer/queries/*.ts` (all query hooks)
  - **TanStack Query config:** `retry: 3, retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 30000)`
  - **Test:** Failed query retries 3 times with increasing delays
  - **Design Details:** `design/error-handling.md` "Retry"

- [ ] User-initiated retry button
  - **File:** `src/renderer/components/ErrorBanner.tsx`
  - **Implementation:** "Retry" button triggers `queryClient.refetch()` or action retry
  - **Test:** Retry button refetches data
  - **Design Details:** `design/ui-ux-principles.md` page 18

### Offline Mode

- [ ] Detect network disconnection
  - **File:** `src/main/websocket.ts` or `src/renderer/hooks/useWebSocket.ts`
  - **Implementation:** Track WebSocket connection state, listen to `window.navigator.onLine`
  - **Test:** Offline flag set when disconnected
  - **Design Details:** `design/error-handling.md` "Offline"

- [ ] Show "Offline" indicator in UI
  - **File:** `src/renderer/components/StatusBar.tsx`
  - **Display:** "⚠ No network connection — Using cached data (synced 2h ago)"
  - **Styling:** Reduced opacity, warning color
  - **Test:** Indicator appears when offline
  - **Design Details:** `design/ui-ux-principles.md` page 18

- [ ] Queue actions while offline
  - **File:** `src/renderer/store/ui.ts` (add `queuedActions` array)
  - **Implementation:** Store approve/comment/request-changes actions in store
  - **Retry on reconnect:** Send queued actions when online
  - **Test:** Actions queued offline, sent when reconnected
  - **Design Details:** `design/error-handling.md` "Offline"

- [ ] Cache all data locally for offline viewing
  - **File:** TanStack Query with localStorage caching
  - **Implementation:** Use `gcTime` and localStorage persister
  - **Test:** PR list visible when offline
  - **Design Details:** `design/data-flow.md` "Cache Invalidation"

### Go Crash Detection & Recovery

- [ ] Detect Go process crash
  - **File:** `src/main/go-spawn.ts`
  - **Implementation:** Listen to `goProcess.on('exit')`, check if unexpected
  - **Test:** Process exit triggers detection
  - **Design Details:** `design/error-handling.md` "Internal"

- [ ] Show "Go Engine Crashed" error to user
  - **File:** `src/renderer/components/ErrorBanner.tsx`
  - **Implementation:** Emit IPC event when Go exits unexpectedly
  - **Test:** Error banner shows crash message
  - **Design Details:** Phase 6 task description

- [ ] Auto-restart Go process
  - **File:** `src/main/go-spawn.ts`
  - **Implementation:** Call `spawnGoProcess()` again after crash
  - **Backoff:** Wait 2s, then retry (max 3 attempts)
  - **Test:** Process restarts, reconnects
  - **Design Details:** `design/error-handling.md` "Crash Recovery"

- [ ] Resync after Go restart
  - **File:** `src/renderer/hooks/useWebSocket.ts`
  - **Implementation:** Invalidate all queries when WebSocket reconnects
  - **Test:** Data refreshes after Go restart
  - **Design Details:** `design/data-flow.md`

### Logging & Privacy

- [ ] Detailed error logging on server side (Go)
  - **File:** Go codebase (not in Electron scope)
  - **Implementation:** Log full stack trace, context, user action
  - **Test:** Error logs appear in Go logs
  - **Design Details:** `design/error-handling.md` "Logging"

- [ ] No sensitive data in error messages
  - **File:** `src/renderer/utils/error.ts`
  - **Implementation:** Strip tokens, passwords, personal data from messages
  - **Test:** Error message contains no secrets
  - **Design Details:** `design/security-model.md` "Logging & Audit"

- [ ] No sensitive data in logs visible to user
  - **File:** All logging calls
  - **Implementation:** `console.log()` only safe data, use `console.debug()` for details
  - **Test:** Sensitive data not logged
  - **Design Details:** `design/security-model.md`

### Phase 6 Summary

**Acceptance Criteria:**
- [ ] All errors handled with neverthrow, no unhandled exceptions
- [ ] All error codes mapped to user-friendly messages
- [ ] Error UI displays gracefully, doesn't crash app
- [ ] Retry logic works with exponential backoff
- [ ] Offline mode queues actions, shows indicator
- [ ] Go crash detected, triggers auto-restart
- [ ] All queries invalidate on reconnect
- [ ] No sensitive data in user-visible messages or logs
- [ ] Tests cover all error paths and recovery scenarios

**Files to Create/Modify:**
- Create: `src/renderer/components/ErrorBanner.tsx`
- Create: `src/renderer/components/ToastContainer.tsx`
- Create: `src/renderer/utils/error.ts` (error message formatter)
- Modify: `src/renderer/store/ui.ts` (add queuedActions)
- Modify: `src/main/go-spawn.ts` (crash detection, auto-restart)
- Modify: All query hooks (add retry config)
- Modify: `src/renderer/hooks/useWebSocket.ts` (offline detection)
- Create: `tests/integration/error-handling.test.ts`
- Create: `tests/integration/offline.test.ts`

---

## Cross-Phase Verification

### Design Document Alignment

| Design Doc | Phase | Implementation File | Verification |
|---|---|---|---|
| `keyboard-bindings.md` | 3 | `src/renderer/hooks/useKeyboard.ts` | All 25 bindings match spec |
| `ui-ux-principles.md` (page 11-12) | 4 | `src/renderer/components/DiffViewer.tsx` | Colors, typography, layout match |
| `api-contracts.md` | 5 | `src/main/websocket.ts` | Message format, event types match |
| `error-handling.md` | 6 | `src/renderer/utils/error.ts` | Error codes, messages match |
| `security-model.md` | 6 | All auth/logging files | Token handling, data privacy match |
| `data-flow.md` | 2-6 | All data files | Data flow matches system design |

### Testing Requirements by Phase

| Phase | Unit Tests | Integration Tests | E2E Tests |
|---|---|---|---|
| 3 | Keyboard bindings (all 25) | Store dispatch | Keyboard nav in app |
| 4 | Diff parser, highlighter | DiffViewer rendering | Open PR, view diff |
| 5 | Action dispatchers | WebSocket events | Approve PR end-to-end |
| 6 | Error formatter | Error scenarios | Full error flow |

**Minimum Coverage:** 80% across all tests

### Code Quality Checklist (All Phases)

- [ ] No hardcoded strings (use constants/config)
- [ ] No `console.log()` in production code (use logger)
- [ ] All async functions return `ResultAsync<T, E>`
- [ ] No `try/catch` blocks (neverthrow only)
- [ ] All IPC calls through preload bridge
- [ ] No direct SQLite writes from renderer
- [ ] All keyboard handlers in `useKeyboard.ts`
- [ ] All store state in `src/renderer/store/`
- [ ] Components max 400 lines
- [ ] Files max 800 lines
- [ ] Immutable state updates (no mutations)

---

## Final Checklist

**Before marking Phase complete:**

1. **Spec Compliance:**
   - [ ] Read design doc for phase
   - [ ] Check each requirement is implemented
   - [ ] Compare code output against design examples (colors, typography, message text)

2. **Testing:**
   - [ ] Unit tests written and passing
   - [ ] Integration tests passing
   - [ ] Manual testing of user flows
   - [ ] Error paths tested

3. **Code Quality:**
   - [ ] Type checking passes
   - [ ] No ESLint errors
   - [ ] Code review completed
   - [ ] No hardcoded values
   - [ ] Immutable patterns used

4. **Git:**
   - [ ] All changes committed
   - [ ] Commit messages follow conventional commits
   - [ ] Branch is clean for next phase

5. **Documentation:**
   - [ ] This checklist updated
   - [ ] README updated with new features
   - [ ] Comments added to complex logic

---

**Last Updated:** 2026-04-08
**Next Review:** After Phase 3 completion

