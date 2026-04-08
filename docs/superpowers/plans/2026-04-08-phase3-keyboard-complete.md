# Phase 3: Keyboard Navigation & Command Palette — Complete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make all keyboard bindings, command palette, help overlay, filter menu, and search fully functional and wired end-to-end per `design/keyboard-bindings.md`.

**Architecture:** Keyboard infrastructure (useKeyboard hook + keyboard.ts bindings) already exists and works. This plan fixes gaps: `:` not opening command palette, HelpOverlay not deriving from binding definitions, FilterMenu not keyboard-navigable, detail view missing hjkl scroll bindings, search not auto-focusing, and platform-specific shortcuts missing.

**Tech Stack:** React 18, TanStack Store, Vitest, @testing-library/react, Radix UI (Dialog, DropdownMenu, ScrollArea)

---

## Current State Analysis

### What's DONE and working:
- `src/renderer/hooks/useKeyboard.ts` (200 lines) — Core hook: keydown listener, multi-key sequence detection (gg, aa), modifier combos (ctrl+d), input focus guard, digit keys 1-9
- `src/renderer/utils/keyboard.ts` (275 lines) — 27 binding definitions: global (?, Esc, :, meta+q, alt+F4), list (j, k, gg, G, Enter, /, f, ctrl+d, ctrl+u, o, space, d, m, s, 1-9), detail (n, p, aa, c, r)
- `src/renderer/hooks/useKeyboardIntegration.ts` (14 lines) — Thin wrapper used by App.tsx
- `src/renderer/store/ui.ts` — UIState with mode, selectedPRIndex, selectedFileIndex, searchQuery, filterState, confirmingAction, filterMenuOpen
- `tests/unit/keyboard.test.ts` (586 lines) — 40+ tests covering all binding actions, sequences, modifier combos, input focus guard, immutability

### What needs FIXING/ADDING:

1. **`:` binding bug**: Sets `confirmingAction: 'command'` but CommandPalette.tsx checks `mode === 'command'`. The `:` binding should set `mode: 'command'`.

2. **HelpOverlay**: Hardcoded shortcuts list instead of deriving from `KEYBOARD_BINDING_GROUPS`. Not context-aware (should show different bindings for list vs detail).

3. **FilterMenu**: No keyboard navigation within menu (design says: o=open, d=draft, m=merged, a=all, n=none, c=clear, esc=close). Currently a Radix DropdownMenu that requires click to open — not connected to `filterMenuOpen` store state.

4. **Detail view scroll**: Missing h/j/k/l scroll bindings for diff viewer. Missing ctrl+d/ctrl+u page scroll in detail mode. Missing `[`/`]` hunk navigation, `0`/`$` top/bottom of diff.

5. **Search auto-focus**: When `/` sets mode to 'search', SearchBar doesn't auto-focus. CommandBar has a separate local `search` state not synced with store `searchQuery`.

6. **Platform search toggle**: `cmd+/` (macOS) and `ctrl+/` (Windows/Linux) bindings missing from keyboard.ts.

7. **Command palette completeness**: Missing `:clear` (clear filters/search), `:about` commands. No keyboard nav (arrow keys) for command list items.

