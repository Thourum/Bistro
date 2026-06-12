import type { Store } from '@tanstack/store';
import type { KeybindingOverrides } from '../../types/settings';
import { authAction, PROVIDERS } from '../providers';
import type { UIState } from '../store/ui';

export type AppMode = UIState['mode'];

/**
 * A single keyboard binding. `keys` uses TanStack Hotkeys syntax verbatim:
 * a hotkey string for single chords (`'j'`, `'Shift+G'`, `'Mod+/'`,
 * `'Escape'`) or an array of hotkey strings for Vim-style sequences
 * (`['g', 'g']`). No translation layer exists — what is written here is what
 * gets registered.
 */
export interface KeyBinding {
  /** Stable, human-legible id used by settings overrides, e.g. `list.down`. */
  actionId: string;
  /** TanStack hotkey string, or an array of them for a sequence. */
  keys: string | readonly string[];
  description: string;
  /** Modes in which this binding is active. Empty array = all modes. */
  modes: readonly AppMode[];
  /** Excluded from the help overlay (e.g. the repetitive 2–9 jump keys). */
  hidden?: boolean;
  action: (store: Store<UIState>) => void;
}

/** True when the binding is a multi-chord sequence (['g','g']). */
export function isSequenceBinding(
  binding: KeyBinding,
): binding is KeyBinding & { keys: readonly string[] } {
  return Array.isArray(binding.keys);
}

// ─── Global bindings ─────────────────────────────────────────────────────────

const globalBindings: KeyBinding[] = [
  {
    actionId: 'global.help',
    // The matcher requires exact modifier state, so shifted punctuation is
    // written as Shift + the physical base key ('?' = Shift+/ on US layouts).
    keys: 'Shift+/',
    description: 'Help',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({
        ...s,
        mode: 'help',
        helpSourceMode: s.mode === 'detail' ? 'detail' : 'list',
      }));
    },
  },
  {
    actionId: 'global.escape',
    keys: 'Escape',
    description: 'Back / Cancel',
    modes: ['list', 'detail', 'diff', 'help', 'search', 'settings'],
    action: (store) => {
      const { mode } = store.state;
      if (mode === 'help') {
        store.setState((s) => ({ ...s, mode: 'list' }));
      } else if (mode === 'diff') {
        store.setState((s) => ({ ...s, mode: 'detail' }));
      } else if (mode === 'detail') {
        store.setState((s) => ({ ...s, mode: 'list' }));
      } else if (mode === 'search') {
        store.setState((s) => ({ ...s, mode: 'list', searchQuery: '' }));
      } else if (mode === 'settings') {
        store.setState((s) => ({ ...s, mode: 'list' }));
      } else if (mode === 'list') {
        store.setState((s) => ({
          ...s,
          filterMenuOpen: false,
          confirmingAction: null,
        }));
      }
    },
  },
  {
    actionId: 'global.command',
    keys: 'Shift+;', // ':' on US layouts
    description: 'Commands',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'command' }));
    },
  },
  {
    actionId: 'global.settings',
    keys: 'Mod+,',
    description: 'Settings',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'settings' }));
    },
  },
  {
    actionId: 'global.quit.mac',
    keys: 'Meta+Q',
    description: 'Quit',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.quit.win',
    keys: 'Alt+F4',
    description: 'Quit',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.search',
    // 'Mod' resolves to Meta on macOS and Control elsewhere — one binding
    // covers all platforms, and formatForDisplay renders it natively.
    keys: 'Mod+/',
    description: 'Search',
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
];

// ─── Login bindings (derived from PROVIDER_META) ────────────────────────────

const loginBindings: KeyBinding[] = PROVIDERS.map((provider) => ({
  actionId: `login.provider.${provider.id}`,
  keys: provider.key,
  description: `Login with ${provider.label}`,
  modes: ['login'] as AppMode[],
  action: (store: Store<UIState>) => {
    store.setState((s) => ({ ...s, confirmingAction: authAction(provider) }));
  },
}));

// ─── PR List bindings (active in list mode only) ──────────────────────────────

/** Jump bindings for digits 1–9; only the first is shown in the help overlay. */
const jumpBindings: KeyBinding[] = Array.from({ length: 9 }, (_, i) => {
  const digit = i + 1;
  return {
    actionId: `list.jumpTo${digit}`,
    keys: String(digit),
    description: digit === 1 ? 'Jump to PR (1–9)' : `Jump to PR #${digit}`,
    modes: ['list'] as AppMode[],
    hidden: digit !== 1,
    action: (store: Store<UIState>) => {
      store.setState((s) => ({ ...s, selectedPRIndex: digit - 1 }));
    },
  };
});

