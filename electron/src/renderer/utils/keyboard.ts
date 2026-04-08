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

// ─── Help overlay bindings ────────────────────────────────────────────────────

const helpBindings: BindingDefinition[] = [
  {
    keys: ['j'],
    description: 'Scroll help text down',
    modes: ['help'],
    action: (_store) => {
      // Scroll is handled by the HelpOverlay component via CSS/DOM
    },
  },
  {
    keys: ['k'],
    description: 'Scroll help text up',
    modes: ['help'],
    action: (_store) => {
      // Scroll is handled by the HelpOverlay component via CSS/DOM
    },
  },
];

export const KEYBOARD_BINDING_GROUPS: BindingDefinition[] = [
  ...globalBindings,
  ...listBindings,
  ...detailBindings,
  ...helpBindings,
];

/**
 * Returns the sequence string for a binding definition.
 * Single-key bindings return the key itself; multi-key bindings join them.
 */
export function bindingSequence(binding: BindingDefinition): string {
  return binding.keys.join('');
}