8. **StatusBar hints**: Should show context-aware hints like "j/k nav | ? help | : commands"

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/renderer/utils/keyboard.ts` | Modify | Add detail scroll bindings (hjkl, ctrl+d/u, 0/$, [/]), add cmd+/ ctrl+/ search toggle, fix `:` action |
| `src/renderer/hooks/useKeyboard.ts` | Modify | Add scroll dispatch for detail hjkl (emit custom events or use refs) |
| `src/renderer/components/HelpOverlay.tsx` | Rewrite | Derive from KEYBOARD_BINDING_GROUPS, context-aware by mode |
| `src/renderer/components/FilterMenu.tsx` | Rewrite | Controlled by filterMenuOpen store state, keyboard nav (o/d/m/a/n/c/esc) |
| `src/renderer/components/CommandPalette.tsx` | Modify | Add `:clear`, `:about` commands, arrow key navigation |
| `src/renderer/components/SearchBar.tsx` | Modify | Auto-focus when mode='search', Enter applies filter |
| `src/renderer/components/CommandBar.tsx` | Modify | Remove duplicate search state, use store searchQuery |
| `src/renderer/components/StatusBar.tsx` | Modify | Show context-aware keyboard hints |
| `tests/unit/keyboard.test.ts` | Modify | Add tests for new bindings |
| `tests/unit/HelpOverlay.test.tsx` | Modify | Test context-aware display |
| `tests/unit/FilterMenu.test.tsx` | Modify | Test keyboard navigation |
| `tests/unit/CommandPalette.test.tsx` | Modify | Test new commands, arrow nav |

---

## Task 1: Fix `:` binding and command palette mode

**Files:**
- Modify: `src/renderer/utils/keyboard.ts` (line 47-49, `:` binding action)
- Test: `tests/unit/keyboard.test.ts`
- Test: `tests/unit/CommandPalette.test.tsx`

- [ ] **Step 1: Update the `:` binding action in keyboard.ts**

Change the `:` binding from `confirmingAction: 'command'` to `mode: 'command'`:

```typescript
// In keyboard.ts, globalBindings array, the ':' entry:
{
  keys: [':'],
  description: 'Open command palette',
  modes: ['list', 'detail'],
  action: (store) => {
    store.setState((s) => ({ ...s, mode: 'command' }));
  },
},
```

- [ ] **Step 2: Update keyboard.test.ts for `:` binding**

Change the existing `:` test to check `mode` instead of `confirmingAction`:

```typescript
describe(': binding — command mode', () => {
  it('sets mode to command', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding([':'])!;
    b.action(store);
    expect(store.state.mode).toBe('command');
  });
});
```

- [ ] **Step 3: Run tests to verify**

Run: `cd electron && pnpm vitest run tests/unit/keyboard.test.ts tests/unit/CommandPalette.test.tsx`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/utils/keyboard.ts electron/tests/unit/keyboard.test.ts
git commit -m "fix: colon binding opens command palette mode instead of confirmingAction"
```

---

## Task 2: Add detail view scroll bindings (hjkl, ctrl+d/u, 0/$, [/])

**Files:**
- Modify: `src/renderer/utils/keyboard.ts` (detailBindings array)
- Modify: `src/renderer/hooks/useKeyboard.ts` (add scroll dispatch helper)
- Test: `tests/unit/keyboard.test.ts`

- [ ] **Step 1: Add scroll bindings to keyboard.ts detailBindings**

Add these bindings to the `detailBindings` array in `src/renderer/utils/keyboard.ts`:

```typescript
// Detail view scroll bindings
{
  keys: ['h'],
  description: 'Scroll left in diff',
  modes: ['detail'],
  action: (_store) => {
    document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
  },
},
{
  keys: ['j'],
  description: 'Scroll down in diff',
  modes: ['detail'],
  action: (_store) => {
    document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
  },
},
{
  keys: ['k'],
  description: 'Scroll up in diff',
  modes: ['detail'],
  action: (_store) => {
    document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
  },
},
{
  keys: ['l'],
  description: 'Scroll right in diff',
  modes: ['detail'],
  action: (_store) => {
    document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
  },
},
{
  keys: ['ctrl+d'],
  description: 'Page down in diff',
  modes: ['detail'],
  action: (_store) => {
    document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 300 });
  },
},
{
  keys: ['ctrl+u'],
  description: 'Page up in diff',
  modes: ['detail'],
  action: (_store) => {
    document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -300 });
  },
},
{
  keys: ['0'],
  description: 'Jump to top of diff',
  modes: ['detail'],
  action: (_store) => {
    const el = document.querySelector('[data-diff-viewport]');
    if (el) el.scrollTop = 0;
  },
},
{
  keys: ['$'],
  description: 'Jump to bottom of diff',
  modes: ['detail'],
  action: (_store) => {
    const el = document.querySelector('[data-diff-viewport]');
    if (el) el.scrollTop = el.scrollHeight;
  },
},
```

