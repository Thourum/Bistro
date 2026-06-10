import type { Store } from '@tanstack/store';
import type { KeybindingOverrides } from '../../types/settings';
import { authAction, PROVIDERS } from '../providers';
import type { UIState } from '../store/ui';

export type AppMode = UIState['mode'];

export interface BindingDefinition {
  /** Stable, human-legible id used by settings overrides, e.g. `list.down`. */
  actionId: string;
  /** One or more key strings that trigger this binding (e.g. ['j'], ['g','g']) */
  keys: string[];
  description: string;
  /** Modes in which this binding is active. Empty array = all non-search modes. */
  modes: AppMode[];
  action: (store: Store<UIState>) => void;
}

// ─── Global bindings (active in list, detail, help) ──────────────────────────

const globalBindings: BindingDefinition[] = [
  {
    actionId: 'global.help',
    keys: ['?'],
    description: 'Show help / bindings overlay',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'help' }));
    },
  },
  {
    actionId: 'global.escape',
    keys: ['Escape'],
    description: 'Close help / cancel input / back to list',
    modes: ['list', 'detail', 'help', 'search', 'settings'],
    action: (store) => {
      const { mode } = store.state;
      if (mode === 'help') {
        store.setState((s) => ({ ...s, mode: 'list' }));
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
    keys: [':'],
    description: 'Open command mode',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'command' }));
    },
  },
  {
    actionId: 'global.quit.mac',
    keys: ['meta+q'],
    description: 'Quit app (macOS)',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.quit.win',
    keys: ['alt+F4'],
    description: 'Quit app (Linux/Windows)',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.search.mac',
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
    actionId: 'global.search.win',
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
];

// ─── Login bindings (derived from PROVIDER_META) ────────────────────────────

const loginBindings: BindingDefinition[] = PROVIDERS.map((provider) => ({
  actionId: `login.provider.${provider.id}`,
  keys: [provider.key],
  description: `Login with ${provider.label}`,
  modes: ['login'] as AppMode[],
  action: (store: Store<UIState>) => {
    store.setState((s) => ({ ...s, confirmingAction: authAction(provider) }));
  },
}));

// ─── PR List bindings (active in list mode only) ──────────────────────────────

const listBindings: BindingDefinition[] = [
  {
    actionId: 'list.down',
    keys: ['j'],
    description: 'Move to next PR',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 1 }));
    },
  },
  {
    actionId: 'list.up',
    keys: ['k'],
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
    keys: ['G'],
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
    keys: ['Enter'],
    description: 'Open selected PR in detail view',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'detail' }));
    },
  },
  {
    actionId: 'list.search',
    keys: ['/'],
    description: 'Focus search box',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'search' }));
    },
  },
  {
    actionId: 'list.filter',
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
  {
    actionId: 'list.pageDown',
    keys: ['ctrl+d'],
    description: 'Page down (5 PRs at a time)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 5 }));
    },
  },
  {
    actionId: 'list.pageUp',
    keys: ['ctrl+u'],
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
    keys: ['o'],
    description: 'Open selected PR in browser',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'open-browser' }));
    },
  },
  {
    actionId: 'list.preview',
    keys: [' '],
    description: 'Preview PR metadata tooltip',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'preview' }));
    },
  },
  {
    actionId: 'list.draft',
    keys: ['d'],
    description: 'Mark PR as draft / un-draft',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'draft' }));
    },
  },
  {
    actionId: 'list.mute',
    keys: ['m'],
    description: 'Toggle mute (hide PR from list)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'mute' }));
    },
  },
  {
    actionId: 'list.star',
    keys: ['s'],
    description: 'Star / unstar PR',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'star' }));
    },
  },
  {
    actionId: 'list.jumpToN',
    keys: ['1-9'],
    description: 'Jump to Nth PR (1 = first, 9 = ninth)',
    modes: ['list'],
    action: (_store) => {
      // Resolved by the hook via a special-case; no-op placeholder.
    },
  },
];

// ─── Detail View bindings (active in detail mode only) ────────────────────────

