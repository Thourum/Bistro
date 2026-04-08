import type { Store } from '@tanstack/store';
import type { UIState } from '../store/ui';

export type AppMode = UIState['mode'];

export interface BindingDefinition {
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
    keys: ['?'],
    description: 'Show help / bindings overlay',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'help' }));
    },
  },
  {
    keys: ['Escape'],
    description: 'Close help / cancel input / back to list',
    modes: ['list', 'detail', 'help', 'search'],
    action: (store) => {
      const { mode } = store.state;
      if (mode === 'help') {
        store.setState((s) => ({ ...s, mode: 'list' }));
      } else if (mode === 'detail') {
        store.setState((s) => ({ ...s, mode: 'list' }));
      } else if (mode === 'search') {
        store.setState((s) => ({ ...s, mode: 'list', searchQuery: '' }));
      } else if (mode === 'list') {
        store.setState((s) => ({ ...s, filterMenuOpen: false, confirmingAction: null }));
      }
    },
  },
  {
    keys: [':'],
    description: 'Open command mode',
    modes: ['list', 'detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'command' }));
    },
  },
  {
    // macOS: Cmd+Q  — triggers window.close() via IPC; store signals intent
    keys: ['meta+q'],
    description: 'Quit app (macOS)',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    // Linux/Windows: Alt+F4 — same quit intent
    keys: ['alt+F4'],
    description: 'Quit app (Linux/Windows)',
    modes: ['list', 'detail', 'help', 'search'],
    action: (_store) => {
      window.electronAPI?.quitApp?.();
    },
  },
];

// ─── PR List bindings (active in list mode only) ──────────────────────────────

const listBindings: BindingDefinition[] = [
  {
    keys: ['j'],
    description: 'Move to next PR',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 1 }));
    },
  },
  {
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
    keys: ['g', 'g'],
    description: 'Jump to top of PR list',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: 0 }));
    },
  },
  {
    keys: ['G'],
    description: 'Jump to bottom of PR list',
    modes: ['list'],
    action: (store) => {
      // Bounds clamping is handled by the consumer; store accepts any index.
      store.setState((s) => ({ ...s, selectedPRIndex: Number.MAX_SAFE_INTEGER }));
    },
  },
  {
    keys: ['Enter'],
    description: 'Open selected PR in detail view',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'detail' }));
    },
  },
  {
    keys: ['/'],
    description: 'Focus search box',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, mode: 'search' }));
    },
  },
  {
    keys: ['f'],
    description: 'Toggle filter menu',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, filterMenuOpen: !s.filterMenuOpen }));
    },
  },
  {
    keys: ['ctrl+d'],
    description: 'Page down (5 PRs at a time)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedPRIndex: s.selectedPRIndex + 5 }));
    },
  },
  {
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
    keys: ['o'],
    description: 'Open selected PR in browser',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'open-browser' }));
    },
  },
  {
    keys: [' '],
    description: 'Preview PR metadata tooltip',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'preview' }));
    },
  },
  {
    keys: ['d'],
    description: 'Mark PR as draft / un-draft',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'draft' }));
    },
  },
  {
    keys: ['m'],
    description: 'Toggle mute (hide PR from list)',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'mute' }));
    },
  },
  {
    keys: ['s'],
    description: 'Star / unstar PR',
    modes: ['list'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'star' }));
    },
  },
  {
    // Digit keys 1–9: jump directly to the Nth PR in the visible list.
    // The hook resolves the actual index from event.key at dispatch time;
    // the binding entry acts as the canonical definition for the group.
    keys: ['1-9'],
    description: 'Jump to Nth PR (1 = first, 9 = ninth)',
    modes: ['list'],
    action: (_store) => {
      // Resolved by the hook via resolveDigitBinding(); no-op placeholder.
    },
  },
];

// ─── Detail View bindings (active in detail mode only) ────────────────────────

const detailBindings: BindingDefinition[] = [
  {
    keys: ['n'],
    description: 'Next file in diff',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, selectedFileIndex: s.selectedFileIndex + 1 }));
    },
  },
  {
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
    keys: ['a', 'a'],
    description: 'Approve PR (double-tap to confirm)',
    modes: ['detail'],
    action: (store) => {
      const { confirmingAction } = store.state;
      if (confirmingAction === 'approve') {
        // Second tap: confirm approve
        store.setState((s) => ({ ...s, confirmingAction: null }));
        // Actual API call is handled by a separate effect watching confirmingAction
      } else {
        store.setState((s) => ({ ...s, confirmingAction: 'approve' }));
      }
    },
  },
  {
    keys: ['c'],
    description: 'Comment on current line',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'comment' }));
    },
  },
  {
    keys: ['r'],
    description: 'Reply to existing comment',
    modes: ['detail'],
    action: (store) => {
      store.setState((s) => ({ ...s, confirmingAction: 'reply' }));
    },
  },
];

// Help overlay bindings (j/k scroll) are deferred to the HelpOverlay task.

export const KEYBOARD_BINDING_GROUPS: BindingDefinition[] = [
  ...globalBindings,
  ...listBindings,
  ...detailBindings,
];

/**
 * Returns the sequence string for a binding definition.
 * Single-key bindings return the key itself; multi-key bindings join them.
 */
export function bindingSequence(binding: BindingDefinition): string {
  return binding.keys.join('');
}