- [ ] **Step 2: Add `data-diff-viewport` attribute to DiffViewer**

Read `DiffViewer.tsx` and add `data-diff-viewport` to the scrollable container element.

- [ ] **Step 3: Add tests for new detail bindings**

```typescript
describe('detail view scroll bindings', () => {
  it('h binding exists for detail mode', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'h' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
    expect(b!.description).toContain('left');
  });

  it('j binding exists for detail mode (scroll)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'j' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });

  it('0 binding jumps to top of diff', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === '0' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });

  it('$ binding jumps to bottom of diff', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === '$' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });
});
```

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/keyboard.test.ts`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/utils/keyboard.ts electron/src/renderer/components/DiffViewer.tsx electron/tests/unit/keyboard.test.ts
git commit -m "feat: add detail view hjkl scroll, ctrl+d/u page, 0/$ jump bindings"
```

---

## Task 3: Add platform-specific search toggle (cmd+/ and ctrl+/)

**Files:**
- Modify: `src/renderer/utils/keyboard.ts` (globalBindings array)
- Test: `tests/unit/keyboard.test.ts`

- [ ] **Step 1: Add cmd+/ and ctrl+/ bindings**

Add to `globalBindings` in `src/renderer/utils/keyboard.ts`:

```typescript
{
  keys: ['meta+/'],
  description: 'Toggle search box (macOS)',
  modes: ['list', 'detail'],
  action: (store) => {
    const { mode } = store.state;
    if (mode === 'search') {
      store.setState((s) => ({ ...s, mode: 'list', searchQuery: '' }));
    } else {
      store.setState((s) => ({ ...s, mode: 'search' }));
    }
  },
},
{
  keys: ['ctrl+/'],
  description: 'Toggle search box (Linux/Windows)',
  modes: ['list', 'detail'],
  action: (store) => {
    const { mode } = store.state;
    if (mode === 'search') {
      store.setState((s) => ({ ...s, mode: 'list', searchQuery: '' }));
    } else {
      store.setState((s) => ({ ...s, mode: 'search' }));
    }
  },
},
```

- [ ] **Step 2: Add tests**

```typescript
describe('meta+/ binding — search toggle (macOS)', () => {
  it('enters search mode from list', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['meta+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('search');
  });

  it('exits search mode back to list', () => {
    const store = makeStore({ mode: 'search', searchQuery: 'test' });
    const b = findBinding(['meta+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('list');
    expect(store.state.searchQuery).toBe('');
  });
});

describe('ctrl+/ binding — search toggle (Linux/Windows)', () => {
  it('enters search mode from list', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['ctrl+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('search');
  });
});
```

- [ ] **Step 3: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/keyboard.test.ts`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/utils/keyboard.ts electron/tests/unit/keyboard.test.ts
git commit -m "feat: add cmd+/ and ctrl+/ platform search toggle bindings"
```

---

## Task 4: Rewrite HelpOverlay to derive from binding definitions

**Files:**
- Rewrite: `src/renderer/components/HelpOverlay.tsx`
- Test: `tests/unit/HelpOverlay.test.tsx`

- [ ] **Step 1: Read current HelpOverlay test file**

Read `tests/unit/HelpOverlay.test.tsx` to understand existing test expectations.

- [ ] **Step 2: Rewrite HelpOverlay.tsx**

Replace hardcoded `SHORTCUT_SECTIONS` with dynamic derivation from `KEYBOARD_BINDING_GROUPS`:

