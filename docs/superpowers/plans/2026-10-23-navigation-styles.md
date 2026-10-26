# Navigation Styles + Onboarding Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users choose a vim, emacs, or standard keyboard style; each style is a full default keymap; onboarding gains a step to pick a style and try it in a sandbox.

**Architecture:** `utils/keybindings.ts` becomes an action catalog with no keys. Keys come from per-style tables in `utils/keymaps.ts`, and user overrides are applied on top. A new `navigation.style` setting selects the table. The onboarding sandbox reuses the real hotkey layer: `useAppHotkeys` evaluates activation against a virtual route (the sandbox's pane) and dispatches to a pure sandbox reducer.

**Tech Stack:** Electron + React 19 + TypeScript, `@tanstack/react-hotkeys` / `@tanstack/hotkeys` 0.11.0, `@tanstack/store`, zod, `@thourum/fault`, Vitest, Biome.

**Spec:** `docs/superpowers/specs/2026-10-23-navigation-styles-design.md` (read it; the full keymap tables there are authoritative).

## Global Constraints

- Run all pnpm commands from `electron/`. Verify with: `pnpm type-check && pnpm lint && pnpm test`.
- Electron TS: **never write `try`/`catch`/`throw`** in committed code. Use `@thourum/fault` `Result`/`ResultAsync` with tagged faults.
- Validate external data with zod. No `as` casts on external data (test fixtures may cast).
- Hotkeys are registered **only** in `hooks/useAppHotkeys.ts`. Keybinding config stays pure data: no handlers in `keybindings.ts` / `keymaps.ts`.
- appStore writes live in `services/`. Hooks own React state/effects.
- No React component tests (project convention). Test utils and reducers only. Write a test only where it would fail if the logic broke.
- Use the project's design tokens/classes (`bg-surface-container*`, `text-on-surface*`, `border-outline-variant*`, `text-primary`, `mono-text`, the `Kbd` / `Button` components). No raw hex.
- Default style is `vim` and must behave **exactly** as today.
- Hotkey string format: modifiers capitalised with uppercase letters (`Control+N`, `Alt+Shift+,`), bare letters lowercase (`j`, `['d','n']`), the same as the existing file.
- Commit format: `<type>: <description>` (feat, fix, refactor, docs, test, chore).

---

## File map

| File | Responsibility |
|---|---|
| `electron/src/types/settings.ts` | `navigation` settings section, `NavigationStyle`, `NAVIGATION_STYLES` |
| `electron/src/main/ipc.ts` | telemetry line for `navigation` |
| `electron/src/main/settings.test.ts` (new) | normalise fills / validates `navigation` |
| `electron/src/renderer/utils/keybindings.ts` | action catalog (no keys), `getEffectiveBindings(style, overrides)` |
| `electron/src/renderer/utils/keymaps.ts` (new) | `KEYMAPS` (shared + vim + emacs + standard tables), `SEQUENCE_TIMEOUT_MS` |
| `electron/src/renderer/utils/keymaps.test.ts` (new) | keymap integrity: known ids, collisions, prefixes, valid strings, macOS Option matching |
| `electron/src/renderer/utils/keybindingActions.ts` | handlers for new actions |
| `electron/src/renderer/utils/onboardingSandbox.ts` (new) | sandbox state, tasks, pure reducer |
| `electron/src/renderer/utils/onboardingSandbox.test.ts` (new) | reducer tests |
| `electron/src/renderer/utils/keybindingActivation.ts` | `isBindingEnabled` (sandbox-aware activation) |
| `electron/src/renderer/services/onboardingSandbox.ts` (new) | open/close/dispatch sandbox in appStore |
| `electron/src/renderer/services/navigationStyle.ts` (new) | `setNavigationStyle`, `cycleNavigationStyle` |
| `electron/src/renderer/store/ui.ts` | `onboardingSandbox` slice; `onboarding_continue` confirming action |
| `electron/src/renderer/hooks/useAppHotkeys.ts` | style-aware bindings, timeout, sandbox routing |
| `electron/src/renderer/components/NavigationStylePicker.tsx` (new) | three style cards (onboarding + settings) |
| `electron/src/renderer/pages/OnboardingNavigationStep.tsx` (new) | step 1: picker + sandbox + checklist |
| `electron/src/renderer/pages/OnboardingSandbox.tsx` (new) | fake list/detail/diff rendering |
| `electron/src/renderer/pages/OnboardingPage.tsx` | two-step flow, Continue button |
| `electron/src/renderer/pages/settings/KeyboardSection.tsx` | style picker at the top |
| Callers: `components/HelpOverlay.tsx`, `components/TourOverlay.tsx`, `components/AppFooter.tsx` | pass style to `getEffectiveBindings` |
| `website/src/pages/docs/keybindings.mdx`, `CLAUDE.md`, `docs/DESIGN.md` | docs |

---

### Task 1: `navigation` settings section

**Files:**
- Modify: `electron/src/types/settings.ts`
- Modify: `electron/src/main/ipc.ts` (`trackSettingsPatch`, ~line 265)
- Create: `electron/src/main/settings.test.ts`

**Interfaces:**
- Produces:
  - `export const NAVIGATION_STYLES = ['vim', 'emacs', 'standard'] as const;`
  - `export type NavigationStyle = (typeof NAVIGATION_STYLES)[number];`
  - `AppSettings['navigation']` = `{ style: NavigationStyle }`, default `{ style: 'vim' }`
  - `export type NavigationSettings`

- [ ] **Step 1: Write the failing test** (`electron/src/main/settings.test.ts`)

`main/settings.ts` imports `electron` and `electron-store` at module load. Mock both, following the `vi.mock('electron', …)` block at the top of `src/main/ipc.test.ts`. Copy whatever that block stubs, then add `vi.mock('electron-store', () => ({ default: class {} }))`.

```ts
import { describe, expect, it, vi } from 'vitest';

vi.mock('electron', () => ({
  app: { getPath: vi.fn(() => '/tmp'), on: vi.fn() },
  BrowserWindow: { getAllWindows: vi.fn(() => []) },
  shell: { openPath: vi.fn() },
}));
vi.mock('electron-store', () => ({ default: class {} }));

import { normalise } from './settings';

describe('normalise navigation settings', () => {
  it('defaults navigation.style to vim for settings files written before the section existed', () => {
    expect(normalise({ appearance: { theme: 'dark' } }).navigation).toEqual({
      style: 'vim',
    });
  });

  it('keeps a valid style and drops an invalid one back to the default', () => {
    expect(normalise({ navigation: { style: 'emacs' } }).navigation.style).toBe('emacs');
    expect(normalise({ navigation: { style: 'nano' } }).navigation.style).toBe('vim');
  });
});
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `cd electron && pnpm vitest run src/main/settings.test.ts`
Expected: FAIL (`navigation` is undefined / a type error). If it fails because of a module-load error in the mocks instead, fix the mocks first so the failure is the missing `navigation`.

- [ ] **Step 3: Implement the schema** in `electron/src/types/settings.ts`

Add this next to the other section schemas:

```ts
export const NAVIGATION_STYLES = ['vim', 'emacs', 'standard'] as const;
const navigationSchema = z.object({ style: z.enum(NAVIGATION_STYLES) });
export type NavigationStyle = (typeof NAVIGATION_STYLES)[number];
export type NavigationSettings = z.infer<typeof navigationSchema>;
```

In `appSettingsSchema`, add `navigation: navigationSchema,` after `classification`. In `DEFAULT_SETTINGS`, add `navigation: { style: 'vim' },` in the same position.

- [ ] **Step 4: Add telemetry** in `electron/src/main/ipc.ts` `trackSettingsPatch`, following the existing pattern:

```ts
  if (patch.navigation !== undefined) {
    void telemetry?.track('settings_updated', {
      section: 'navigation',
      style: next.navigation.style,
    });
  }
```

If the telemetry event's property type rejects `style`, drop that property and keep only `section: 'navigation'`.

- [ ] **Step 5: Check the renderer's local-storage path**

Read `mergeLocalSettings` in `electron/src/renderer/store/settings.ts`. If it merges section by section from `DEFAULT_SETTINGS` generically, nothing changes. If it lists sections explicitly, add `navigation` the same way as the others.

- [ ] **Step 6: Run the tests and checks**

Run: `cd electron && pnpm vitest run src/main/settings.test.ts && pnpm type-check`
Expected: PASS. Type-check may surface fixtures that build a full `AppSettings` literal (e.g. in `ipc.test.ts`). Add `navigation: { style: 'vim' }` to each.

- [ ] **Step 7: Commit**

```bash
git add electron/src/types/settings.ts electron/src/main/ipc.ts electron/src/main/settings.test.ts electron/src/renderer/store/settings.ts
git commit -m "feat: add navigation.style setting"
```

---

### Task 2: Split the action catalog from the keys (vim only, behaviour-preserving)

**Files:**
- Modify: `electron/src/renderer/utils/keybindings.ts`
- Create: `electron/src/renderer/utils/keymaps.ts`
- Modify: `electron/src/renderer/hooks/useAppHotkeys.ts`, `components/HelpOverlay.tsx` (~line 150), `components/TourOverlay.tsx` (~line 39/68), `components/AppFooter.tsx` (~line 32/47), `pages/settings/KeyboardSection.tsx` (~line 15/17)
- Modify: `electron/src/renderer/utils/keybindings.test.ts`, plus any other test importing `DEFAULT_KEYBINDINGS` (`tourScreens.test.ts`, `legendPresets.test.ts`: grep for it)

**Interfaces:**
- Consumes: `NavigationStyle` from `types/settings.ts` (Task 1).
- Produces:
  - `keybindings.ts`: `export type BindingDefinition = Omit<KeyBinding, 'keys'>;`, `export const ACTION_CATALOG: readonly BindingDefinition[]`, `export function getEffectiveBindings(style: NavigationStyle, overrides: KeybindingOverrides): KeyBinding[]`. `KeyBinding` (with `keys`) stays the resolved output type, so consumers are unchanged. `DEFAULT_KEYBINDINGS` is **removed**.
  - `keymaps.ts`: `export type KeyTable = Readonly<Record<string, string | readonly string[]>>;`, `export const KEYMAPS: Readonly<Record<NavigationStyle, KeyTable>>`, `export const SEQUENCE_TIMEOUT_MS: Readonly<Record<NavigationStyle, number>>`.

- [ ] **Step 1: Snapshot today's keys** (throwaway, not committed)

Run:

```bash
cd electron && cat > /tmp/dump-keys.test.ts <<'EOF'
import { it, vi } from 'vitest';
vi.mock('../src/renderer/router', () => ({ router: { navigate: vi.fn() } }));
import { writeFileSync } from 'node:fs';
import { DEFAULT_KEYBINDINGS } from '../src/renderer/utils/keybindings';
it('dump', () => {
  writeFileSync('/tmp/keys-before.json', JSON.stringify(Object.fromEntries(DEFAULT_KEYBINDINGS.map((b) => [b.actionId, b.keys])), null, 1));
});
EOF
mkdir -p tests/tmp && cp /tmp/dump-keys.test.ts tests/tmp/dump-keys.test.ts && sed -i '' 's#\.\./src#../../src#g' tests/tmp/dump-keys.test.ts && pnpm vitest run tests/tmp/dump-keys.test.ts; rm -rf tests/tmp
```

Expected: `/tmp/keys-before.json` exists with about 80 entries. If the router mock path is wrong, the `vi.mock('../router')` in `keybindings.test.ts` shows the right target. Adjust and rerun.

- [ ] **Step 2: Create `keymaps.ts` with the shared and vim tables**

```ts
import type { NavigationStyle } from '../../types/settings';
import { PROVIDERS } from '../providers';

/** actionId → hotkey; a string is one hotkey, an array is a sequence. */
export type KeyTable = Readonly<Record<string, string | readonly string[]>>;

const LIST_JUMP_COUNT = 9;
const LANE_COUNT = 6;

function jumpKeys(format: (digit: number) => string): KeyTable {
  return Object.fromEntries(
    Array.from({ length: LIST_JUMP_COUNT }, (_, index) => [
      `list.jumpTo${index + 1}`,
      format(index + 1),
    ]),
  );
}

function laneKeys(
  format: (lane: number) => string | readonly string[],
): KeyTable {
  return Object.fromEntries(
    Array.from({ length: LANE_COUNT }, (_, index) => [
      `inbox.lane${index + 1}`,
      format(index + 1),
    ]),
  );
}

/** Identical in every style; see the spec's "Shared" table. */
const SHARED_KEYS: KeyTable = {
  'global.escape': 'Escape',
  'global.settings': 'Mod+,',
  'global.quit.mac': 'Meta+Q',
  'global.quit.win': 'Alt+F4',
  'onboarding.finish': 'Space',
  ...Object.fromEntries(
    PROVIDERS.map((provider) => [`login.provider.${provider.id}`, provider.key]),
  ),
  'list.downArrow': 'ArrowDown',
  'list.upArrow': 'ArrowUp',
  'list.open': 'Enter',
  'detail.scrollDownArrow': 'ArrowDown',
  'detail.scrollUpArrow': 'ArrowUp',
  'detail.scrollLeftArrow': 'ArrowLeft',
  'detail.scrollRightArrow': 'ArrowRight',
  'detail.inlineReply': 'Enter',
  'filter.needsAction': 'n',
  'filter.touchesYou': 'y',
  'filter.open': 'o',
  'filter.draft': 'd',
  'filter.all': 'a',
  'filter.close': 'Escape',
  'threads.resolve': 'r',
  'threads.unresolve': 'u',
};

const VIM_KEYS: KeyTable = {
  ...SHARED_KEYS,
  ...jumpKeys(String),
  ...laneKeys((lane) => ['g', String(lane)]),
  'global.help': 'Shift+/',
  'global.command': 'Shift+;',
  'global.search': 'Mod+/',
  'list.down': 'j',
  'list.up': 'k',
  'list.top': ['g', 'g'],
  'list.bottom': 'Shift+G',
  'list.search': '/',
  'list.filter': 'f',
  'filter.toggle': 'f',
  'list.pageDown': 'Control+D',
  'list.pageUp': 'Control+U',
  'inbox.markLaneRead': 'Shift+R',
  'list.openBrowser': 'o',
  'list.toggleGroup': 'Space',
  'list.draft': 'd',
  'list.mute': 'm',
  'list.star': 's',
  'detail.nextFile': 'n',
  'detail.prevFile': 'p',
  'detail.viewDiff': 'Space',
  'detail.threads': ['g', 'u'],
  'detail.dismissNits': ['d', 'n'],
  'detail.dismissBots': ['d', 'b'],
  'detail.undoDismiss': 'u',
  'detail.filterCritical': ['f', 'c'],
  'detail.filterWarning': ['f', 'w'],
  'detail.filterSuggestion': ['f', 's'],
  'detail.filterNit': ['f', 'n'],
  'detail.filterHuman': ['f', 'h'],
  'detail.filterClear': ['f', 'f'],
  'detail.approve': ['a', 'a'],
  'detail.comment': 'c',
  'detail.reply': 'r',
  'detail.requestChanges': 'Shift+R',
  'diff.sinceLastReview': 's',
  'diff.toggleStyle': 'Shift+S',
  'diff.openFileBrowser': 'o',
  'diff.toggleCollapse': 'z',
  'diff.toggleWhitespace': 'w',
  'diff.markViewed': 'v',
  'diff.nextFile': 'Shift+J',
  'diff.prevFile': 'Shift+K',
  'diff.nextComment': 'n',
  'diff.prevComment': 'p',
  'detail.scrollLeft': 'h',
  'detail.scrollDown': 'j',
  'detail.scrollUp': 'k',
  'detail.scrollRight': 'l',
  'detail.pageDown': 'Control+D',
  'detail.pageUp': 'Control+U',
  'detail.scrollTop': '0',
  'detail.scrollBottom': 'Shift+4',
  'threads.next': 'j',
  'threads.prev': 'k',
};

/**
 * Per-style key tables. emacs/standard point at vim until their tables land
 * in the next task; the style can't be selected in the UI before then.
 */
export const KEYMAPS = {
  vim: VIM_KEYS,
  emacs: VIM_KEYS,
  standard: VIM_KEYS,
} satisfies Record<NavigationStyle, KeyTable>;

/** Sequence steps must follow each other within this window. */
export const SEQUENCE_TIMEOUT_MS = {
  vim: 500,
  emacs: 500,
  standard: 500,
} satisfies Record<NavigationStyle, number>;
```

- [ ] **Step 3: Turn `keybindings.ts` into a catalog**

- Add `import type { KeybindingOverrides, NavigationStyle } from '../../types/settings';` and `import { KEYMAPS } from './keymaps';`.
- Add `export type BindingDefinition = Omit<KeyBinding, 'keys'>;`.
- Change every `*_BINDINGS` array type from `readonly KeyBinding[]` to `readonly BindingDefinition[]`, and **delete every `keys:` property** in them, including in the `JUMP_BINDINGS`, `LANE_JUMP_BINDINGS` and `LOGIN_BINDINGS` generators. Keep `actionId`, `description`, `activation` and `hidden` exactly as they are.
- Rename `DEFAULT_KEYBINDINGS` to `ACTION_CATALOG` (type `readonly BindingDefinition[]`), and update its doc comment to: `/** Every bindable action; keys come from the selected style's table in keymaps.ts. */`
- Replace `getEffectiveBindings` with:

```ts
/** Resolve keys: user override (empty = unbound) → style table → unbound. */
export function getEffectiveBindings(
  style: NavigationStyle,
  overrides: KeybindingOverrides,
): KeyBinding[] {
  const table = KEYMAPS[style];
  return ACTION_CATALOG.flatMap((definition) => {
    const override = overrides[definition.actionId];
    if (override !== undefined) {
      if (override.length === 0) return [];
      const keys = override.length === 1 ? override[0] : override;
      return keys === undefined ? [] : [{ ...definition, keys }];
    }
    const keys = table[definition.actionId];
    return keys === undefined ? [] : [{ ...definition, keys }];
  });
}
```

- [ ] **Step 4: Pass the style at every call site**

In each caller, next to the existing `const overrides = useStore(settingsStore, (s) => s.keybindings);`, add `const style = useStore(settingsStore, (s) => s.navigation.style);`. Change `getEffectiveBindings(overrides)` to `getEffectiveBindings(style, overrides)` and add `style` to the surrounding `useMemo` dependency arrays. Callers: `HelpOverlay.tsx`, `TourOverlay.tsx`, `AppFooter.tsx`, `KeyboardSection.tsx`, `useAppHotkeys.ts`.

In `useAppHotkeys.ts`, also:
- delete the local `SEQUENCE_TIMEOUT_MS` constant and import it from `../utils/keymaps`;
- pass `timeout: SEQUENCE_TIMEOUT_MS[style]` to `useHotkeySequences`.

- [ ] **Step 5: Update the tests** that used `DEFAULT_KEYBINDINGS`

In `keybindings.test.ts`, replace `DEFAULT_KEYBINDINGS` with `getEffectiveBindings('vim', {})`. The `binding()` helper searches that array, and the "resolves an action for every configured binding" test should loop over `ACTION_CATALOG` so unbound actions are covered too. Do the same in any other test that imports `DEFAULT_KEYBINDINGS`. Add one test here:

```ts
describe('getEffectiveBindings', () => {
  it('lets a user override beat the style table and an empty override unbind', () => {
    const bindings = getEffectiveBindings('vim', {
      'list.down': ['x'],
      'list.mute': [],
      'list.top': ['g', 'h'],
    });
    const keysOf = (id: string) => bindings.find((b) => b.actionId === id)?.keys;
    expect(keysOf('list.down')).toBe('x');
    expect(keysOf('list.top')).toEqual(['g', 'h']);
    expect(keysOf('list.mute')).toBeUndefined();
    expect(keysOf('list.up')).toBe('k');
  });
});
```

- [ ] **Step 6: Prove vim is unchanged**

Rerun the Step 1 dump, now on `getEffectiveBindings('vim', {})` (change the import and expression), write the result to `/tmp/keys-after.json`, then run `diff /tmp/keys-before.json /tmp/keys-after.json`.
Expected: no output (key **order** may differ; if `diff` complains only about order, compare with `node -e` after sorting keys). Delete the temporary test file afterwards.

- [ ] **Step 7: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add electron/src/renderer
git commit -m "refactor: separate keybinding action catalog from per-style key tables"
```

---

### Task 3: Emacs and standard keymaps, new alias actions, integrity tests

**Files:**
- Modify: `electron/src/renderer/utils/keymaps.ts`
- Modify: `electron/src/renderer/utils/keybindings.ts` (two catalog entries)
- Modify: `electron/src/renderer/utils/keybindingActions.ts` (two handlers)
- Create: `electron/src/renderer/utils/keymaps.test.ts`

**Interfaces:**
- Consumes: `KEYMAPS`, `KeyTable`, `SEQUENCE_TIMEOUT_MS`, `ACTION_CATALOG`, `getEffectiveBindings`, `isActive` (Task 2).
- Produces: catalog actions `global.cancel` and `detail.undoDismissAlt`; real `KEYMAPS.emacs` and `KEYMAPS.standard`; `SEQUENCE_TIMEOUT_MS.emacs = 1500`.

- [ ] **Step 1: Write the failing integrity tests** (`keymaps.test.ts`)

```ts
import { matchesKeyboardEvent, normalizeHotkey, validateHotkey } from '@tanstack/hotkeys';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({ router: { navigate: vi.fn() } }));

import { NAVIGATION_STYLES } from '../../types/settings';
import type { OverlayKind } from '../overlays/OverlayContext';
import { ACTION_CATALOG, getEffectiveBindings, isActive, type RouteBase } from './keybindings';
import { KEYMAPS } from './keymaps';

const PLATFORMS = ['mac', 'windows', 'linux'] as const;
const ROUTES: readonly RouteBase[] = ['list', 'detail', 'diff', 'settings', 'login', 'onboarding'];
const OVERLAYS: readonly OverlayKind[] = ['command', 'help', 'search', 'filter', 'repoPicker', 'addRepo', 'threads', 'tour'];
const CONTEXTS = [
  ...ROUTES.map((routeBase) => ({ routeBase, activeOverlay: null })),
  ...OVERLAYS.map((activeOverlay) => ({ routeBase: 'list' as const, activeOverlay })),
];
/** Pairs that intentionally share a key in one context. */
const ALLOWED_SHARED = new Set(['filter.close|global.escape']);

const steps = (keys: string | readonly string[]): readonly string[] =>
  typeof keys === 'string' ? [keys] : keys;

describe.each(NAVIGATION_STYLES)('%s keymap', (style) => {
  it('only maps actions that exist in the catalog', () => {
    const known = new Set(ACTION_CATALOG.map((definition) => definition.actionId));
    expect(Object.keys(KEYMAPS[style]).filter((id) => !known.has(id))).toEqual([]);
  });

  it('uses only valid hotkey strings', () => {
    for (const keys of Object.values(KEYMAPS[style])) {
      for (const step of steps(keys)) {
        expect(validateHotkey(step).errors, step).toEqual([]);
      }
    }
  });

  it.each(PLATFORMS)('has no ambiguous keys in any context on %s', (platform) => {
    for (const context of CONTEXTS) {
      const bindings = getEffectiveBindings(style, {}).filter((b) => isActive(b, context));
      const byKey = new Map<string, string[]>();
      for (const binding of bindings) {
        const key = steps(binding.keys).map((step) => normalizeHotkey(step, platform)).join(' ');
        byKey.set(key, [...(byKey.get(key) ?? []), binding.actionId]);
      }
      for (const [key, ids] of byKey) {
        const pair = ids.toSorted().join('|');
        if (ids.length > 1 && !ALLOWED_SHARED.has(pair)) {
          expect.fail(`${style}/${platform} ${JSON.stringify(context)}: "${key}" → ${pair}`);
        }
      }
      const keys = [...byKey.keys()];
      for (const key of keys) {
        for (const other of keys) {
          if (other !== key && other.startsWith(`${key} `)) {
            expect.fail(`${style}/${platform} ${JSON.stringify(context)}: "${key}" is a prefix of "${other}"`);
          }
        }
      }
    }
  });
});

describe('emacs Meta bindings on macOS', () => {
  // Option rewrites event.key (Alt+X → "≈"); TanStack must fall back to event.code.
  const CODE_BY_KEY: Readonly<Record<string, string>> = { ',': 'Comma', '.': 'Period', '/': 'Slash' };
  const codeFor = (key: string): string =>
    CODE_BY_KEY[key] ?? (/^\d$/.test(key) ? `Digit${key}` : `Key${key.toUpperCase()}`);

  it('matches every Alt+ step from its physical key', () => {
    for (const keys of Object.values(KEYMAPS.emacs)) {
      for (const step of steps(keys).filter((s) => s.includes('Alt+'))) {
        const parts = step.split('+');
        const key = parts[parts.length - 1] ?? '';
        const event = {
          type: 'keydown',
          key: '∆',
          code: codeFor(key),
          altKey: true,
          shiftKey: parts.includes('Shift'),
          ctrlKey: parts.includes('Control'),
          metaKey: false,
          isComposing: false,
        } as unknown as KeyboardEvent;
        expect(matchesKeyboardEvent(event, step, 'mac'), step).toBe(true);
      }
    }
  });
});
```

Notes for the implementer:
- `ACTION_CATALOG`, `isActive` and `RouteBase` are exported from `keybindings.ts`. If `OverlayKind` lives elsewhere, import it from where `keybindings.ts` does.
- If the arrays `ROUTES` / `OVERLAYS` drift from `ALL_ROUTES` / `ALL_OVERLAYS` in `keybindings.ts`, export those two constants from `keybindings.ts` and use them here instead.

- [ ] **Step 2: Run the tests**

Run: `cd electron && pnpm vitest run src/renderer/utils/keymaps.test.ts`
Expected: the vim cases PASS; emacs/standard pass trivially (they still point at vim); the macOS Meta test passes vacuously (no `Alt+` yet). **If a vim case fails**, that's an existing collision. Stop and report it to the controller with the message text; do not change vim keys.

- [ ] **Step 3: Add the two alias actions**

In `keybindings.ts` `GLOBAL_BINDINGS`, right after `global.escape`:

```ts
  {
    actionId: 'global.cancel',
    description: 'Back / Cancel',
    activation: {
      routes: ['list', 'detail', 'diff', 'settings'],
      overlays: ALL_OVERLAYS,
    },
    hidden: true,
  },
```

Add a `detail.undoDismissAlt` entry right after `detail.undoDismiss`: copy that entry's `activation`, use the same description, and add `hidden: true`.

In `keybindingActions.ts` `ACTIONS`:
- next to `'global.escape': handleEscape,` add `'global.cancel': handleEscape,`;
- next to `'detail.undoDismiss'` add `'detail.undoDismissAlt': () => emitAppEvent('bistro:comments:undo-dismiss'),`.

The Escape-only special casing in `useAppHotkeys` (`isEscape`) keys off `keys === 'Escape'`, so `C-g` correctly doesn't get it. Leave that code alone.

- [ ] **Step 4: Add the emacs and standard tables** to `keymaps.ts` (above `KEYMAPS`)

```ts
const EMACS_KEYS: KeyTable = {
  ...SHARED_KEYS,
  ...jumpKeys((digit) => `Alt+${digit}`),
  ...laneKeys((lane) => ['Control+C', String(lane)]),
  'global.help': 'Control+H',
  'global.command': 'Alt+X',
  'global.cancel': 'Control+G',
  // Mod+/ is Control+/ on Windows/Linux, which is undo here.
  'global.search': ['Control+C', '/'],
  'list.down': 'Control+N',
  'list.up': 'Control+P',
  'list.top': 'Alt+Shift+,',
  'list.bottom': 'Alt+Shift+.',
  'list.search': 'Control+S',
  'list.filter': ['Control+C', 'Control+F'],
  'filter.toggle': ['Control+C', 'Control+F'],
  'list.pageDown': 'Control+V',
  'list.pageUp': 'Alt+V',
  'inbox.markLaneRead': ['Control+C', 'Control+R'],
  'list.openBrowser': ['Control+C', 'Control+O'],
  'list.toggleGroup': 'Tab',
  'list.draft': ['Control+C', 'Control+D'],
  'list.mute': ['Control+C', 'Control+M'],
  'list.star': ['Control+C', 'Control+S'],
  'detail.nextFile': 'Alt+N',
  'detail.prevFile': 'Alt+P',
  'detail.viewDiff': 'Enter',
  'detail.threads': ['Control+C', 'Control+T'],
  'detail.dismissNits': ['Control+C', 'Control+D', 'n'],
  'detail.dismissBots': ['Control+C', 'Control+D', 'b'],
  'detail.undoDismiss': 'Control+/',
  'detail.undoDismissAlt': ['Control+X', 'u'],
  'detail.filterCritical': ['Control+C', 'Control+F', 'c'],
  'detail.filterWarning': ['Control+C', 'Control+F', 'w'],
  'detail.filterSuggestion': ['Control+C', 'Control+F', 's'],
  'detail.filterNit': ['Control+C', 'Control+F', 'n'],
  'detail.filterHuman': ['Control+C', 'Control+F', 'h'],
  'detail.filterClear': ['Control+C', 'Control+F', 'f'],
  'detail.approve': ['Control+C', 'Control+A'],
  'detail.comment': ['Control+C', 'Control+C'],
  'detail.reply': ['Control+C', 'Control+R'],
  'detail.requestChanges': ['Control+C', 'Control+W'],
  'diff.sinceLastReview': ['Control+C', 'Control+L'],
  'diff.toggleStyle': ['Control+C', 'Control+S'],
  'diff.openFileBrowser': ['Control+C', 'Control+O'],
  'diff.toggleCollapse': 'Tab',
  'diff.toggleWhitespace': ['Control+C', 'Control+W'],
  'diff.markViewed': ['Control+C', 'Control+V'],
  'diff.nextFile': 'Control+Alt+N',
  'diff.prevFile': 'Control+Alt+P',
  'diff.nextComment': 'Alt+N',
  'diff.prevComment': 'Alt+P',
  'detail.scrollLeft': 'Control+B',
  'detail.scrollDown': 'Control+N',
  'detail.scrollUp': 'Control+P',
  'detail.scrollRight': 'Control+F',
  'detail.pageDown': 'Control+V',
  'detail.pageUp': 'Alt+V',
  'detail.scrollTop': 'Alt+Shift+,',
  'detail.scrollBottom': 'Alt+Shift+.',
  'threads.next': 'Control+N',
  'threads.prev': 'Control+P',
};

/** Arrows/Home/End/PgUp/PgDn/Ctrl+Tab move; actions keep vim's plain letters. */
const STANDARD_KEYS: KeyTable = {
  ...SHARED_KEYS,
  ...jumpKeys(String),
  ...laneKeys((lane) => `Mod+${lane}`),
  'global.help': 'F1',
  'global.command': 'Mod+K',
  'global.search': 'Mod+/',
  'list.top': 'Home',
  'list.bottom': 'End',
  'list.search': 'Mod+F',
  'list.filter': 'Mod+Shift+F',
  'filter.toggle': 'Mod+Shift+F',
  'list.pageDown': 'PageDown',
  'list.pageUp': 'PageUp',
  'inbox.markLaneRead': 'Shift+R',
  'list.openBrowser': 'o',
  'list.toggleGroup': 'Space',
  'list.draft': 'd',
  'list.mute': 'm',
  'list.star': 's',
  'detail.nextFile': 'Control+Tab',
  'detail.prevFile': 'Control+Shift+Tab',
  'detail.viewDiff': 'Enter',
  'detail.threads': 't',
  'detail.dismissNits': ['d', 'n'],
  'detail.dismissBots': ['d', 'b'],
  'detail.undoDismiss': 'Mod+Z',
  'detail.filterCritical': ['f', 'c'],
  'detail.filterWarning': ['f', 'w'],
  'detail.filterSuggestion': ['f', 's'],
  'detail.filterNit': ['f', 'n'],
  'detail.filterHuman': ['f', 'h'],
  'detail.filterClear': ['f', 'f'],
  // Approve already asks for confirmation, so one key is safe here.
  'detail.approve': 'a',
  'detail.comment': 'c',
  'detail.reply': 'r',
  'detail.requestChanges': 'Shift+R',
  'diff.sinceLastReview': 's',
  'diff.toggleStyle': 'Shift+S',
  'diff.openFileBrowser': 'o',
  'diff.toggleCollapse': 'z',
  'diff.toggleWhitespace': 'w',
  'diff.markViewed': 'v',
  'diff.nextFile': 'Control+Tab',
  'diff.prevFile': 'Control+Shift+Tab',
  'diff.nextComment': 'n',
  'diff.prevComment': 'p',
  'detail.pageDown': 'PageDown',
  'detail.pageUp': 'PageUp',
  'detail.scrollTop': 'Home',
  'detail.scrollBottom': 'End',
  'threads.next': 'ArrowDown',
  'threads.prev': 'ArrowUp',
};
```

Set `KEYMAPS = { vim: VIM_KEYS, emacs: EMACS_KEYS, standard: STANDARD_KEYS }` and delete the temporary doc-comment sentence about vim placeholders. Set `SEQUENCE_TIMEOUT_MS.emacs` to `1500` with the comment `// Prefix chords (C-c C-a) are typed slower than g g.`

- [ ] **Step 5: Run the integrity tests**

Run: `cd electron && pnpm vitest run src/renderer/utils/keymaps.test.ts`
Expected: PASS. If something fails:
- **collision or prefix failure in emacs/standard:** the table above has a mistake. Fix it by picking a key that follows the style's rules (emacs `C-c C-<mnemonic>`; standard arrow/desktop keys for movement), and list every key you changed in your report so the spec table can be updated.
- **validateHotkey error:** the string format is wrong (e.g. `Ctrl` vs `Control`). Fix the format, not the intended key.
- **macOS Meta test failure:** do **not** pick replacements yourself. Report the failing steps to the controller (the spec requires a decision).

- [ ] **Step 6: Verify everything**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. (`keybindings.test.ts`'s "resolves an action for every configured binding" now covers the two new actions.)

- [ ] **Step 7: Commit**

```bash
git add electron/src/renderer/utils
git commit -m "feat: add emacs and standard keymaps with integrity tests"
```

---

### Task 4: Sandbox reducer, sandbox-aware activation, hotkey routing

**Files:**
- Create: `electron/src/renderer/utils/onboardingSandbox.ts`, `electron/src/renderer/utils/onboardingSandbox.test.ts`
- Create: `electron/src/renderer/services/onboardingSandbox.ts`
- Modify: `electron/src/renderer/store/ui.ts` (`UIState`, `initialUIState`)
- Modify: `electron/src/renderer/utils/keybindingActivation.ts`
- Modify: `electron/src/renderer/hooks/useAppHotkeys.ts`
- Modify: `electron/src/renderer/utils/keybindings.test.ts` (an `isBindingEnabled` test)

**Interfaces:**
- Consumes: `getEffectiveBindings`, `isActive`, `KeyBinding`, `RouteBase` (Task 2); `global.cancel` (Task 3).
- Produces:
  - `utils/onboardingSandbox.ts`: `SandboxPane`, `SandboxTaskId`, `SandboxState`, `SandboxTask { id: SandboxTaskId; label: string; actionIds: readonly string[] }`, `SANDBOX_TASKS: readonly SandboxTask[]`, `SANDBOX_ACTION_IDS: ReadonlySet<string>`, `SANDBOX_PR_COUNT`, `SANDBOX_FILE_COUNT`, `SANDBOX_COMMENT_COUNT`, `INITIAL_SANDBOX_STATE: SandboxState`, `reduceSandbox(state: SandboxState, actionId: string): SandboxState`.
  - `UIState.onboardingSandbox: SandboxState | null` (initial `null`).
  - `services/onboardingSandbox.ts`: `openSandbox(): void` (also used as a reset), `closeSandbox(): void`, `dispatchSandboxAction(actionId: string): void`.
  - `keybindingActivation.ts`: `isBindingEnabled(binding: KeyBinding, state: { routeBase: RouteBase | null; activeOverlay: OverlayKind | null; sandboxPane: SandboxPane | null }): boolean`.

- [ ] **Step 1: Write the failing reducer tests** (`onboardingSandbox.test.ts`)

```ts
import { describe, expect, it } from 'vitest';
import {
  INITIAL_SANDBOX_STATE,
  reduceSandbox,
  SANDBOX_PR_COUNT,
  type SandboxState,
} from './onboardingSandbox';

const run = (actionIds: readonly string[], from: SandboxState = INITIAL_SANDBOX_STATE) =>
  actionIds.reduce(reduceSandbox, from);

describe('reduceSandbox', () => {
  it('ticks moveDown only after three down presses and clamps the cursor', () => {
    const twice = run(['list.down', 'list.downArrow']);
    expect(twice.completed).not.toContain('moveDown');
    const many = run(Array.from({ length: SANDBOX_PR_COUNT + 2 }, () => 'list.down'));
    expect(many.completed).toContain('moveDown');
    expect(many.cursor).toBe(SANDBOX_PR_COUNT - 1);
  });

  it('walks list → detail → diff and back with Escape or C-g, ticking each task once', () => {
    const state = run([
      'list.bottom',
      'list.open',
      'detail.approve',
      'detail.viewDiff',
      'diff.nextFile',
      'diff.nextComment',
      'global.escape',
      'global.cancel',
    ]);
    expect(state.pane).toBe('list');
    expect(state.completed.toSorted()).toEqual(
      ['approve', 'back', 'bottom', 'nextComment', 'nextFile', 'open', 'viewDiff'].toSorted(),
    );
  });

  it('ignores Escape on the list pane and open outside it', () => {
    expect(run(['global.escape'])).toEqual(INITIAL_SANDBOX_STATE);
    const inDetail = run(['list.open']);
    expect(run(['list.open'], inDetail)).toEqual(inDetail);
  });
});
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `cd electron && pnpm vitest run src/renderer/utils/onboardingSandbox.test.ts`
Expected: FAIL (module not found).

- [ ] **Step 3: Implement `utils/onboardingSandbox.ts`**

```ts
export type SandboxPane = 'list' | 'detail' | 'diff';
export type SandboxTaskId =
  | 'moveDown'
  | 'bottom'
  | 'open'
  | 'approve'
  | 'viewDiff'
  | 'nextFile'
  | 'nextComment'
  | 'back';

export interface SandboxState {
  readonly pane: SandboxPane;
  readonly cursor: number;
  readonly downPresses: number;
  readonly fileIndex: number;
  readonly commentIndex: number;
  readonly approved: boolean;
  readonly completed: readonly SandboxTaskId[];
}

export interface SandboxTask {
  readonly id: SandboxTaskId;
  readonly label: string;
  /** Checklist shows the first of these that the active style binds. */
  readonly actionIds: readonly string[];
}

export const SANDBOX_PR_COUNT = 5;
export const SANDBOX_FILE_COUNT = 2;
export const SANDBOX_COMMENT_COUNT = 2;
const DOWN_PRESSES_TO_COMPLETE = 3;

export const SANDBOX_TASKS: readonly SandboxTask[] = [
  { id: 'moveDown', label: 'Move down 3 times', actionIds: ['list.down', 'list.downArrow'] },
  { id: 'bottom', label: 'Jump to the bottom', actionIds: ['list.bottom'] },
  { id: 'open', label: 'Open a PR', actionIds: ['list.open'] },
  { id: 'approve', label: 'Approve it', actionIds: ['detail.approve'] },
  { id: 'viewDiff', label: 'View the diff', actionIds: ['detail.viewDiff'] },
  { id: 'nextFile', label: 'Next file', actionIds: ['diff.nextFile'] },
  { id: 'nextComment', label: 'Next comment', actionIds: ['diff.nextComment'] },
  { id: 'back', label: 'Go back', actionIds: ['global.escape', 'global.cancel'] },
];

export const INITIAL_SANDBOX_STATE: SandboxState = {
  pane: 'list',
  cursor: 0,
  downPresses: 0,
  fileIndex: 0,
  commentIndex: 0,
  approved: false,
  completed: [],
};

const clamp = (value: number, count: number): number =>
  Math.min(Math.max(value, 0), count - 1);

function complete(state: SandboxState, id: SandboxTaskId): SandboxState {
  return state.completed.includes(id)
    ? state
    : { ...state, completed: [...state.completed, id] };
}

function moveDown(state: SandboxState): SandboxState {
  const next = {
    ...state,
    cursor: clamp(state.cursor + 1, SANDBOX_PR_COUNT),
    downPresses: state.downPresses + 1,
  };
  return next.downPresses >= DOWN_PRESSES_TO_COMPLETE ? complete(next, 'moveDown') : next;
}

const moveUp = (state: SandboxState): SandboxState => ({
  ...state,
  cursor: clamp(state.cursor - 1, SANDBOX_PR_COUNT),
});

const PANE_BACK: Readonly<Record<SandboxPane, SandboxPane | null>> = {
  list: null,
  detail: 'list',
  diff: 'detail',
};

function goBack(state: SandboxState): SandboxState {
  const previous = PANE_BACK[state.pane];
  return previous === null ? state : complete({ ...state, pane: previous }, 'back');
}

const TRANSITIONS: Readonly<Record<string, (state: SandboxState) => SandboxState>> = {
  'list.down': moveDown,
  'list.downArrow': moveDown,
  'list.up': moveUp,
  'list.upArrow': moveUp,
  'list.top': (state) => ({ ...state, cursor: 0 }),
  'list.bottom': (state) => complete({ ...state, cursor: SANDBOX_PR_COUNT - 1 }, 'bottom'),
  'list.open': (state) =>
    state.pane !== 'list'
      ? state
      : complete({ ...state, pane: 'detail', fileIndex: 0, commentIndex: 0, approved: false }, 'open'),
  'detail.approve': (state) => complete({ ...state, approved: true }, 'approve'),
  'detail.viewDiff': (state) =>
    state.pane !== 'detail' ? state : complete({ ...state, pane: 'diff' }, 'viewDiff'),
  'diff.nextFile': (state) =>
    complete({ ...state, fileIndex: clamp(state.fileIndex + 1, SANDBOX_FILE_COUNT) }, 'nextFile'),
  'diff.prevFile': (state) => ({ ...state, fileIndex: clamp(state.fileIndex - 1, SANDBOX_FILE_COUNT) }),
  'diff.nextComment': (state) =>
    complete(
      { ...state, commentIndex: clamp(state.commentIndex + 1, SANDBOX_COMMENT_COUNT) },
      'nextComment',
    ),
  'diff.prevComment': (state) => ({
    ...state,
    commentIndex: clamp(state.commentIndex - 1, SANDBOX_COMMENT_COUNT),
  }),
  'global.escape': goBack,
  'global.cancel': goBack,
};

/** Actions the onboarding sandbox handles instead of the real app. */
export const SANDBOX_ACTION_IDS: ReadonlySet<string> = new Set(Object.keys(TRANSITIONS));

export function reduceSandbox(state: SandboxState, actionId: string): SandboxState {
  const transition = TRANSITIONS[actionId];
  return transition === undefined ? state : transition(state);
}
```

Note: the `'open'` test expects `run(['list.open'], inDetail)` to equal `inDetail`. The guard returns the same object, so `toEqual` passes.

- [ ] **Step 4: Run the reducer tests**

Run: `cd electron && pnpm vitest run src/renderer/utils/onboardingSandbox.test.ts`
Expected: PASS.

- [ ] **Step 5: Write the failing activation test** (append to `keybindings.test.ts`)

```ts
import { isBindingEnabled } from './keybindingActivation';

describe('isBindingEnabled on the onboarding route', () => {
  const vim = getEffectiveBindings('vim', {});
  const find = (id: string) => {
    const found = vim.find((b) => b.actionId === id);
    if (found === undefined) throw new Error(`Expected binding ${id}`);
    return found;
  };
  const onboarding = (sandboxPane: 'list' | 'detail' | 'diff' | null) => ({
    routeBase: 'onboarding' as const,
    activeOverlay: null,
    sandboxPane,
  });

  it('activates sandbox actions by the sandbox pane, not the real route', () => {
    expect(isBindingEnabled(find('list.down'), onboarding('list'))).toBe(true);
    expect(isBindingEnabled(find('list.down'), onboarding('detail'))).toBe(false);
    expect(isBindingEnabled(find('diff.nextFile'), onboarding('diff'))).toBe(true);
    expect(isBindingEnabled(find('list.mute'), onboarding('list'))).toBe(false);
    expect(isBindingEnabled(find('list.down'), onboarding(null))).toBe(false);
  });

  it('only lets Space continue from the sandbox list pane', () => {
    expect(isBindingEnabled(find('onboarding.finish'), onboarding('list'))).toBe(true);
    expect(isBindingEnabled(find('onboarding.finish'), onboarding('detail'))).toBe(false);
    expect(isBindingEnabled(find('onboarding.finish'), onboarding(null))).toBe(true);
  });
});
```

(Task 5 renames `onboarding.finish` to `onboarding.continue`, along with this test.)

- [ ] **Step 6: Run it and confirm it fails**

Run: `cd electron && pnpm vitest run src/renderer/utils/keybindings.test.ts`
Expected: FAIL (`isBindingEnabled` is not exported).

- [ ] **Step 7: Implement `isBindingEnabled`** in `keybindingActivation.ts`

```ts
import { SANDBOX_ACTION_IDS, type SandboxPane } from './onboardingSandbox';

/** Onboarding actions that only make sense while the sandbox step is showing. */
const SANDBOX_STEP_ACTION_IDS: ReadonlySet<string> = new Set([
  'onboarding.nextStyle',
  'onboarding.prevStyle',
]);
const CONTINUE_ACTION_IDS: ReadonlySet<string> = new Set([
  'onboarding.finish',
  'onboarding.continue',
]);

/**
 * `isActive`, plus the onboarding sandbox: while it is mounted, sandbox
 * actions activate as if the app were on the sandbox's pane.
 */
export function isBindingEnabled(
  binding: KeyBinding,
  {
    routeBase,
    activeOverlay,
    sandboxPane,
  }: {
    routeBase: RouteBase | null;
    activeOverlay: OverlayKind | null;
    sandboxPane: SandboxPane | null;
  },
): boolean {
  const real = { routeBase, activeOverlay };
  if (routeBase !== 'onboarding') return isActive(binding, real);
  if (sandboxPane === null) {
    return !SANDBOX_STEP_ACTION_IDS.has(binding.actionId) && isActive(binding, real);
  }
  // In an opened fake PR, Space belongs to the PR (vim detail.viewDiff).
  if (CONTINUE_ACTION_IDS.has(binding.actionId)) {
    return sandboxPane === 'list' && isActive(binding, real);
  }
  if (SANDBOX_ACTION_IDS.has(binding.actionId)) {
    return isActive(binding, { routeBase: sandboxPane, activeOverlay });
  }
  return isActive(binding, real);
}
```

Re-export it from `keybindings.ts` next to `isActive`: `export { isActive, isBindingEnabled, routeBaseFromPath } from './keybindingActivation';`. Task 5 removes `'onboarding.finish'` from `CONTINUE_ACTION_IDS`.

- [ ] **Step 8: Add the store slice and the service**

`store/ui.ts`: add `onboardingSandbox: SandboxState | null;` to `UIState` with doc comment `/** Onboarding style sandbox; null when the step isn't showing. */`, and `onboardingSandbox: null,` to `initialUIState`. Import the type from `../utils/onboardingSandbox`.

`services/onboardingSandbox.ts`:

```ts
import { appStore } from '../store';
import { INITIAL_SANDBOX_STATE, reduceSandbox } from '../utils/onboardingSandbox';

/** Mounts (or resets) the onboarding sandbox. */
export function openSandbox(): void {
  appStore.setState((state) => ({ ...state, onboardingSandbox: INITIAL_SANDBOX_STATE }));
}

export function closeSandbox(): void {
  appStore.setState((state) => ({ ...state, onboardingSandbox: null }));
}

export function dispatchSandboxAction(actionId: string): void {
  appStore.setState((state) =>
    state.onboardingSandbox === null
      ? state
      : { ...state, onboardingSandbox: reduceSandbox(state.onboardingSandbox, actionId) },
  );
}
```

- [ ] **Step 9: Route hotkeys through the sandbox** in `useAppHotkeys.ts`

- Read the pane reactively: `const sandboxPane = useStore(appStore, (s) => s.onboardingSandbox?.pane ?? null);`.
- In both `hotkeyDefs` and `sequenceDefs`, change `enabled: isActive(binding, { routeBase, activeOverlay })` to `enabled: isBindingEnabled(binding, { routeBase, activeOverlay, sandboxPane })`, and add `sandboxPane` to both `useMemo` dependency arrays.
- Add this module-level helper and use it in both callbacks instead of `getBindingAction(binding.actionId)?.(makeCtx())`. In the hotkey callback, keep the Escape `defaultPrevented` guard **before** the call.

```ts
/** Sandbox actions on the onboarding route drive the fake PR, not the app. */
function runBinding(actionId: string, context: KeyActionContext): void {
  if (
    context.routeBase === 'onboarding' &&
    appStore.state.onboardingSandbox !== null &&
    SANDBOX_ACTION_IDS.has(actionId)
  ) {
    dispatchSandboxAction(actionId);
    return;
  }
  getBindingAction(actionId)?.(context);
}
```

Imports: `isBindingEnabled` from `../utils/keybindings`, `SANDBOX_ACTION_IDS` from `../utils/onboardingSandbox`, `dispatchSandboxAction` from `../services/onboardingSandbox`. Drop the `isActive` import if nothing else uses it.

- [ ] **Step 10: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add electron/src/renderer
git commit -m "feat: route onboarding sandbox keys through the real hotkey layer"
```

---

### Task 5: Two-step onboarding with style picker and sandbox

**Files:**
- Modify: `electron/src/renderer/utils/keybindings.ts` (`ONBOARDING_BINDINGS`)
- Modify: `electron/src/renderer/utils/keymaps.ts` (`SHARED_KEYS`)
- Modify: `electron/src/renderer/utils/keybindingActions.ts`
- Modify: `electron/src/renderer/utils/keybindingActivation.ts` (drop `'onboarding.finish'`)
- Modify: `electron/src/renderer/utils/keybindings.test.ts` (rename in the test)
- Modify: `electron/src/renderer/store/ui.ts` (`ConfirmingAction`)
- Create: `electron/src/renderer/services/navigationStyle.ts`
- Create: `electron/src/renderer/components/NavigationStylePicker.tsx`
- Create: `electron/src/renderer/pages/OnboardingNavigationStep.tsx`, `electron/src/renderer/pages/OnboardingSandbox.tsx`
- Modify: `electron/src/renderer/pages/OnboardingPage.tsx`

**Interfaces:**
- Consumes: Task 4's `SANDBOX_TASKS`, `SANDBOX_PR_COUNT`, `SANDBOX_FILE_COUNT`, `SANDBOX_COMMENT_COUNT`, `openSandbox`, `closeSandbox`, `UIState.onboardingSandbox`; Task 1's `NAVIGATION_STYLES`, `NavigationStyle`; `formatBindingKeys(binding: KeyBinding): string` from `utils/legendPresets.ts`; `updateSettings(patch: Partial<AppSettings>): ResultAsync<void, Fault>` from `store/settings.ts`; `addToast` from `components/ToastContainer.tsx`.
- Produces:
  - actions `onboarding.continue` (`Space`), `onboarding.nextStyle` (`Tab`), `onboarding.prevStyle` (`Shift+Tab`), all with activation `{ routes: ['onboarding'] }`;
  - `ConfirmingAction` member `'onboarding_continue'` (replaces `'onboarding_finish'`);
  - `services/navigationStyle.ts`: `setNavigationStyle(style: NavigationStyle): ResultAsync<void, Fault>`, `cycleNavigationStyle(delta: 1 | -1): void`;
  - `<NavigationStylePicker selected={NavigationStyle} onSelect={(style: NavigationStyle) => void} />`.

- [ ] **Step 1: Rename finish → continue and add style cycling (catalog, keys, handlers)**

- `keybindings.ts` `ONBOARDING_BINDINGS`: rename `onboarding.finish` to `onboarding.continue` with description `'Continue'`. Add `onboarding.nextStyle` (`'Next navigation style'`) and `onboarding.prevStyle` (`'Previous navigation style'`), each with activation `{ routes: ['onboarding'] }`.
- `keymaps.ts` `SHARED_KEYS`: replace `'onboarding.finish': 'Space'` with `'onboarding.continue': 'Space'`, `'onboarding.nextStyle': 'Tab'`, `'onboarding.prevStyle': 'Shift+Tab'`.
- `store/ui.ts`: replace `'onboarding_finish'` with `'onboarding_continue'` in `ConfirmingAction`.
- `keybindingActivation.ts`: `CONTINUE_ACTION_IDS` becomes `new Set(['onboarding.continue'])`. It's a single id now, so replace the set with a `binding.actionId === 'onboarding.continue'` check.
- `keybindings.test.ts`: rename `'onboarding.finish'` to `'onboarding.continue'` in the Task 4 test.
- `keybindingActions.ts` `ACTIONS`: replace the `onboarding.finish` entry with

```ts
  'onboarding.continue': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'onboarding_continue',
    })),
  'onboarding.nextStyle': () => cycleNavigationStyle(1),
  'onboarding.prevStyle': () => cycleNavigationStyle(-1),
```

- [ ] **Step 2: Create `services/navigationStyle.ts`**

```ts
import type { Fault, ResultAsync } from '@thourum/fault';
import { addToast } from '../components/ToastContainer';
import { settingsStore, updateSettings } from '../store/settings';
import { NAVIGATION_STYLES, type NavigationStyle } from '../../types/settings';

export function setNavigationStyle(style: NavigationStyle): ResultAsync<void, Fault> {
  return updateSettings({ navigation: { style } });
}

/** Step to the neighbouring style (wrapping); reports a failed save as a toast. */
export function cycleNavigationStyle(delta: 1 | -1): void {
  const current = NAVIGATION_STYLES.indexOf(settingsStore.state.navigation.style);
  const next = NAVIGATION_STYLES.at((current + delta) % NAVIGATION_STYLES.length);
  if (next === undefined) return;
  void setNavigationStyle(next).match(
    () => undefined,
    (error) =>
      addToast({
        type: 'error',
        message: `Navigation style could not be saved: ${error.message}`,
        dismissible: true,
      }),
  );
}
```

`.at(-1)` wraps index `-1` to the last style, so `delta = -1` from `vim` gives `standard`. If importing `addToast` from a component into a service breaks a project boundary rule or lint, follow whatever pattern `services/prActions.ts` uses to report errors.

- [ ] **Step 3: Create `components/NavigationStylePicker.tsx`**

The three cards are clickable buttons in a 3-column grid. The selected card gets `border-primary`; the others get `border-outline-variant/30`. Match the look of the existing onboarding cards: `bg-surface-container-lowest`, `p-6`, `mono-text` headings, `text-on-surface-variant` body.

```tsx
import type { NavigationStyle } from '../../types/settings';
import { NAVIGATION_STYLES } from '../../types/settings';
import { cn } from '../lib/utils';

interface NavigationStyleCard {
  readonly label: string;
  readonly pitch: string;
  readonly caveat?: string;
}

const CARDS = {
  vim: { label: 'Vim', pitch: 'h j k l, g g, G — modal muscle memory.' },
  emacs: {
    label: 'Emacs',
    pitch: 'C-n / C-p to move, C-c C-<key> for actions.',
    caveat: 'On Windows and Linux, C-c also means copy outside text fields.',
  },
  standard: {
    label: 'Standard',
    pitch: 'Arrows, Home / End, Page Up / Down, and the mouse.',
  },
} satisfies Record<NavigationStyle, NavigationStyleCard>;

interface NavigationStylePickerProps {
  selected: NavigationStyle;
  onSelect: (style: NavigationStyle) => void;
}

export function NavigationStylePicker({ selected, onSelect }: NavigationStylePickerProps) {
  return (
    <div role="radiogroup" aria-label="Navigation style" className="grid grid-cols-1 md:grid-cols-3 gap-4">
      {NAVIGATION_STYLES.map((style) => {
        const card: NavigationStyleCard = CARDS[style];
        const isSelected = style === selected;
        return (
          <button
            key={style}
            type="button"
            role="radio"
            aria-checked={isSelected}
            onClick={() => onSelect(style)}
            className={cn(
              'text-left p-6 bg-surface-container-lowest border transition-colors',
              isSelected ? 'border-primary' : 'border-outline-variant/30 hover:border-outline',
            )}
          >
            <span className="mono-text text-xs font-bold uppercase tracking-widest text-on-surface">
              {card.label}
            </span>
            <p className="mt-3 text-sm text-on-surface-variant leading-snug">{card.pitch}</p>
            {card.caveat !== undefined && (
              <p className="mt-2 mono-text text-[10px] text-outline leading-relaxed">{card.caveat}</p>
            )}
          </button>
        );
      })}
    </div>
  );
}
```

Check that `cn` lives at `renderer/lib/utils.ts` (it's in the file tree) and fix the import path if needed.

- [ ] **Step 4: Create `pages/OnboardingSandbox.tsx`** (fake list / detail / diff)

Props: `{ state: SandboxState }`. Render a fixed-height panel (`h-72`, `bg-surface-container-lowest border border-outline-variant/30 mono-text text-sm`) whose content follows `state.pane`:
- **list:** `SANDBOX_PR_COUNT` rows of fake PR titles from a local `FAKE_PRS: readonly string[]` constant of 5 plausible titles, e.g. `'#412 Fix race in sync queue'`. The row at `state.cursor` is highlighted with `bg-surface-container-high`.
- **detail:** the title of PR `state.cursor`, two file names from a local `FAKE_FILES` constant of `SANDBOX_FILE_COUNT` entries (`{ path: string; lines: readonly string[] }`, 4–6 diff lines each, `+`/`-` prefixes), and an "Approved" badge (the existing `components/ui/badge.tsx`) when `state.approved`.
- **diff:** `FAKE_FILES[state.fileIndex]` lines with `+` lines in a success tone and `-` lines in an error tone, using whichever diff token classes `DiffView` already uses. Show `SANDBOX_COMMENT_COUNT` inline comment markers; the one at `state.commentIndex` is highlighted.

This is presentational only: no hotkeys and no handlers. Use `<pre>`-free semantic markup with `aria-live="polite"` on the panel so pane changes are announced.

- [ ] **Step 5: Create `pages/OnboardingNavigationStep.tsx`**

```tsx
import { useEffect, useMemo } from 'react';
import { NavigationStylePicker } from '../components/NavigationStylePicker';
import { Kbd } from '../components/ui/kbd';
import { useStore } from '../hooks/useStore';
import { closeSandbox, openSandbox } from '../services/onboardingSandbox';
import { setNavigationStyle } from '../services/navigationStyle';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { getEffectiveBindings } from '../utils/keybindings';
import { formatBindingKeys } from '../utils/legendPresets';
import { SANDBOX_TASKS } from '../utils/onboardingSandbox';
import { OnboardingSandbox } from './OnboardingSandbox';

interface OnboardingNavigationStepProps {
  onError: (message: string) => void;
}

export function OnboardingNavigationStep({ onError }: OnboardingNavigationStepProps) {
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const sandbox = useStore(appStore, (s) => s.onboardingSandbox);

  // Switching style restarts the checklist from the inbox pane.
  useEffect(() => {
    openSandbox();
    return closeSandbox;
  }, [style]);

  const taskKeys = useMemo(() => {
    const bindings = getEffectiveBindings(style, overrides);
    return SANDBOX_TASKS.map((task) => {
      const binding = task.actionIds
        .map((id) => bindings.find((b) => b.actionId === id))
        .find((found) => found !== undefined);
      return { task, keys: binding === undefined ? '—' : formatBindingKeys(binding) };
    });
  }, [style, overrides]);

  return (
    <section className="space-y-8">
      <NavigationStylePicker
        selected={style}
        onSelect={(next) =>
          void setNavigationStyle(next).match(
            () => undefined,
            (error) => onError(`Navigation style could not be saved: ${error.message}`),
          )
        }
      />
      <div className="grid grid-cols-1 md:grid-cols-12 gap-4">
        <div className="md:col-span-8">
          {sandbox !== null && <OnboardingSandbox state={sandbox} />}
        </div>
        <ol className="md:col-span-4 space-y-2">
          {taskKeys.map(({ task, keys }) => {
            const isDone = sandbox?.completed.includes(task.id) === true;
            return (
              <li key={task.id} className="flex items-center justify-between gap-3 text-sm">
                <span className={isDone ? 'text-on-surface line-through' : 'text-on-surface-variant'}>
                  {task.label}
                </span>
                <Kbd className="px-2 text-[10px] font-bold">{keys}</Kbd>
              </li>
            );
          })}
        </ol>
      </div>
    </section>
  );
}
```

Show a done marker with a material-symbols `check` icon (the page already uses that font, e.g. `<span className="material-symbols-outlined text-primary">check</span>`) as well as the strikethrough, so it isn't conveyed by strikethrough alone.

- [ ] **Step 6: Rework `OnboardingPage.tsx` into two steps**

- Add `const [step, setStep] = React.useState<'navigation' | 'devFolder'>('navigation');`.
- Replace the `onboarding_finish` effect with:

```tsx
  const advance = React.useCallback(() => {
    if (step === 'navigation') {
      setStep('devFolder');
      return;
    }
    finish();
  }, [step, finish]);

  React.useEffect(() => {
    if (confirmingAction !== 'onboarding_continue') return;
    appStore.setState((s) => ({ ...s, confirmingAction: null }));
    advance();
  }, [confirmingAction, advance]);
```

- Under the existing header, render `step === 'navigation' ? <OnboardingNavigationStep onError={setDevFolderError} /> : <the existing dev-folder content>`. If `devFolderError` is only rendered inside the dev-folder section, rename the state to `stepError` and render it under both steps.
- **Delete** the hardcoded "Core Movements" and "Omni-Search" cards (the `j`/`k`/`Enter`/`⌘K` `<kbd>` literals and the "standard vim-like bindings" copy). Step 1 replaces them.
- In the footer (where the old `Space` hint is), render a `<Button onClick={advance}>` labelled `Continue` on step 1 and `Finish` on step 2. Next to it, show the hint `<Kbd>{continueKeys}</Kbd>`, where `continueKeys` comes from `formatBindingKeys` of the effective `onboarding.continue` binding (resolved the same way as in Step 5). Add a small step indicator (`1 / 2`) using existing `mono-text` styles.

- [ ] **Step 7: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. Grep for leftovers: `grep -rn "onboarding_finish\|onboarding.finish" electron/src` should return nothing.

- [ ] **Step 8: Check by hand in the running app**

Reset onboarding: set `appearance.onboardingComplete` to `false` in `settings.json` (Settings → Keyboard → `settings.json` link, or the file under the app's userData). Then run `cd electron && pnpm start` and check:
1. Step 1 shows three cards with vim selected. `Tab` / `Shift+Tab` and clicking switch styles, and the checklist keys update.
2. In each style, every checklist item ticks with the keys shown: list → `Enter` → detail → approve → view diff → next file → next comment → back.
3. `Space` continues only from the inbox pane; the Continue button always works; step 2 Finish completes onboarding and lands on `/prs`.

If you can't launch the app (no display or no auth), say so explicitly in your report instead of claiming you checked it.

- [ ] **Step 9: Commit**

```bash
git add electron/src/renderer
git commit -m "feat: add navigation style step with sandbox to onboarding"
```

---

### Task 6: Style picker in Settings → Keyboard

**Files:**
- Modify: `electron/src/renderer/pages/settings/KeyboardSection.tsx`

**Interfaces:**
- Consumes: `NavigationStylePicker` (Task 5), `setNavigationStyle` (Task 5), `showSettingsError` from `./SettingsComponents` (already imported there).

- [ ] **Step 1: Add the picker** at the top of the `<Section id="keyboard" title="Keyboard">` body, above the existing `<p>`:

```tsx
      <NavigationStylePicker
        selected={style}
        onSelect={(next) => void setNavigationStyle(next).match(() => undefined, showSettingsError)}
      />
```

`style` is already read in this component since Task 2. If `showSettingsError`'s signature doesn't accept a `Fault`, adapt it the same way the existing `openSettingsFile` call site in this file does.

- [ ] **Step 2: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

- [ ] **Step 3: Check by hand**

`pnpm start` → Settings → Keyboard. Switching styles updates the binding list below immediately, and the help overlay (`?` in vim, `C-h` in emacs, `F1` in standard) and the footer show the new keys. Report if you couldn't run the app.

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/pages/settings/KeyboardSection.tsx
git commit -m "feat: choose navigation style in Settings → Keyboard"
```

---

### Task 7: Docs and final verification

**Files:**
- Modify: `website/src/pages/docs/keybindings.mdx`
- Modify: `CLAUDE.md` (the keyboard-bindings convention bullet)
- Modify: `docs/DESIGN.md` (only if it describes the onboarding screen)

- [ ] **Step 1: Rewrite `keybindings.mdx`**

Keep its frontmatter and section order (Global, PR inbox, …). Add a short "Navigation styles" intro:
- what vim / emacs / standard mean;
- how to choose (onboarding, Settings → Keyboard, or `"navigation": { "style": "emacs" }` in `settings.json`);
- that `keybindings` overrides apply on top of the chosen style.

Change each table to `| Action | Vim | Emacs | Standard |`, with values taken from `electron/src/renderer/utils/keymaps.ts` (the code is authoritative; the spec table may lag if Task 3 changed keys). Write keys the way the page already does (`Ctrl+d`, `g g`). Mention the emacs copy/cut caveat on Windows/Linux.

- [ ] **Step 2: Update `CLAUDE.md`**

Replace the keyboard convention bullet's first clause with: "Keyboard bindings are pure config: the action catalog (`actionId`, description, activation) lives in `electron/src/renderer/utils/keybindings.ts`, and keys come from the per-style tables (vim / emacs / standard, selected by `settings.navigation.style`) in `utils/keymaps.ts`; `utils/keymaps.test.ts` guards collisions." Keep the rest of the bullet (handlers, activation, `useAppHotkeys`, textarea submit). Add: "The onboarding sandbox reuses `useAppHotkeys` via `isBindingEnabled` + `utils/onboardingSandbox.ts`; don't register sandbox hotkeys separately."

- [ ] **Step 3: `docs/DESIGN.md`**

`grep -n -i onboarding docs/DESIGN.md`. If it describes the onboarding layout, add a short paragraph on the two steps (style cards + sandbox/checklist, then the dev folder). If not, change nothing.

- [ ] **Step 4: Full verification**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. Go was not touched, so no Go checks are needed (confirm with `git diff --stat master...HEAD -- go/` showing nothing).

- [ ] **Step 5: Check the remaining spec verify items**

These need the running app (`pnpm start`, emacs style). Record the results in your report:
1. **Mid-sequence singles:** in a PR detail with emacs, type `C-c C-f c`. Does `C-f` (scroll right) also fire? Compare with vim `f n`: does `n` (next file) also fire? If both styles behave the same, note it and keep it. If only emacs misbehaves, report back.
2. **Menu accelerators:** in standard style, check that `Mod+K`, `Mod+Shift+F`, `Mod+1`, `Mod+Z` reach Bistro and aren't swallowed by the app menu.

If you can't run the app, list these as unchecked in the report; don't mark them done.

- [ ] **Step 6: Commit**

```bash
git add website/src/pages/docs/keybindings.mdx CLAUDE.md docs/DESIGN.md
git commit -m "docs: document navigation styles"
```
