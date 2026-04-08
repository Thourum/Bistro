# Bistro Keyboard Bindings — Complete Reference

**Purpose:** Authoritative keyboard binding reference for users and implementers.

**Audience:** End users, Electron developers, onboarding docs.

---

## Core Philosophy

**Vim-inspired but discoverable:**
- Power users get single-key access to everything
- New users press `?` to see bindings
- No complex chords (Ctrl+Alt+Shift+X), only simple modifiers
- Mouse works but is intentionally slower (no right-click menus)
- Every action has a keyboard binding

---

## Quick Reference

| Context | Key | Action |
|---------|-----|--------|
| **Everywhere** | `?` | Show help / bindings |
| | `esc` | Close help, cancel input |
| | `:` | Open command mode |
| **PR List** | `j` / `k` | Next / Previous PR |
| | `gg` | Jump to top |
| | `G` | Jump to bottom |
| | `enter` | Open selected PR |
| | `/` | Focus search box |
| | `f` | Toggle filter menu |
| **Detail View** | `hjkl` | Scroll (h=left, j=down, k=up, l=right) |
| | `n` | Next file in diff |
| | `p` | Previous file in diff |
| | `a` | Approve PR (double-tap to confirm) |
| | `c` | Comment on current line |
| | `r` | Reply to existing comment |
| | `esc` | Back to PR list |
| **Global** | `cmd+/` | Toggle search box (macOS) |
| | `ctrl+/` | Toggle search box (Linux/Windows) |

---

## Complete Bindings by Context

### 1. Everywhere (Global)

```
?       → Open help overlay
        → Shows all bindings for current context
        → Can be opened from any screen

esc     → Close dialog / cancel current input
        → Exit search, help, command mode
        → Go back to previous context

cmd+q   → Quit app (macOS)
alt+f4  → Quit app (Windows/Linux)
```

---

### 2. PR List Pane (Left Side)

**Navigation:**

```
j       → Move down one PR
k       → Move up one PR
ctrl+d  → Page down (5 PRs at a time)
ctrl+u  → Page up (5 PRs at a time)
gg      → Jump to first PR
G       → Jump to last PR
1-9     → Jump to Nth PR (e.g., 3 jumps to PR #3 in list)
```

**Filtering & Search:**

```
/       → Focus search box (no modifier needed)
        → Type to search by: title, author, repo, file name
        → Results update as you type (FTS5, local)
        → Escape to clear and exit search

f       → Toggle filter panel
        → Shows: Open (5) | Draft (2) | Merged (12)
        → Press 'o' for open, 'd' for draft, 'm' for merged
        → Can combine filters: 'od' shows open + draft
        → Press 'f' again to collapse

clear   → Clear all filters (in filter menu, or type ':clear')
```

**Actions on PR:**

```
enter   → Open selected PR in detail view
         (Loads diff first, then comments)

o       → Open in browser (GitHub/GitLab web)
         (Requires network)

space   → Preview PR metadata (3-line tooltip)
         → Shows: repo, author, CI status, +/- lines
         → Disappears when moving to next PR

d       → Mark as draft / un-draft
         (Moves PR between Draft and Open sections)

m       → Toggle mute (don't show in list)
         (Stores preference in SQLite)

s       → Star / unstar PR (personal marking)
         (For future phases: enable filtering by starred)
```

**Example flow:**
```
j j j                   ← Move down 3 PRs
enter                   ← Open PR #3
[Detail pane loads]
hjkl                    ← Scroll diff
a a                     ← Approve
esc                     ← Back to list
j                       ← Next PR
enter                   ← Open
```

---

### 3. Detail View (Center Pane + Right Status)

**Diff Navigation:**

```
h       → Scroll left (for long lines)
j       → Scroll down (next line in diff)
k       → Scroll up (previous line in diff)
l       → Scroll right (for long lines)

ctrl+d  → Page down
ctrl+u  → Page up

n       → Next file in diff
         (Jumps to first line of next file)

p       → Previous file in diff
         (Jumps to first line of previous file)

f       → Jump to first modified file (favorite files)
         (Cycles through files with +/- lines)

[       → Jump to previous hunk (@@... @@)
]       → Jump to next hunk
```