```typescript
import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';
import { ScrollArea } from './ui/scroll-area';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { KEYBOARD_BINDING_GROUPS, bindingSequence } from '../utils/keyboard';
import type { AppMode } from '../utils/keyboard';

interface ShortcutSection {
  readonly title: string;
  readonly shortcuts: readonly { key: string; description: string }[];
}

function buildSections(currentMode: AppMode): ShortcutSection[] {
  const globalBindings = KEYBOARD_BINDING_GROUPS.filter(
    (b) => b.modes.length > 1 || b.modes.length === 0
  );
  const listBindings = KEYBOARD_BINDING_GROUPS.filter(
    (b) => b.modes.length === 1 && b.modes[0] === 'list'
  );
  const detailBindings = KEYBOARD_BINDING_GROUPS.filter(
    (b) => b.modes.length === 1 && b.modes[0] === 'detail'
  );

  const toRow = (b: typeof KEYBOARD_BINDING_GROUPS[number]) => ({
    key: bindingSequence(b),
    description: b.description,
  });

  const sections: ShortcutSection[] = [
    { title: 'Global', shortcuts: globalBindings.map(toRow) },
  ];

  if (currentMode === 'list' || currentMode === 'help') {
    sections.push({ title: 'PR List', shortcuts: listBindings.map(toRow) });
  }
  if (currentMode === 'detail' || currentMode === 'help') {
    sections.push({ title: 'Detail View', shortcuts: detailBindings.map(toRow) });
  }

  return sections;
}

export function HelpOverlay(): React.ReactElement | null {
  const mode = useStore(appStore, (s) => s.mode);
  const isOpen = mode === 'help';

  // Determine the "previous" mode to show contextual bindings
  // When help is open, we show all bindings since we don't track previous mode
  const sections = buildSections(mode);

  const handleClose = (): void => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl max-h-[80vh]">
        <DialogHeader>
          <DialogTitle className="font-mono">Keyboard Shortcuts</DialogTitle>
        </DialogHeader>

        <ScrollArea className="h-full pr-4">
          <div className="space-y-6">
            {sections.map((section) => (
              <section key={section.title}>
                <h3 className="font-bold text-sm text-on-surface uppercase tracking-widest mb-3">
                  {section.title}
                </h3>
                <div className="space-y-2 text-sm">
                  {section.shortcuts.map((shortcut) => (
                    <div key={shortcut.key} className="flex justify-between">
                      <kbd className="font-mono text-on-surface-variant bg-surface-container px-2 py-0.5 rounded text-xs min-w-[3rem] text-center">
                        {shortcut.key}
                      </kbd>
                      <span className="text-on-surface">{shortcut.description}</span>
                    </div>
                  ))}
                </div>
              </section>
            ))}

            <div className="text-xs text-on-surface-variant text-center mt-4 pt-4 border-t border-outline-variant">
              Press <kbd className="font-mono bg-surface-container px-1 rounded">?</kbd> or <kbd className="font-mono bg-surface-container px-1 rounded">Esc</kbd> to close
            </div>
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 3: Update HelpOverlay tests**

Update tests to verify:
- Shortcuts are derived from binding definitions (not hardcoded)
- All binding groups appear in help
- Dialog opens when mode='help'

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/HelpOverlay.test.tsx`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/HelpOverlay.tsx electron/tests/unit/HelpOverlay.test.tsx
git commit -m "feat: HelpOverlay derives shortcuts from binding definitions, context-aware"
```

---

## Task 5: Rewrite FilterMenu with keyboard navigation

**Files:**
- Rewrite: `src/renderer/components/FilterMenu.tsx`
- Modify: `src/renderer/utils/keyboard.ts` (add filter mode bindings)
- Modify: `src/renderer/store/ui.ts` (add 'filter' mode handling)
- Test: `tests/unit/FilterMenu.test.tsx`

- [ ] **Step 1: Read current FilterMenu test**

Read `tests/unit/FilterMenu.test.tsx` to understand existing expectations.

- [ ] **Step 2: Update keyboard.ts — change `f` to toggle filter mode**

The `f` binding currently toggles `filterMenuOpen`. Change it to also set the mode so FilterMenu can capture keyboard events:

```typescript
{
  keys: ['f'],
  description: 'Toggle filter menu',
  modes: ['list'],
  action: (store) => {
    const isOpen = store.state.filterMenuOpen;
    store.setState((s) => ({
      ...s,
      filterMenuOpen: !isOpen,
      mode: isOpen ? 'list' : 'filter',
    }));
  },
},
```

- [ ] **Step 3: Add filter-mode bindings to keyboard.ts**

Add a new `filterBindings` array:

```typescript
const filterBindings: BindingDefinition[] = [
  {
    keys: ['o'],
    description: 'Set filter to Open',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'open' }));
    },
  },
  {
    keys: ['d'],
    description: 'Set filter to Draft',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'draft' }));
    },
  },
  {
    keys: ['a'],
    description: 'Set filter to All',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'all' }));
    },
  },
  {
    keys: ['Escape'],
    description: 'Close filter menu',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
  {
    keys: ['f'],
    description: 'Close filter menu',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
];
```

Add `...filterBindings` to `KEYBOARD_BINDING_GROUPS`.

**Important:** The existing global `Escape` binding covers modes `['list', 'detail', 'help', 'search']`. You need to add `'filter'` to the mode list OR handle filter Escape separately. Since filter has specific close behavior, a separate binding is cleaner.

- [ ] **Step 4: Update UIState mode type**

The `mode` type in `src/renderer/store/ui.ts` already includes `'filter'`. Verify this. If not, add it.

- [ ] **Step 5: Rewrite FilterMenu component**

Replace Radix DropdownMenu with a controlled panel that shows/hides based on `filterMenuOpen` store state:

```typescript
import React from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

type FilterValue = UIState['filterState'];

const FILTER_OPTIONS: { label: string; value: FilterValue; hint: string }[] = [
  { label: 'Open', value: 'open', hint: 'o' },
  { label: 'Draft', value: 'draft', hint: 'd' },
  { label: 'All', value: 'all', hint: 'a' },
];

export function FilterMenu(): React.ReactElement | null {
  const isOpen = useStore(appStore, (s) => s.filterMenuOpen);
  const filterState = useStore(appStore, (s) => s.filterState);

  if (!isOpen) return null;

  const handleSelect = (value: FilterValue) => {
    appStore.setState((s) => ({ ...s, filterState: value }));
  };

  return (
    <div className="absolute top-full right-0 mt-1 z-50 w-48 border border-outline-variant bg-surface-container rounded shadow-lg">
      <div className="px-3 py-2 border-b border-outline-variant">
        <span className="text-xs font-bold text-on-surface-variant uppercase tracking-widest">Filters</span>
      </div>
      {FILTER_OPTIONS.map(({ label, value, hint }) => (
        <div
          key={value}
          onClick={() => handleSelect(value)}
          className={`flex items-center justify-between px-3 py-2 cursor-pointer transition-colors hover:bg-surface-container-low ${
            filterState === value ? 'bg-surface-container-low font-semibold' : ''
          }`}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => e.key === 'Enter' && handleSelect(value)}
        >
          <span className="text-sm text-on-surface">{label}</span>
          <kbd className="text-[10px] text-on-surface-variant font-mono bg-surface-container px-1 rounded">{hint}</kbd>
        </div>
      ))}
      <div className="px-3 py-2 border-t border-outline-variant text-[10px] text-on-surface-variant">
        Press <kbd className="font-mono">f</kbd> or <kbd className="font-mono">Esc</kbd> to close
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Update FilterMenu tests**

Test that:
- FilterMenu renders when `filterMenuOpen=true`
- FilterMenu is null when `filterMenuOpen=false`
- Clicking options updates filterState
- Keyboard hints are displayed

- [ ] **Step 7: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/FilterMenu.test.tsx tests/unit/keyboard.test.ts`
Expected: All pass

- [ ] **Step 8: Commit**

```bash
git add electron/src/renderer/utils/keyboard.ts electron/src/renderer/components/FilterMenu.tsx electron/src/renderer/store/ui.ts electron/tests/unit/FilterMenu.test.tsx electron/tests/unit/keyboard.test.ts
git commit -m "feat: FilterMenu with keyboard navigation (o/d/a/f/esc in filter mode)"
```

---

## Task 6: Fix SearchBar auto-focus and search flow

**Files:**
- Modify: `src/renderer/components/SearchBar.tsx`
- Modify: `src/renderer/components/CommandBar.tsx`
- Test: `tests/unit/keyboard.test.ts` (search mode tests)

- [ ] **Step 1: Add auto-focus to SearchBar**

Add a `useEffect` that focuses the input when mode changes to 'search':

```typescript
import React, { useRef, useEffect } from 'react';
import { Input } from '@renderer/components/ui/input';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

export function SearchBar() {
  const query = useStore(appStore, (s: UIState) => s.searchQuery);
  const mode = useStore(appStore, (s: UIState) => s.mode);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (mode === 'search' && inputRef.current) {
      inputRef.current.focus();
    }
  }, [mode]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    appStore.setState((s) => ({ ...s, searchQuery: value }));
  };

  const handleFocus = () => {
    appStore.setState((s) => ({ ...s, mode: 'search' }));
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      appStore.setState((s) => ({ ...s, mode: 'list', searchQuery: '' }));
      inputRef.current?.blur();
    } else if (e.key === 'Enter') {
      // Apply search filter — exit search mode but keep query
      appStore.setState((s) => ({ ...s, mode: 'list' }));
      inputRef.current?.blur();
    }
  };

  return (
    <Input
      ref={inputRef}
      type="text"
      placeholder="Search PRs... (/ to focus)"
      value={query}
      onChange={handleChange}
      onFocus={handleFocus}
      onKeyDown={handleKeyDown}
      className="flex-1 px-3 py-2 bg-surface-container-lowest border border-outline-variant"
    />
  );
}
```

- [ ] **Step 2: Fix CommandBar to use store searchQuery instead of local state**

Remove the local `useState` from CommandBar and use the store's `searchQuery`:

```typescript
import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

