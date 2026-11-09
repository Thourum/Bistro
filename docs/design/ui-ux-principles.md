# Bistro UI/UX Principles — Design Strategy

## Mission

Define the visual and interaction language for Bistro's interface. These principles guide Google Stitch mockups and inform Electron implementation. The goal: **keyboard-first, scannable, distraction-free code review.**

---

## Core Philosophy

### Keyboard is Primary, Mouse is Secondary

- Every action has a single-key binding (a, c, r, j, k, ?)
- Tab order is logical and predictable
- No drag-and-drop workflows
- No right-click context menus
- Hover hints appear but are not required to understand UI
- Default state: keyboard user is in focus, mouse user is accommodated

### Information Density Over Visual Delight

- Maximize signal, minimize noise
- Every pixel serves a purpose
- No animations except functional feedback (loading, state change)
- Use whitespace to separate concepts, not decoration
- Dark mode by default (respects developer preference)

### Scanability Before Aesthetics

- Use typography hierarchy (size, weight) to establish importance
- Color is used functionally (status indicators, not decoration)
- Consistent alignment and spacing (vertical rhythm)
- Labels are clear and consistent across screens
- Abbreviations only where universal (PR, CI, SHA)

---

## Layout Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Command Bar (search / help / quick actions)            │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  PR List Pane        │  Detail Pane (Diff/Comments)   │  Status Pane
│  (40% width)         │  (50% width)                   │  (10% width)
│                      │                                 │
│  • PR 1              │  FILE: src/auth.ts             │  ✓ Approved
│  • PR 2 (selected)   │  +15 -3 lines                  │  ⚠ Changes Req
│  • PR 3              │                                 │  • CI Pending
│  • PR 4              │  [Diff content here]           │
│                      │                                 │  Filter:
│  [Search result]     │  Comments:                     │  • Open (5)
│  [Filtered list]     │  @alice: "Add error check"     │  • Draft (2)
│                      │                                 │  • Merged (12)
│                      │  [Reply box]                    │
└─────────────────────────────────────────────────────────┘
│  Status Bar: Synced at 14:32 | j/k to nav | ? for help │
└─────────────────────────────────────────────────────────┘
```

### Three Panes Explained

**Left: PR List** (persistent)
- Shows all PRs in current filter (open/draft/merged)
- Visual indicators: state (open/closed), review status (approved/changes req/pending), unread comments
- Selection highlight (inverse or highlight bar)
- Search & filter controls at top
- Keyboard: j/k to navigate, Enter to open, gg/G to jump, / to search

**Center: Detail View** (primary focus)
- Diff viewer (file-by-file)
- Comments anchored to lines (inline review)
- Author, created date, base/head branches
- CI status (check runs, pass/fail status)
- Keyboard: scroll/hjkl for diff nav, n/p for next/prev file, r to reply, a to approve

**Right: Status Sidebar** (always-visible summary)
- Approval status (big, hard to miss)
- CI status (all checks at a glance)
- Filter counts (how many open, draft, merged)
- Labels / tags on the PR
- Repo name (which repo does this PR belong to?)
- Keyboard: n/p to cycle through statuses, m to toggle filters

---

## Visual Hierarchy

### Typography

```
Primary Heading (H1): 24px, bold, top margin
  PR title, main section headers

Secondary Heading (H2): 18px, bold
  File names, comment threads, section headers

Body Text: 14px, regular
  PR descriptions, comment bodies, diff context

Code / Monospace: 13px, Menlo/Monaco
  File paths, diffs, commit SHAs, code snippets

Metadata: 12px, muted (50% opacity)
  Timestamps, author names, line numbers