const listBindings: KeyBinding[] = [
  {
    actionId: 'list.down',
    keys: 'j',
    description: 'Next',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 1 }));
    },
  },
  {
    actionId: 'list.up',
    keys: 'k',
    description: 'Previous',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({
        ...s,
        selectedPRIndex: Math.max(0, s.selectedPRIndex - 1),
      }));
    },
  },
  {
    actionId: 'list.downArrow',
    keys: 'ArrowDown',
    description: 'Next',
    modes: ['list'],
    hidden: true,
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 1 }));
    },
  },
  {
    actionId: 'list.upArrow',
    keys: 'ArrowUp',
    description: 'Previous',
    modes: ['list'],
    hidden: true,
    action: (store) => {
      store.setState((s) => ({
        ...s,
        selectedPRIndex: Math.max(0, s.selectedPRIndex - 1),
      }));
    },
  },
  {
    actionId: 'list.top',
    keys: ['g', 'g'],
    description: 'Jump to top',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: 0 }));
    },
  },
  {
    actionId: 'list.bottom',
    keys: 'Shift+G',
    description: 'Jump to bottom',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({
        ...s,
        selectedPRIndex: Number.MAX_SAFE_INTEGER,
      }));
    },
  },
  {
    actionId: 'list.open',
    keys: 'Enter',
    description: 'Open',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'detail' }));
    },
  },
  {
    actionId: 'list.search',
    keys: '/',
    description: 'Search',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'search' }));
    },
  },
  {
    actionId: 'list.filter',
    keys: 'f',
    description: 'Filter',
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
  {
    actionId: 'list.pageDown',
    keys: 'Control+D',
    description: 'Down (5 PRs)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 5 }));
    },
  },
  {
    actionId: 'list.pageUp',
    keys: 'Control+U',
    description: 'Up (5 PRs)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({
        ...s,
        selectedPRIndex: Math.max(0, s.selectedPRIndex - 5),
      }));
    },
  },
  {
    actionId: 'list.openBrowser',
    keys: 'o',
    description: 'Open (in Browser)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'open-browser' }));
    },
  },
  {
    actionId: 'list.preview',
    keys: 'Space',
    description: 'Preview',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'preview' }));
    },
  },
  {
    actionId: 'list.draft',
    keys: 'd',
    description: 'Toggle draft',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'draft' }));
    },
  },
  {
    actionId: 'list.mute',
    keys: 'm',
    description: 'Mute',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'mute' }));
    },
  },
  {
    actionId: 'list.star',
    keys: 's',
    description: 'Star',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'star' }));
    },
  },
  ...jumpBindings,
];

// ─── Detail View bindings (active in detail mode only) ────────────────────────

const detailBindings: KeyBinding[] = [
  {
    actionId: 'detail.nextFile',
    keys: 'n',
    description: 'Next file',
    modes: ['detail', 'diff'],
    action: (store) => {
      store.setState((s) => ({
        ...s,
        selectedFileIndex: s.selectedFileIndex + 1,
      }));
    },
  },
  {
    actionId: 'detail.prevFile',
    keys: 'p',
    description: 'Previous file',
    modes: ['detail', 'diff'],
    action: (store) => {
      store.setState((s) => ({
        ...s,
        selectedFileIndex: Math.max(0, s.selectedFileIndex - 1),
      }));
    },
  },
  {
    actionId: 'detail.viewDiff',
    keys: 'Space',
    description: 'View diff',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'diff' }));
    },
  },
  {
    actionId: 'detail.approve',
    keys: ['a', 'a'],
    description: 'Approve (double-tap)',
    modes: ['detail'],
    action: (store) => {
      const { confirmingAction } = store.state;
      if (confirmingAction === 'approve') {
        store.setState((s) => ({
          ...s,
          confirmingAction: 'approve_confirmed',
        }));
      } else {
        store.setState((s) => ({ ...s, confirmingAction: 'approve' }));
      }
    },
  },
  {
    actionId: 'detail.comment',
    keys: 'c',
    description: 'Comment',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'comment' }));
    },
  },
  {
    actionId: 'detail.reply',
    keys: 'r',
    description: 'Reply',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'reply' }));
    },
  },
];

// ─── Filter mode bindings (active when filter menu is open) ──────────────────
// Selecting a filter applies it AND closes the menu, so `f a` behaves like a
// sequence: `f` reveals the options in the footer legend, the next key picks
// one and returns to the list.

function applyFilter(
  store: Store<UIState>,
  filterState: UIState['filterState'],
): void {
  store.setState((s) => ({
    ...s,
    filterState,
    filterMenuOpen: false,
    mode: 'list',
  }));
}