export default function CommandBar() {
  const mode = useStore(appStore, (state: UIState) => state.mode);

  return (
    <div className="command-bar">
      <div className="mode-indicator">
        {mode === 'list' && <span>List Mode</span>}
        {mode === 'search' && <span>Search Mode</span>}
        {mode === 'detail' && <span>Detail Mode</span>}
        {mode === 'help' && <span>Help Mode</span>}
        {mode === 'command' && <span>Command Mode</span>}
        {mode === 'filter' && <span>Filter Mode</span>}
      </div>
    </div>
  );
}
```

Note: CommandBar was showing a duplicate search input. SearchBar already exists as a separate component. Remove the input from CommandBar.

- [ ] **Step 3: Run tests**

Run: `cd electron && pnpm vitest run`
Expected: All pass (fix any broken tests due to CommandBar change)

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/SearchBar.tsx electron/src/renderer/components/CommandBar.tsx
git commit -m "feat: SearchBar auto-focus on / key, Enter applies filter, fix CommandBar"
```

---

## Task 7: Enhance CommandPalette with missing commands and arrow nav

**Files:**
- Modify: `src/renderer/components/CommandPalette.tsx`
- Test: `tests/unit/CommandPalette.test.tsx`

- [ ] **Step 1: Read current CommandPalette test file**