**PR Actions:**

```
a       → Approve PR
         → First press: "Press 'a' again to confirm"
         → Second press: Sends approval to GitHub
         → Status pane turns green ✓
         → Takes 1-3s (network round-trip)

c       → Comment on current line
         → Opens reply box below line
         → Placeholder: "Add comment (Cmd+Enter to send)"
         → Type comment, Cmd+Enter to post
         → Comment appears locally, waits for sync

r       → Reply to existing comment
         → If cursor on comment thread: open reply
         → If not on comment: show "No comment selected" hint

d       → Download PR as patch file (future)
         → Saves as ~/Downloads/org-repo-123.patch

x       → Open CI details in sidebar
         → Shows all check runs and their status
         → Press 'x' again to close
```

**Comment Threads:**

```
When focused on a comment thread:

j/k     → Next / Previous comment in thread
r       → Reply to this comment
e       → Edit comment (if authored by you)
del     → Delete comment (if authored by you)
```

**Status Pane Interaction (Right):**

```
When right sidebar is focused:

n       → Next status field (cycle: approval → CI → metadata)
p       → Previous status field (cycle backwards)
m       → Toggle metadata view (collapse/expand)
tab     → Focus next interactive element
```

**General in Detail:**

```
esc     → Back to PR list
         (Preserves scroll position, selection)

?       → Show help for current view
         (Different from list help)

0       → Jump to top of diff
$       → Jump to bottom of diff
```

**Example flow:**
```
[In detail view]
j j j                   ← Scroll down 3 lines
c                       ← Comment on current line
[Reply box opens]
This fix looks good     ← Type comment
cmd+enter               ← Post comment
[Comment appears locally]
n                       ← Jump to next file
a a                     ← Approve PR
esc                     ← Back to list
```

---

### 4. Search / Filter Mode

```
When search box is active (after pressing /):

type...     → Search by title, author, repo, file
            → Results update as you type
            → Highlighting shows matches

enter       → Focus first result, exit search
esc         → Close search, return to list
```

**Special search syntax (future):**

```
author:alice            ← Filter by author (Phase 2)
repo:org/repo           ← Filter by repo
label:bug               ← Filter by label
file:*.ts               ← Filter by file path
```

---

### 5. Filter Menu (Press 'f' on PR List)

```
When filter panel is open:

o           → Toggle "Open" filter
d           → Toggle "Draft" filter
m           → Toggle "Merged" filter
a           → Select All filters
n           → Deselect all (none)

esc         → Close filter menu
f           → Toggle filter menu (same key to close)
```

**Display:**

```
Filters:
[x] Open (5)        ← Press 'o' to toggle
[ ] Draft (2)
[x] Merged (12)

[Clear all]         ← Press 'c' to clear
```

---

### 6. Help / Bindings (Press '?')

```
When help overlay is open:

?       → Close help (or 'esc')
j/k     → Scroll help text
enter   → Try binding (if applicable)
        → E.g., press enter on "j/k navigation"
        → App demonstrates by moving one PR
```

**Help layout:**

```
┌─────────────────────────────────────┐
│ Bistro Keyboard Bindings            │
├─────────────────────────────────────┤
│                                     │
│ PR List:                            │
│ j/k         Navigate up/down        │
│ enter       Open PR                 │
│ /           Search                  │
│ f           Filter menu             │
│                                     │
│ Detail View:                        │
│ hjkl        Scroll diff             │
│ n/p         Next/previous file      │
│ a           Approve                 │
│ c           Comment                 │
│                                     │
│ Press any key to close              │
└─────────────────────────────────────┘
```

---

### 7. Command Mode (Press ':')

```
When in command mode:

:clear          → Clear all filters and search
:focus list     → Move focus to PR list
:focus detail   → Move focus to detail pane
:settings       → Open settings panel
:about          → Show app version and credits
:help           → Show help (same as '?')
```