const filterBindings: KeyBinding[] = [
  {
    actionId: 'filter.needsAction',
    keys: 'n',
    description: 'Needs Action',
    modes: ['filter'],
    action: (store) => {
      applyFilter(store, 'needs_action');
    },
  },
  {
    actionId: 'filter.open',
    keys: 'o',
    description: 'Open',
    modes: ['filter'],
    action: (store) => {
      applyFilter(store, 'open');
    },
  },
  {
    actionId: 'filter.draft',
    keys: 'd',
    description: 'Draft',
    modes: ['filter'],
    action: (store) => {
      applyFilter(store, 'draft');
    },
  },
  {
    actionId: 'filter.all',
    keys: 'a',
    description: 'All',
    modes: ['filter'],
    action: (store) => {
      applyFilter(store, 'all');
    },
  },
  {
    actionId: 'filter.close',
    keys: 'Escape',
    description: 'Close',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
  {
    actionId: 'filter.toggle',
    keys: 'f',
    description: 'Close',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
];

// ─── Detail View scroll bindings ─────────────────────────────────────────────

const detailScrollBindings: KeyBinding[] = [
  {
    actionId: 'detail.scrollLeft',
    keys: 'h',
    description: 'Scroll left',
    modes: ['detail', 'diff'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
    },
  },
  {
    actionId: 'detail.scrollDown',
    keys: 'j',
    description: 'Scroll down',
    modes: ['detail', 'diff'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUp',
    keys: 'k',
    description: 'Scroll up',
    modes: ['detail', 'diff'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.scrollRight',
    keys: 'l',
    description: 'Scroll right',
    modes: ['detail', 'diff'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
    },
  },
  {
    actionId: 'detail.scrollDownArrow',
    keys: 'ArrowDown',
    description: 'Scroll down',
    modes: ['detail', 'diff'],
    hidden: true,
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUpArrow',
    keys: 'ArrowUp',
    description: 'Scroll up',
    modes: ['detail', 'diff'],
    hidden: true,
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.scrollLeftArrow',
    keys: 'ArrowLeft',
    description: 'Scroll left',
    modes: ['detail', 'diff'],
    hidden: true,
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
    },
  },
  {
    actionId: 'detail.scrollRightArrow',
    keys: 'ArrowRight',
    description: 'Scroll right',
    modes: ['detail', 'diff'],
    hidden: true,
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
    },
  },
  {
    actionId: 'detail.pageDown',
    keys: 'Control+D',
    description: 'Page down',
    modes: ['detail', 'diff'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 300 });
    },
  },
  {
    actionId: 'detail.pageUp',
    keys: 'Control+U',
    description: 'Page up',
    modes: ['detail', 'diff'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -300 });
    },
  },
  {
    actionId: 'detail.scrollTop',
    keys: '0',
    description: 'Jump to top',
    modes: ['detail', 'diff'],
    action: (_store) => {
      const el = document.querySelector(
        '[data-diff-viewport]',
      ) as HTMLElement | null;
      if (el) el.scrollTop = 0;
    },
  },
  {
    actionId: 'detail.scrollBottom',
    keys: 'Shift+4', // '$' on US layouts
    description: 'Jump to bottom',
    modes: ['detail', 'diff'],
    action: (_store) => {
      const el = document.querySelector(
        '[data-diff-viewport]',
      ) as HTMLElement | null;
      if (el) el.scrollTop = el.scrollHeight;
    },
  },
];

/**
 * Default (code-defined) keybindings. This is the authoritative source of
 * truth; overrides from `settings.keybindings` are merged on top via
 * {@link getEffectiveBindings}.
 */
export const DEFAULT_KEYBINDINGS: KeyBinding[] = [
  ...globalBindings,
  ...loginBindings,
  ...listBindings,
  ...detailBindings,
  ...detailScrollBindings,
  ...filterBindings,
];

/**
 * Merge user overrides from `settings.keybindings` onto the defaults.
 *
 * Override values are arrays of TanStack hotkey strings:
 * - Absent entries fall back to the default keys.
 * - Empty arrays (`[]`) are treated as "remove this binding".
 * - One-element arrays replace the binding with a single hotkey.
 * - Multi-element arrays replace it with a sequence.
 */
export function getEffectiveBindings(
  overrides: KeybindingOverrides,
): KeyBinding[] {
  const result: KeyBinding[] = [];
  for (const binding of DEFAULT_KEYBINDINGS) {
    const override = overrides[binding.actionId];
    if (override === undefined) {
      result.push(binding);
      continue;
    }
    if (override.length === 0) continue;
    const keys = override.length === 1 ? (override[0] as string) : override;
    result.push({ ...binding, keys });
  }
  return result;
}