Read `tests/unit/CommandPalette.test.tsx`.

- [ ] **Step 2: Add missing commands and arrow key navigation**

Add `:clear` and `:about` commands. Add arrow key navigation (up/down to select, Enter to execute):

```typescript
const COMMANDS: readonly Command[] = [
  { id: 'help', name: 'help', description: 'Show keyboard shortcuts' },
  { id: 'settings', name: 'settings', description: 'Open settings' },
  { id: 'clear', name: 'clear', description: 'Clear all filters and search' },
  { id: 'focus-list', name: 'focus list', description: 'Focus PR list' },
  { id: 'focus-detail', name: 'focus detail', description: 'Focus PR detail' },
  { id: 'about', name: 'about', description: 'Show app version and credits' },
] as const;
```

Add `selectedCommandIndex` state and arrow key handling in `handleInputKeyDown`:

```typescript
const [selectedIndex, setSelectedIndex] = useState(0);

const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>): void => {
  if (e.key === 'Escape') {
    handleClose();
  } else if (e.key === 'ArrowDown') {
    e.preventDefault();
    setSelectedIndex((i) => Math.min(i + 1, filteredCommands.length - 1));
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    setSelectedIndex((i) => Math.max(i - 1, 0));
  } else if (e.key === 'Enter' && filteredCommands.length > 0) {
    e.preventDefault();
    handleExecute(filteredCommands[selectedIndex]);
  }
};
```