const detailBindings: BindingDefinition[] = [
  {
    actionId: 'detail.nextFile',
    keys: ['n'],
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
    keys: ['p'],
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
    keys: ['c'],
    description: 'Comment on current line',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'comment' }));
    },
  },
  {
    actionId: 'detail.reply',
    keys: ['r'],
    description: 'Reply to existing comment',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'reply' }));
    },
  },
];

// ─── Filter mode bindings (active when filter menu is open) ──────────────────

const filterBindings: BindingDefinition[] = [
  {
    actionId: 'filter.needsAction',
    keys: ['n'],
    description: 'Set filter to Needs Action',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'needs_action' }));
    },
  },
  {
    actionId: 'filter.open',
    keys: ['o'],
    description: 'Set filter to Open',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'open' }));
    },
  },
  {
    actionId: 'filter.draft',
    keys: ['d'],
    description: 'Set filter to Draft',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'draft' }));
    },
  },
  {
    actionId: 'filter.all',
    keys: ['a'],
    description: 'Set filter to All',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterState: 'all' }));
    },
  },
  {
    actionId: 'filter.close',
    keys: ['Escape'],
    description: 'Close filter menu',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
  {
    actionId: 'filter.toggle',
    keys: ['f'],
    description: 'Close filter menu',
    modes: ['filter'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: false, mode: 'list' }));
    },
  },
];

// ─── Detail View scroll bindings ─────────────────────────────────────────────

const detailScrollBindings: BindingDefinition[] = [
  {
    actionId: 'detail.scrollLeft',
    keys: ['h'],
    description: 'Scroll left in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
    },
  },
  {
    actionId: 'detail.scrollDown',
    keys: ['j'],
    description: 'Scroll down in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUp',
    keys: ['k'],
    description: 'Scroll up in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.scrollRight',
    keys: ['l'],
    description: 'Scroll right in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
    },
  },
  {
    actionId: 'detail.pageDown',
    keys: ['ctrl+d'],
    description: 'Page down in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 300 });
    },
  },
  {
    actionId: 'detail.pageUp',
    keys: ['ctrl+u'],
    description: 'Page up in diff',
    modes: ['detail'],
    action: (_store) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -300 });
    },
  },
  {
    actionId: 'detail.scrollTop',
    keys: ['0'],
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
    keys: ['$'],
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
 * Default (code-defined) keyboard bindings. This is the authoritative source
 * of truth; overrides from `settings.keybindings` are merged on top via
 * {@link getEffectiveBindings}.
 */
export const DEFAULT_KEYBOARD_BINDINGS: BindingDefinition[] = [
  ...globalBindings,
  ...loginBindings,
  ...listBindings,
  ...detailBindings,
  ...detailScrollBindings,
  ...filterBindings,
];

/**
 * Back-compat alias — existing callers import `KEYBOARD_BINDING_GROUPS`.
 * Prefer `DEFAULT_KEYBOARD_BINDINGS` or `getEffectiveBindings()` in new code.
 */
export const KEYBOARD_BINDING_GROUPS: BindingDefinition[] =
  DEFAULT_KEYBOARD_BINDINGS;

/**
 * Merge user overrides from `settings.keybindings` onto the defaults.
 *
 * - Absent entries fall back to the default keys.
 * - Empty arrays (`[]`) are treated as "remove this binding".
 * - Non-empty arrays replace the default keys wholesale.
 */
export function getEffectiveBindings(
  overrides: KeybindingOverrides,
): BindingDefinition[] {
  const result: BindingDefinition[] = [];
  for (const binding of DEFAULT_KEYBOARD_BINDINGS) {
    const override = overrides[binding.actionId];
    if (override === undefined) {
      result.push(binding);
      continue;
    }
    if (override.length === 0) continue;
    result.push({ ...binding, keys: override });
  }
  return result;
}

/**
 * Returns the sequence string for a binding definition.
 * Single-key bindings return the key itself; multi-key bindings join them.
 */
export function bindingSequence(binding: BindingDefinition): string {
  return binding.keys.join('');
}