```

### Color Palette (Dark Mode Primary)

```
Background: #0d1117 (GitHub-like dark)
Surface: #161b22 (slightly lighter, cards/panels)
Text Primary: #e6edf3 (light gray, high contrast)
Text Secondary: #8b949e (muted, metadata)
Accent: #58a6ff (blue, for interactive elements)
Success: #3fb950 (green, approved/CI pass)
Warning: #d29922 (orange, pending/attention)
Error: #f85149 (red, failed/changes requested)
Border: #30363d (subtle separator)
```

### State Indicators (Status Pane Right)

```
✓ Approved          → Green background, white checkmark
✗ Changes Requested → Red background, white X
⧗ Pending           → Yellow background, white hourglass
◯ Draft             → Gray background, white circle
◯ Merged            → Purple background, white checkmark
```

---

## PR List Pane Details

### Each PR Row Shows

```
┌─────────────────────────────────────┐
│ [State] Title of the PR (#123)     │  ← Title, number
│ repo-name · @author · 2 days ago    │  ← Metadata
│ +15 -3 | 4 files | ✓ 2 ◯ 1 ✗ 0    │  ← Stats + review status
└─────────────────────────────────────┘
```

**Visual Elements:**
- State badge (left edge): [OPEN], [DRAFT], [MERGED], [CLOSED]
- Separator line between rows (subtle border)
- Hover highlight (slightly lighter background)
- Selection indicator (bold left border or inverse)
- Unread indicator (blue dot if new comments since last sync)

### Filtering & Search

**Search bar at top:**
- Placeholder: "Search by title, author, file, repo..."
- Quick filters as buttons below: "Open", "Draft", "Merged" (toggle)
- Results update as you type (FTS5, local)
- Escape to clear, type again to refine

---

## Detail Pane: Diff Viewer

### File Header

```
src/auth.ts
+15 -3 lines | 1 comment
────────────────────────
```

**Then the diff:**
- Line numbers (left, muted)
- Diff markers (+ - space, left margin)
- Code (syntax highlighted)
- Inline comments (right of line, visually distinct)

### Comment Threads

```
@alice
"Add error check here for null tokens"
2 days ago · Edit · Reply

@bob (author)
"Good catch, will fix"
2 days ago
────────────────────────

[Reply box: Type to comment]
```

**Visual:**
- Avatar (simple initials in circle)
- Author name (bold)
- Comment body (left-aligned)
- Timestamp (muted, right-aligned)
- Actions (Edit, Reply, Resolve — keyboard shortcuts shown on hover)

---

## Status Pane: Right Sidebar

### Approval Status (big, hard to miss)

```
╔════════════════════╗
║   ✓ APPROVED       ║  ← Large, clear
║   By: @alice       ║
║   3 hours ago      ║
╚════════════════════╝
```

Or if not approved:

```
╔════════════════════╗
║   ◯ PENDING        ║
║   Waiting for: @bob║
║   Assigned: @alice ║
╚════════════════════╝
```

### CI Status

```
CI Status:
✓ Tests passed (45/45)
✓ Lint passed
✗ Security scan pending

[View Details]
```

### PR Metadata

```
Repo: org/repo-name
Base: main
Head: feature/auth
+15 -3 | 4 files

Labels: [bug] [high-priority]
```

### Filter Shortcuts

```
Filters:
Open (5)
Draft (2)
Merged (12)

[Clear all]
```

---

## Keyboard Navigation Map

```
┌─────────────────────────────────────┐
│ Command Bar (search, mode)          │  ← /  (search)  ? (help)
├─────────────────────────────────────┤
│                 │                   │
│  j/k (nav)     │  hjkl (scroll)    │  n/p (cycle status)
│  Enter (open)  │  n/p (next file)  │
│  gg/G (jump)   │  r (reply)        │
│  / (search)    │  a (approve)      │
│                │  c (comment)      │
│                │  Esc (back)       │
└─────────────────────────────────────┘
```

---

## Interaction Patterns

### Opening a PR

1. User presses Enter on a PR in list
2. Detail pane loads (diff renders first, from local patches)
3. Comments load second (from SQLite, then fetch fresh)
4. Status sidebar updates
5. Keyboard focus moves to diff
6. User can scroll with j/k or hjkl

**No loading spinners if under 500ms; show spinner if longer**

### Approving a PR

1. User presses `a` (anywhere in detail view)
2. Confirmation hint appears: "Press 'a' again to confirm"
3. User presses `a` again
4. Button briefly shows "Sending..." (max 3s)
5. Status updates to ✓ APPROVED
6. Bottom bar shows "PR approved"
7. WebSocket event triggers list to refresh

### Commenting on a Line

1. User presses `c` on a diff line
2. Reply box appears below that line
3. Placeholder text: "Add comment (Ctrl+Enter to send, Esc to cancel)"
4. User types comment
5. Keyboard shortcut (Cmd+Enter or Ctrl+Enter) to submit
6. Comment appears immediately (optimistic update)
7. WebSocket confirmation updates server-side

---

## Accessibility & Readability

### Contrast

- All text >= WCAG AA standard (4.5:1 for normal, 3:1 for large)
- Don't rely on color alone (use icons + labels)

### Focus Indicators

- Visible focus ring (2px border, accent color)
- High contrast, never removed
- Clear tab order (top-to-bottom, left-to-right)

### Responsive Behavior

**On narrow screens (< 768px):**
- PR List becomes full-width
- Detail pane slides over (or swaps on Enter)
- Status pane collapses into a row at top
- Still fully keyboard navigable

---

## Error & Loading States

### Loading (Network Delay)

```
[PR title...]
Fetching comments... (spinner)
```

- Show spinner only after 200ms
- Timeout after 5s → show error

### Error States

```
⚠ Failed to approve PR
Error: GitHub API rate limited
[Retry] or [Try again in 5 min]
```

- Red border, error icon
- User-friendly message (no stack traces)
- Retry button or keyboard shortcut

### Offline

```
⚠ No network connection
Using cached data (synced 2 hours ago)
```

- Reduced opacity, visual indicator
- Still functional (read/comment locally, queue for sync)

---

## Dark Mode Only (With Light Mode Toggle)

- Default: dark theme (aligns with developer default)
- Light mode available via settings (? key → settings)
- Same contrast, same readability in both

---

## Motion & Animation

**Minimize animations:**
- ✓ Fade on page transition (50ms)
- ✓ Brief highlight on new comment (yellow flash, 300ms)
- ✓ Smooth scroll (100ms between lines)
- ✗ No spinning loaders (use static spinners)
- ✗ No confetti, no celebratory animations
- ✗ No parallax or fancy scrolling effects

**Rationale:** Keep focus on content, reduce cognitive load.

---

## Design Constraints for Stitch Mockups

When creating mockups in Google Stitch, prioritize:

1. **Keyboard focus indicator** — show where keyboard user currently is (left pane, diff line, comment)
2. **Type hierarchy** — make it obvious what's important (PR title >> metadata >> timestamps)
3. **Pane separation** — clear visual boundaries between PR list, diff, status sidebar
4. **Status at a glance** — approval/CI status readable in 1 second
5. **Code readability** — diff is primary content, comments are secondary
6. **Whitespace** — breathing room around sections, not cramped

---

## Success Criteria for UI/UX

✅ User can navigate entire PR and approve without touching mouse
✅ Approval status visible without scrolling (always-on sidebar)
✅ Diff renders before comments load (progressive disclosure)
✅ All actions have single-key shortcuts (no multi-key combos except gg/GG)
✅ First-time user can open a PR and find the main CTA (approve button)
✅ Interface works identically on macOS, Linux, Windows
✅ Dark mode is default, light mode is accessible
✅ Code is readable with syntax highlighting
✅ Comments are clearly tied to specific lines
✅ No "busy" states — always respond to input instantly or show spinner