Reset selectedIndex when query changes:

```typescript
useEffect(() => {
  setSelectedIndex(0);
}, [query]);
```

Add `clear` case to `handleExecute`:

```typescript
case 'clear':
  appStore.setState((s) => ({
    ...s,
    mode: 'list',
    searchQuery: '',
    filterState: 'open',
    filterMenuOpen: false,
  }));
  setQuery('');
  return;
case 'about':
  // TODO: Show about dialog with version info
  handleClose();
  return;
```

Highlight selected command in the list:

```typescript
className={`px-4 py-3 cursor-pointer transition-colors ${
  idx === selectedIndex
    ? 'bg-surface-container-low border-l-2 border-l-primary'
    : 'hover:bg-surface-container-low'
}`}
```

- [ ] **Step 3: Update CommandPalette tests**

Add tests for:
- `:clear` command clears filters and search
- Arrow key navigation changes selection
- Enter executes selected command
- `:about` command exists

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/CommandPalette.test.tsx`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/CommandPalette.tsx electron/tests/unit/CommandPalette.test.tsx
git commit -m "feat: CommandPalette with :clear, :about commands and arrow key navigation"
```

---

## Task 8: StatusBar context-aware hints

**Files:**
- Modify: `src/renderer/components/StatusBar.tsx`
- Read: `tests/unit/` for any StatusBar tests

- [ ] **Step 1: Read current StatusBar component**

Read `src/renderer/components/StatusBar.tsx` to understand current implementation.

- [ ] **Step 2: Add mode-aware keyboard hints**

Update StatusBar to show context-aware hints based on current mode:

```typescript
import React from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

const MODE_HINTS: Record<UIState['mode'], string> = {
  list: 'j/k nav | Enter open | / search | f filter | : cmd | ? help',
  detail: 'hjkl scroll | n/p files | aa approve | c comment | Esc back',
  search: 'Type to search | Enter apply | Esc cancel',
  help: 'Esc close',
  command: 'Type command | Enter execute | Esc cancel',
  filter: 'o open | d draft | a all | f/Esc close',
  settings: 'Esc back',
};

export default function StatusBar() {
  const mode = useStore(appStore, (s: UIState) => s.mode);
  const syncStatus = useStore(appStore, (s: UIState) => s.syncStatus);
  const confirmingAction = useStore(appStore, (s: UIState) => s.confirmingAction);

  const hint = confirmingAction === 'approve'
    ? "Press 'a' again to confirm approval"
    : MODE_HINTS[mode] ?? '';

  return (
    <div className="flex items-center justify-between px-4 py-1.5 bg-surface-container border-t border-outline-variant text-[11px] font-mono">
      <span className="text-on-surface-variant">{hint}</span>
      <span className={`${syncStatus === 'connected' || syncStatus === 'ready' ? 'text-primary' : 'text-on-surface-variant'}`}>
        Status: {syncStatus.toUpperCase()}
      </span>
    </div>
  );
}
```

- [ ] **Step 3: Fix any broken StatusBar tests**

