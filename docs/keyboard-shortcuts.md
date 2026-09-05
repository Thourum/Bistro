# Bistro keyboard shortcuts

Generated from `electron/src/renderer/utils/keybindings.ts` (2026-08-09). This is
the full surface for a help-menu redesign. Every key is user-overridable via
settings, so the UI must render from config, never hardcode.

## Contexts

A binding is active in **one route** (a screen) or **one overlay** (a modal).
When an overlay is open, ONLY that overlay's bindings fire. Routes: Inbox
(`/prs`), PR detail, Diff, Settings. Overlays: command palette, help, search,
filter, repo picker, add-repo, threads, tour.

Note: the same key intentionally means different things per context (`s` = star
in the inbox, since-last-review in a diff; `Shift+R` = mark lane read in the
inbox, request changes on a PR). The redesign should make context obvious.

---

## Global (every route)

| Key | Action |
|---|---|
| `?` | Help (inbox, PR detail, diff) |
| `:` | Command palette (inbox, PR detail, diff) |
| `Cmd+,` | Settings |
| `Cmd+/` | Search |
| `Esc` | Back / cancel — closes an overlay, else leaves the screen |
| `Cmd+Q` / `Alt+F4` | Quit |

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
| `d` | Toggle draft |
| `m` | Mute |
| `s` | Star |

## PR detail

| Key | Action |
|---|---|
| `Space` | Open the diff |
| `n` / `p` | Next / previous file |
| `a a` | Approve (double-tap) |
| `c` | Comment |
| `Shift+R` | Request changes |
| `r` | Reply |
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
| `s` | Since-last-review range diff |
| `Shift+S` | Toggle side-by-side ↔ unified |
| `w` | Hide whitespace-only hunks |
| `z` | Collapse / expand focused file |
| `v` | Mark file viewed |
| `o` | Open file in browser |
| `Enter` | Reply to selected inline comment |
| `h` / `l`, arrows | Scroll |
| `Ctrl+D` / `Ctrl+U` | Page down / up |
| `0` / `$` | Jump to top / bottom |

Note: PR detail mode keeps its own `n` / `p` for file navigation (no inline
comment view there); the diff route's `n` / `p` mean comment navigation
instead — this is a deliberate context-dependent rebind, not a duplicate bug.

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

## Overlay: command palette, help, search, repo picker, add-repo, tour

`Esc` closes. Palette and search use typing + `↑`/`↓` + `Enter`. The first-run
tour (5 screens) uses its own Back/Next/Finish controls; re-open any time with
the `:tour` palette command.

## Palette-only commands (no dedicated key)

Production commands: `:open in browser`, `:open in editor` (verifies local
checkout against the PR head SHA first; offers checkout-and-open when they
don't match), `:diff split` / `:diff unified`, `:view lanes` / `:view groups`,
`:sync`, `:tour`, `:settings`, `:about`, `:help`, `:clear`.

Development builds also expose `:reset-store`.

---

## Known gaps for the redesign

1. Same key, different meanings across contexts (see note above) — worth showing
   the active context prominently.
2. Route-dependent duplicates (`Shift+R`, `s`, `o`, `f`) should be labelled with
   their context so they don't read as contradictions.