**Example:**

```
:clear          ← Type this
                ← All filters removed
```

---

## Platform-Specific Modifiers

| Action | macOS | Linux/Windows |
|--------|-------|---------------|
| Quit | `cmd+q` | `alt+f4` |
| Copy | `cmd+c` | `ctrl+c` |
| Paste | `cmd+v` | `ctrl+v` |
| Approve (secondary) | `cmd+a` | `ctrl+a` |
| Search toggle | `cmd+/` | `ctrl+/` |

---

## Multi-Key Sequences (Vim-style)

```
gg      → Jump to top (two 'g' presses)
G       → Jump to bottom (capital G)

Rationale: gg is familiar to Vim users; G is single-key alternative
```

**Why not many multi-key combos?**
- Harder to discover (not shown in one-line help)
- Slower to execute
- gg/G are exceptions (standard Vim, muscle memory)

---

## Discoverability for New Users

1. **Press `?` anytime** → See all bindings for current context
2. **Hover over buttons** → Keyboard shortcut shown in tooltip (e.g., "Approve (a)")
3. **Status bar hints** → Shows current command: "j/k to nav | ? for help"
4. **First-run guide** → Interactive walkthrough (Phase 2)

---

## Accessibility Considerations

- **Large text in help:** 16px font for readability
- **High contrast:** All keybindings visible against background
- **Audio feedback:** Optional beep on successful action (customizable)
- **Mouse fallback:** All keyboard actions have mouse equivalents, but slower
  - Example: `a` to approve vs. clicking "Approve" button
  - Button is smaller and lower-contrast to encourage keyboard use

---

## Design Rationale

### Why single-key bindings?

| Binding | Why |
|---------|-----|
| `j/k` | Vim standard, comfortable for power users |
| `a` | "Approve" starts with 'a', intuitive |
| `c` | "Comment" starts with 'c' |
| `r` | "Reply" starts with 'r' |
| `n/p` | Vim standard for next/previous |
| `/` | Vim search, muscle memory |
| `f` | "Filter" starts with 'f' |
| `gg/G` | Vim standard for top/bottom |

### Why avoid mouse?

- Keyboard is 3-5x faster for power workflows
- Reduces context switching (hands on keyboard)
- Enables one-handed operation (trackpad only)
- Aligns with Vim/editor user expectations

### Why Vim, not Emacs/other?

- Vim bindings are more common in modern developer tools (VSCode, Sublime, Neovim)
- More discoverable (single-key, not complex chords)
- Better for modal editing (approve → back → next)

---

## Future Extensions (Phase 2+)

```
Custom key bindings:
- User can remap 'a' to 'space' if preferred
- Stored in ~/.bistro/keybindings.json
- Validation: can't bind two actions to same key

Emacs-style bindings (alternative):
- Ctrl+N for next, Ctrl+P for previous
- Requires separate layout file

Per-provider overrides:
- GitHub may support different bindings than GitLab
- E.g., 'g' for GitLab-specific action
```

---

## Testing & Validation

All bindings must:
- [ ] Work on macOS, Linux, Windows
- [ ] Be documented in this file
- [ ] Have help text (shown in `?` overlay)
- [ ] Have status bar hints (shown in bottom bar)
- [ ] Not conflict with OS shortcuts (Cmd+Q, Alt+Tab, etc.)

---

## Questions?

**Q: Why not Emacs bindings (Ctrl+N/Ctrl+P)?**
A: Vim bindings are more common in modern tools and require fewer modifier keys.

**Q: Can I change my bindings?**
A: Phase 1: No. Phase 2: Yes, via settings file.

**Q: What if I press a key that's not bound?**
A: Nothing happens. No error. Keeps experience clean.

**Q: Why 'a a' double-tap to approve instead of just 'a'?**
A: Safety. Prevents accidental approvals from typos. Trade-off: one extra keystroke.

**Q: Can I remap 'j/k' to arrow keys?**
A: Phase 2 feature.

---

**Last updated:** 2025-04-08