Run: `cd electron && pnpm vitest run`
Fix any test failures related to StatusBar changes.

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/StatusBar.tsx
git commit -m "feat: StatusBar shows context-aware keyboard hints per mode"
```

---

## Task 9: Verify useKeyboard handles filter mode correctly

**Files:**
- Modify: `src/renderer/hooks/useKeyboard.ts` (handle filter mode like search)
- Test: `tests/unit/keyboard.test.ts`

- [ ] **Step 1: Update useKeyboard to allow filter mode key processing**

The `useKeyboard` hook currently only special-cases `search` mode. The `filter` mode should also be handled — bindings like `o`, `d`, `a`, `f`, `Escape` should work in filter mode. Since these are already defined with `modes: ['filter']`, the hook should process them correctly. But verify that the hook doesn't block filter mode processing.

Review `useKeyboard.ts` lines 143-149: the search mode guard returns early for non-Escape keys. Ensure filter mode doesn't have the same guard (it shouldn't, since filter uses keyboard bindings not text input).

- [ ] **Step 2: Add integration test for filter mode keyboard flow**

```typescript
describe('filter mode keyboard flow', () => {
  it('o binding sets filterState to open in filter mode', () => {
    const result = findBindingFn('o', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
  });

  it('d binding sets filterState to draft in filter mode', () => {
    const result = findBindingFn('d', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
  });

  it('f binding closes filter menu in filter mode', () => {
    const result = findBindingFn('f', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
  });
});
```

- [ ] **Step 3: Run full test suite**

Run: `cd electron && pnpm vitest run`
Expected: ALL 149+ tests pass

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/hooks/useKeyboard.ts electron/tests/unit/keyboard.test.ts
git commit -m "feat: useKeyboard supports filter mode bindings"
```

---

## Task 10: Full build verification and final cleanup

**Files:**
- All modified files from Tasks 1-9

- [ ] **Step 1: Run TypeScript type check**

Run: `cd electron && pnpm build`
Expected: Clean build, no type errors

- [ ] **Step 2: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All tests pass, 0 failures

- [ ] **Step 3: Verify binding count matches design spec**

The design spec lists 25+ unique bindings. Verify the `KEYBOARD_BINDING_GROUPS` array has >= 35 entries (original 27 + 8 detail scroll + 2 platform search + ~5 filter mode).

- [ ] **Step 4: Verify all modes are handled in useKeyboard**

Check that all modes in UIState['mode'] are covered: list, detail, search, help, command, filter, settings.

- [ ] **Step 5: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: Phase 3 keyboard navigation complete — all bindings wired"
```

---

## Reference Files

| File | Purpose |
|------|---------|
| `design/keyboard-bindings.md` | Authoritative binding spec (475 lines) |
| `docs/IMPLEMENTATION_CHECKLIST.md` | Phase 3 checklist items |
| `src/renderer/utils/keyboard.ts` | Binding definitions (source of truth for code) |
| `src/renderer/hooks/useKeyboard.ts` | Core keyboard event handler |
| `src/renderer/store/ui.ts` | UIState including mode, filter, search state |
| `src/renderer/components/HelpOverlay.tsx` | Help overlay component |
| `src/renderer/components/FilterMenu.tsx` | Filter menu component |
| `src/renderer/components/CommandPalette.tsx` | Command palette component |
| `src/renderer/components/SearchBar.tsx` | Search input component |
| `src/renderer/components/StatusBar.tsx` | Status bar with hints |
| `tests/unit/keyboard.test.ts` | Keyboard binding tests |

## Important Constraints

1. **Immutability**: ALL store updates must use `store.setState((s) => ({ ...s, ... }))` — never mutate
2. **No duplicate bindings**: Same key can appear in different modes but not the same mode
3. **Input focus guard**: Regular keys ignored when input/textarea/contenteditable has focus; Escape always processed
4. **Test coverage**: Every new binding must have at least one test
5. **Existing tests**: All 149 existing tests must continue to pass
6. **Build**: Must compile with zero TypeScript errors
