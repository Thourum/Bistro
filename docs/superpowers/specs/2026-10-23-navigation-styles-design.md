# Navigation styles (vim / emacs / standard) + onboarding picker — design

Status: approved in brainstorming 2026-10-23. Spec 1 of 2; spec 2 (mouse
affordances) is listed under "Out of scope".

## Goal

Let users pick how Bistro's keyboard behaves — **vim**, **emacs**, or
**standard** (arrows + desktop keys, mouse-friendly) — each with its own full
default keymap, and add an onboarding step where they pick a style and try it
in a sandbox before they hit real PRs.

## Decisions

- A style changes **both movement and action keys**. Each style is a complete
  keymap over the same set of actions.
- User overrides in `settings.json` (`keybindings`) still apply on top of the
  selected style and survive style switches.
- Default style is `vim` (today's bindings, unchanged). Existing users get no
  behavior change; they can switch in Settings → Keyboard.
- Standard style uses arrows / Home / End / PgUp / PgDn / Ctrl+Tab for
  movement, but plain letters for actions (same as vim) — it is not
  "no bare letters".
- Emacs `C-c` / `C-x` prefixes shadow copy/cut on Windows/Linux when no text
  field is focused. Accepted; stated on the emacs style card.
- Mouse affordances beyond today's are spec 2, not this spec.

## Architecture

### Action catalog + per-style key tables

`utils/keybindings.ts` keeps the action catalog: `actionId`, `description`,
`activation`, `hidden`. `keys` moves out of the catalog into per-style tables:

```ts
export type NavigationStyle = 'vim' | 'emacs' | 'standard';
type KeyTable = Readonly<Record<string, string | readonly string[]>>;
export const KEYMAPS: Readonly<Record<NavigationStyle, KeyTable>>;
```

The tables live in a new `utils/keymaps.ts`, with one table per style. Same
key format as today: a string is one hotkey, an array is a sequence.

```ts
getEffectiveBindings(style: NavigationStyle, overrides: KeybindingOverrides): KeyBinding[]
```

Resolution per catalog action: user override (empty = unbound) → style table
→ unbound. An action missing from a style's table has no key in that style
(it stays reachable from the command palette). Every caller of
`getEffectiveBindings` (useAppHotkeys, KeyboardSection, TourOverlay,
HelpOverlay, legendPresets, CommandPalette, footer) passes the style read
from `settingsStore`.

Alias actions (`list.downArrow`, `detail.scrollLeftArrow`, …) stay as they
are. Emacs adds one alias, `global.cancel` (`C-g`), which runs the same
handler as `global.escape`. It does **not** get the Escape-specific
`defaultPrevented` / `ignoreInputs: false` handling in `useAppHotkeys`.
Radix closes only on real Escape, so `C-g` closes overlays through the
action handler.

`onboarding.finish` is renamed `onboarding.continue` (see Onboarding).

### Sequence timeout per style

`SEQUENCE_TIMEOUT_MS` becomes a lookup in `useAppHotkeys`:
`{ vim: 500, emacs: 1500, standard: 500 }`. Emacs prefix chords
(`C-c C-a`) are typed slower than `g g`.

### Settings

`types/settings.ts`: new section

```ts
const navigationSchema = z.object({ style: z.enum(['vim', 'emacs', 'standard']) });
// DEFAULT_SETTINGS.navigation = { style: 'vim' }
```

`main/settings.ts` `normalise()` already fills missing sections from
defaults, so there is no `$schema` bump. `main/ipc.ts` tracks
`settings_updated` with `section: 'navigation'`, the same as the other
sections.

## Keymaps

`·` means the binding is the same as vim. **Shared** rows are identical in
every style. Notation is the TanStack string used in the tables (`C-x` is
shorthand for `Control+X`, `M-x` for `Alt+X`).

### Shared (all styles)

| actionId | keys |
|---|---|
| global.escape | `Escape` |
| global.settings | `Mod+,` |
| global.quit.mac / .win | `Meta+Q` / `Alt+F4` |
| onboarding.continue | `Space` |
| onboarding.nextStyle / prevStyle (new) | `Tab` / `Shift+Tab` |
| login.provider.* | provider keys |
| list.downArrow / list.upArrow | `ArrowDown` / `ArrowUp` |
| detail.scroll{Down,Up,Left,Right}Arrow | arrows |
| detail.inlineReply | `Enter` |
| list.open | `Enter` |
| filter.needsAction / touchesYou / open / draft / all / close | `n` / `y` / `o` / `d` / `a` / `Escape` |
| threads.resolve / unresolve | `r` / `u` |

The filter and threads overlays are menus, so their letter choices stay the
same in every style. `filter.toggle` uses the style's `list.filter` key.

### Global, list, detail, diff, scroll, threads

| actionId | vim (today) | emacs | standard |
|---|---|---|---|
| global.help | `Shift+/` | `C-h` | `F1` |
| global.command | `Shift+;` | `M-x` | `Mod+K` |
| global.cancel (new) | — | `C-g` | — |
| global.search | `Mod+/` | `C-c /` | `Mod+/` |
| list.down / list.up | `j` / `k` | `C-n` / `C-p` | — (arrows) |
| list.top / list.bottom | `g g` / `Shift+G` | `M-<` / `M->` | `Home` / `End` |
| list.pageDown / pageUp | `C-d` / `C-u` | `C-v` / `M-v` | `PageDown` / `PageUp` |
| list.jumpTo1–9 | `1`–`9` | `M-1`–`M-9` | · |
| inbox.lane1–6 | `g 1`–`g 6` | `C-c 1`–`C-c 6` | `Mod+1`–`Mod+6` |
| list.search | `/` | `C-s` | `Mod+F` |
| list.filter / filter.toggle | `f` | `C-c C-f` | `Mod+Shift+F` |
| inbox.markLaneRead | `Shift+R` | `C-c C-r` | · |
| list.openBrowser | `o` | `C-c C-o` | · |
| list.toggleGroup | `Space` | `Tab` | · |
| list.draft / mute / star | `d` / `m` / `s` | `C-c C-d` / `C-c C-m` / `C-c C-s` | · |
| detail.nextFile / prevFile | `n` / `p` | `M-n` / `M-p` | `Ctrl+Tab` / `Ctrl+Shift+Tab` |
| detail.viewDiff | `Space` | `Enter` | `Enter` |
| detail.threads | `g u` | `C-c C-t` | `t` |
| detail.dismissNits / dismissBots | `d n` / `d b` | `C-c C-d n` / `C-c C-d b` | · |
| detail.undoDismiss | `u` | `C-/` | `Mod+Z` |
| detail.undoDismissAlt (new alias) | — | `C-x u` | — |
| detail.filter{Critical,Warning,Suggestion,Nit,Human,Clear} | `f c/w/s/n/h/f` | `C-c C-f c/w/s/n/h/f` | · |
| detail.approve | `a a` | `C-c C-a` | `a` (approve already confirms) |
| detail.comment | `c` | `C-c C-c` | · |
| detail.reply | `r` | `C-c C-r` | · |
| detail.requestChanges | `Shift+R` | `C-c C-w` | · |
| diff.nextFile / prevFile | `Shift+J` / `Shift+K` | `C-M-n` / `C-M-p` | `Ctrl+Tab` / `Ctrl+Shift+Tab` |
| diff.nextComment / prevComment | `n` / `p` | `M-n` / `M-p` | · |
| diff.sinceLastReview | `s` | `C-c C-l` | · |
| diff.toggleStyle | `Shift+S` | `C-c C-s` | · |
| diff.openFileBrowser | `o` | `C-c C-o` | · |
| diff.toggleCollapse | `z` | `Tab` | · |
| diff.toggleWhitespace | `w` | `C-c C-w` | · |
| diff.markViewed | `v` | `C-c C-v` | · |
| detail.scrollDown / scrollUp | `j` / `k` | `C-n` / `C-p` | — (arrows) |
| detail.scrollLeft / scrollRight | `h` / `l` | `C-b` / `C-f` | — (arrows) |
| detail.pageDown / pageUp | `C-d` / `C-u` | `C-v` / `M-v` | `PageDown` / `PageUp` |
| detail.scrollTop / scrollBottom | `0` / `Shift+4` | `M-<` / `M->` | `Home` / `End` |
| threads.next / prev | `j` / `k` | `C-n` / `C-p` | `ArrowDown` / `ArrowUp` |

Arrows work for horizontal and vertical scrolling in every style, through
the shared alias actions.

### Verify during implementation

- Every `M-` binding on macOS (`M-x`, `M-v`, `M-n`, `M-<`, `M-1`–`M-9`, …):
  Option rewrites `event.key` (`Alt+X` → `≈`). Confirm that TanStack matches
  on the physical key. If it doesn't, pick alternatives and update this table.
- A single key that equals a later step of a sequence in the same context
  (emacs `C-f` scroll vs `C-c C-f c`; today's vim `n` vs `f n`). Confirm
  whether TanStack also fires the single key mid-sequence. If it does and
  today's vim behaves the same, keep it consistent. Otherwise, add the case
  to the collision test.
- Emacs moves global search to `C-c /` because `Mod+/` is `Control+/` on
  Windows/Linux, which would collide with `C-/` undo in the detail context.
- Electron app-menu accelerators (`Mod+K`, `Mod+Shift+F`, `Mod+1–6`,
  `Mod+Z`). Make sure none is swallowed by a menu role. Change the key if one
  is.
- `C-c C-d` (draft, list) vs `C-c C-d n` (detail) live in different
  contexts. Within one context, no binding may be a strict prefix of another
  sequence. The collision test enforces this.

## Onboarding

`/onboarding` becomes two steps held in local component state (no new
route):

1. **Navigation style** (new)
2. **Development folder** (existing content)

Each step has a clickable **Continue** button next to a `Space` hint. Both
trigger `onboarding.continue`. The last step's Continue finishes onboarding,
which is today's `onboardingComplete` path. The hardcoded "standard vim-like
progression" copy (`OnboardingPage.tsx:176`) is rewritten to be style-neutral.

### Step 1 layout

- Three style cards: Vim / Emacs / Standard, each with a one-line pitch. The
  Emacs card adds the copy/cut caveat for Windows/Linux. Vim is preselected.
- Pick a card by clicking it or with `Tab` / `Shift+Tab`. Picking **persists
  `navigation.style` immediately** through `updateSettings`, so the real
  hotkey layer uses it right away. There is no separate preview mode.
- Below the cards is a sandbox with a checklist beside it.

### Sandbox

A fake inbox (5 PR rows) opens into a fake PR detail (summary plus 2 files),
which opens into a fake diff (2 files, 1 inline comment). It mirrors the
real list → detail → diff flow.

Checklist (ticks when the action fires; it never blocks Continue):

1. Move down 3 times (`list.down` / `list.downArrow`)
2. Jump to the bottom (`list.bottom`)
3. Open a PR (`list.open`)
4. Approve (`detail.approve`; the sandbox only ticks it, no confirm, no request)
5. View the diff (`detail.viewDiff`)
6. Next file (`diff.nextFile`)
7. Next comment (`diff.nextComment`)
8. Go back (`global.escape` / `global.cancel`)

Each item shows the active style's keys through `formatBindingKeys`, so
overrides render correctly. Switching style resets the checklist and returns
the sandbox to the list pane.

### Key routing

The sandbox registers **no** hotkeys of its own, so `useAppHotkeys` stays
the only registration point. It has one branch instead:

- `SANDBOX_ACTION_IDS`: the action ids in the checklist, plus their
  prev/up/alias siblings.
- On route `onboarding`, a binding is enabled when either
  - it is an onboarding-route binding (`onboarding.*`), or
  - it is in `SANDBOX_ACTION_IDS` **and** `isActive(binding, { routeBase:
    sandbox.pane, activeOverlay })`. The sandbox pane (`'list' | 'detail' |
    'diff'`) acts as a virtual route, so the real activation rules and
    context overlaps apply unchanged.
- Dispatch on the onboarding route goes to a `SANDBOX_ACTIONS` table instead
  of `getBindingAction`.
- `onboarding.continue` (`Space`) is enabled only while the sandbox is on
  its **list** pane. Inside an opened fake PR, `Space` belongs to the PR
  (vim `detail.viewDiff`). Esc goes back to the list pane, where `Space`
  continues again. The Continue button always works.

State: an `onboardingSandbox` slice in `appStore` (`pane`, `cursor`,
`fileIndex`, `commentIndex`, `completed: readonly string[]`). Transitions go
through a pure reducer in `utils/onboardingSandbox.ts`; writes go through a
service, following the convention that appStore writes live in `services/`.
The slice resets when the onboarding page unmounts.

## Settings → Keyboard

A style selector (the same three cards, clickable) sits at the top of
`pages/settings/KeyboardSection.tsx`, above the existing grouped binding
list. That list already renders from `getEffectiveBindings`, so it follows
the selected style. The sandbox is not shown here.

## Docs

- `website/src/pages/docs/keybindings.mdx`: per-style tables (generated or
  hand-written from `keymaps.ts`), plus `navigation.style` in the
  `settings.json` section.
- `CLAUDE.md`: update the keyboard-convention line to say keys come from
  per-style tables in `utils/keymaps.ts`, with `keybindings.ts` as the
  action catalog.
- `docs/DESIGN.md`: add the onboarding step-1 layout if the onboarding
  mockup is described there.

## Testing

Only tests that fail when real logic breaks:

1. **Keymap integrity** (`keymaps.test.ts`), for each style:
   - every key-table entry references a catalog `actionId`;
   - no two bindings enabled in the same route/overlay context share a key;
   - no sequence is a strict prefix of another sequence or single key in the
     same context.

   This guards the full tables and future additions.
2. **`getEffectiveBindings(style, overrides)`:** an override beats the style
   table, an empty override unbinds, and an action missing from the table is
   unbound.
3. **Settings normalise:** an old file without `navigation` normalises to
   `{ style: 'vim' }`; an invalid style falls back to the default.
4. **Sandbox reducer:** open/back pane transitions, cursor bounds, and that
   each checklist action ticks once.

No React component tests (project convention). Check onboarding and Settings
→ Keyboard by hand in the running app, in all three styles.

## Out of scope (spec 2: mouse affordances)

- Select diff line(s) with the mouse → comment popover (depends on what
  `@pierre/diffs` exposes for line selection).
- Approve / Request changes buttons in the PR detail view.
- Click to select a diff file or comment.
- Style-dependent visibility of those affordances, if any.
