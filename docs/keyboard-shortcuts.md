# Bistro keyboard shortcuts

All key tables and key examples below are **vim defaults**. Actions and scopes
come from `electron/src/renderer/utils/keybindings.ts`; per-style keys come from
`electron/src/renderer/utils/keymaps.ts`. For vim, Emacs and Standard side by side,
see `website/src/pages/docs/keybindings.mdx`. Every key is user-overridable via
settings, so the UI must render from effective bindings, never hardcode.
To rebind, open Settings → Keyboard, press `Enter` on an action, type the new key
or chord, and press `Enter` to save (`Esc` cancels). `Backspace` restores the
default. Keys that would collide with a binding active in the same place are rejected.

## Contexts

A binding is active in **one route** (a screen) or **one overlay** (a modal).
When an overlay is open, ONLY that overlay's bindings fire. Routes: Inbox
(`/prs`), PR detail, Diff, Settings. Overlays: command palette, help, search,
filter, repo picker, add-repo, threads, line-comment mode.

Note: the same key intentionally means different things per context (`s` = star
in the inbox, since-last-review in a diff; `Shift+R` = mark lane read in the
inbox, request changes on a PR). The redesign should make context obvious.

---

## Cross-route shortcuts

Only quit is truly global. The rest are inactive on login, onboarding and
(except `Esc`) settings. `Mod` = `Cmd` on macOS, `Ctrl` elsewhere.

| Key | Active on | Action |
|---|---|---|
| `?` | inbox, PR detail, diff | Help |
| `:` | inbox, PR detail, diff | Command palette |
| `Mod+,` | inbox, PR detail | Settings |
| `Mod+/` | inbox, PR detail | Search |
| `Esc` | inbox, PR detail, diff, settings, any overlay | Back / cancel — closes an overlay, else leaves the screen |
| `Cmd+Q` (macOS) / `Alt+F4` (Windows/Linux) | every route | Quit |

## Inbox (`/prs`)

| Key | Action |
|---|---|
| `j` / `k` | Next / previous PR |
| `↓` / `↑` | Next / previous PR |
| `g g` | Jump to top |
| `Shift+G` | Jump to bottom |
| `Ctrl+D` / `Ctrl+U` | Down / up 5 PRs |
| `g 1` … `g 6` | Jump to lane 1–6 |
| `1` … `9` | Jump to PR 1–9 in view |
| `Enter` | Open PR |
| `Space` | Expand / collapse repo group |
| `/` | Search |
| `f` | Filter menu |
| `o` | Open PR in browser |
| `Shift+R` | Mark current lane read |
| `d` | Toggle draft — bound, not yet wired (no consumer for the action) |
| `m` | Mute — bound, not yet wired |
| `s` | Star — bound, not yet wired |

## PR detail

| Key | Action |
|---|---|
| `Space` | Open the diff |
| `n` / `p` | Next / previous file |
| `a a` | Approve (includes pending drafts) |
| `c` | Comment |
| `Shift+R` | Request changes (includes pending drafts) |
| `x` | Resolve / unresolve active thread — open the diff and select a thread with `n` / `p` |
| `r` | Reply — bound, not yet wired (use `Enter` on a thread in the diff view) |
| `g u` | Review threads |
| `d n` | Dismiss nits + info |
| `d b` | Dismiss bot comments |
| `u` | Undo last dismissal |
| `f c` / `f w` / `f s` / `f n` | Filter by min severity (critical/warning/suggestion/nit) |
| `f h` | Human comments only |
| `f f` | Clear comment filters |
| `h` `j` `k` `l` | Scroll left/down/up/right |
| `←` `↓` `↑` `→` | Scroll |
| `Ctrl+D` / `Ctrl+U` | Page down / up |
| `0` / `$` | Jump to top / bottom |

## Diff

| Key | Action |
|---|---|
| `j` / `k` | Scroll down / up |
| `Shift+J` / `Shift+K` | Next / previous file |
| `n` / `p` | Next / previous inline comment |
| `s` | Since last review: show only files changed since your last review |
| `i` | Interdiff: the current file since your last review |
| `Shift+S` | Toggle side-by-side ↔ unified |
| `w` | Hide whitespace-only hunks |
| `z` | Collapse / expand focused file |
| `v` | Mark file viewed |
| `o` | Open file in browser |
| `c` | Comment on line |
| `d d` | Delete selected draft |
| `x` | Resolve / unresolve active thread |
| `Enter` | Edit selected draft / reply to selected inline comment |
| `h` / `l`, arrows | Scroll |
| `Ctrl+D` / `Ctrl+U` | Page down / up |
| `0` / `$` | Jump to top / bottom |

Note: PR detail mode keeps its own `n` / `p` for file navigation (no inline
comment view there); the diff route's `n` / `p` mean comment navigation
instead — this is a deliberate context-dependent rebind, not a duplicate bug.

## Comment mode (vim defaults)

Press `c` in the diff to enter line-comment mode. Cursor keys apply while the
composer is closed; `Enter` opens it for typing. Select saved drafts with `n` /
`p` in the diff, then `Enter` to edit or `d d` to delete.

| Key | Action |
|---|---|
| `j` / `k` | Next / previous line |
| `Shift+J` / `Shift+K` | Extend range down / up (within one hunk and side) |
| `h` / `l` | Old / new side (split view) |
| `Enter` | Write comment |
| `Mod+Enter` | Save draft |
| `Esc` | Exit comment mode |

Drafts stay local until you approve, request changes, or choose **Submit review**
in the command palette. Review submissions are not queued for offline replay;
failed submissions keep unsubmitted drafts. **Discard drafts** in the palette
removes all pending drafts for the current PR.

## Overlay: filter

| Key | Action |
|---|---|
| `n` | Needs action |
| `y` | Touches your code |
| `o` | Open |
| `d` | Draft |
| `a` | All |
| `f` / `Esc` | Close |

## Overlay: threads

| Key | Action |
|---|---|
| `j` / `k` | Next / previous thread |
| `r` | Resolve |
| `u` | Unresolve |
| `Esc` | Close |

## Overlay: command palette, help, search, repo picker, add-repo

`Esc` closes. Palette and search use typing + `↑`/`↓` + `Enter`. The `:tour`
palette command brings back the first-review guide on the inbox.

## Palette-only commands (no dedicated key)

Production commands: `:open in browser`, `:open in editor` (verifies local
checkout against the PR head SHA first; offers checkout-and-open when they
don't match), `:diff split` / `:diff unified`, `:view lanes` / `:view groups`,
`:sync`, `:tour`, `:settings`, `:help`, `:clear`.

`:about` appears in the palette but has no handler yet (selecting it is a no-op).
Development builds also expose `:reset-store`.

---

## Known gaps for the redesign

1. Same key, different meanings across contexts (see note above) — worth showing
   the active context prominently.
2. Route-dependent duplicates (`Shift+R`, `s`, `o`, `f`) should be labelled with
   their context so they don't read as contradictions.
