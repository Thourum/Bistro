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
    keys: '?',
    description: 'Show help / bindings overlay',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'help' }));
    },
  },
  {
    actionId: 'global.escape',
    keys: 'Escape',
    description: 'Close help / cancel input / back to list',
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
    keys: ':',
    description: 'Open command mode',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'command' }));
    },
  },
  {
    actionId: 'global.quit.mac',
    keys: 'Meta+Q',
    description: 'Quit app (macOS)',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.quit.win',
    keys: 'Alt+F4',
    description: 'Quit app (Linux/Windows)',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.search.mac',
    keys: 'Meta+/',
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
    actionId: 'global.search.win',
    keys: 'Control+/',
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
    description:
      digit === 1 ? 'Jump to Nth PR (press 1–9)' : `Jump to PR #${digit}`,
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
    description: 'Move to next PR',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 1 }));
    },
  },
  {
    actionId: 'list.up',
    keys: 'k',
    description: 'Move to previous PR',
    modes: ['list'],
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
    description: 'Jump to top of PR list',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: 0 }));
    },
  },
  {
    actionId: 'list.bottom',
    keys: 'Shift+G',
    description: 'Jump to bottom of PR list',
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
    description: 'Open selected PR in detail view',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'detail' }));
    },
  },
  {
    actionId: 'list.search',
    keys: '/',
    description: 'Focus search box',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'search' }));
    },
  },
  {
    actionId: 'list.filter',
    keys: 'f',
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
  {
    actionId: 'list.pageDown',
    keys: 'Control+D',
    description: 'Page down (5 PRs at a time)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 5 }));
    },
  },
  {
    actionId: 'list.pageUp',
    keys: 'Control+U',
    description: 'Page up (5 PRs at a time)',
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
    description: 'Open selected PR in browser',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'open-browser' }));
    },
  },
  {
    actionId: 'list.preview',
    keys: 'Space',
    description: 'Preview PR metadata tooltip',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'preview' }));
    },
  },
  {
    actionId: 'list.draft',
    keys: 'd',
    description: 'Mark PR as draft / un-draft',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'draft' }));
    },
  },
  {
    actionId: 'list.mute',
    keys: 'm',
    description: 'Toggle mute (hide PR from list)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'mute' }));
    },
  },
  {
    actionId: 'list.star',
    keys: 's',
    description: 'Star / unstar PR',
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
    description: 'Next file in diff',
    modes: ['detail'],
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
    description: 'Previous file in diff',
    modes: ['detail'],
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
    description: 'Open full-screen diff view',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'diff' }));
    },
  },
  {
    actionId: 'detail.approve',
    keys: ['a', 'a'],
    description: 'Approve PR (double-tap to confirm)',
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
    description: 'Comment on current line',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'comment' }));
    },
  },
  {
    actionId: 'detail.reply',
    keys: 'r',
    description: 'Reply to existing comment',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'reply' }));
    },
  },
];

// ─── Filter mode bindings (active when filter menu is open) ──────────────────

const filterBindings: KeyBinding[] = [
  {
    actionId: 'filter.needsAction',
    keys: 'n',
    description: 'Set filter to Needs Action',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'needs_action' }));
    },
  },
  {
    actionId: 'filter.open',
    keys: 'o',
    description: 'Set filter to Open',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'open' }));
    },
  },
  {
    actionId: 'filter.draft',
    keys: 'd',
    description: 'Set filter to Draft',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'draft' }));
    },
  },
  {
    actionId: 'filter.all',
    keys: 'a',
    description: 'Set filter to All',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'all' }));
    },
  },
  {
    actionId: 'filter.close',
    keys: 'Escape',
    description: 'Close filter menu',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
  {
    actionId: 'filter.toggle',
    keys: 'f',
    description: 'Close filter menu',
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
    description: 'Scroll left in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
    },
  },
  {
    actionId: 'detail.scrollDown',
    keys: 'j',
    description: 'Scroll down in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUp',
    keys: 'k',
    description: 'Scroll up in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.scrollRight',
    keys: 'l',
    description: 'Scroll right in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
    },
  },
  {
    actionId: 'detail.pageDown',
    keys: 'Control+D',
    description: 'Page down in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 300 });
    },
  },
  {
    actionId: 'detail.pageUp',
    keys: 'Control+U',
    description: 'Page up in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -300 });
    },
  },
  {
    actionId: 'detail.scrollTop',
    keys: '0',
    description: 'Jump to top of diff',
    modes: ['detail'],
    action: (_store) => {
      const el = document.querySelector(
        '[data-diff-viewport]',
      ) as HTMLElement | null;
      if (el) el.scrollTop = 0;
    },
  },
  {
    actionId: 'detail.scrollBottom',
    keys: '$',
    description: 'Jump to bottom of diff',
    modes: ['detail'],
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
