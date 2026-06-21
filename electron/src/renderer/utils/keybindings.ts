import type { Store } from '@tanstack/store';
import type { KeybindingOverrides } from '../../types/settings';
import type {
  OverlayContextValue,
  OverlayKind,
} from '../overlays/OverlayContext';
import { authAction, PROVIDERS } from '../providers';
import { router } from '../router';
import type { UIState } from '../store/ui';

// RouteBase: the "base" route derived from the current pathname.
// Includes 'login' and 'onboarding' so their bindings can be properly gated
// (auth and onboarding are enforced by declarative route guards).
export type RouteBase =
  | 'list'
  | 'detail'
  | 'diff'
  | 'settings'
  | 'login'
  | 'onboarding';

// AppBase: the "base" route derived from the current pathname.
// Navigation is 100% router-owned; there is no separate AppMode enum.

/**
 * Derive the RouteBase from the current pathname.
 */
export function routeBaseFromPath(pathname: string): RouteBase | null {
  if (/^\/prs\/[^/]+\/diff$/.test(pathname)) return 'diff';
  if (/^\/prs\/[^/]+$/.test(pathname)) return 'detail';
  if (pathname === '/prs' || pathname === '/prs/') return 'list';
  if (pathname === '/settings') return 'settings';
  if (pathname === '/login') return 'login';
  if (pathname === '/onboarding') return 'onboarding';
  return null;
}

/**
 * Binding activation descriptor.
 * - `routes`: the base routes this binding fires on (omit = all routes).
 * - `overlays`: the overlay kinds this binding fires on (overlay-exclusive).
 * When an overlay is active, ONLY bindings with a matching `overlays` entry fire.
 * When no overlay is active, only route-gated bindings fire.
 */
export interface BindingActivation {
  routes?: RouteBase[];
  overlays?: OverlayKind[];
}

/**
 * Context passed to every keybinding action.
 */
export interface KeyActionContext {
  store: Store<UIState>;
  routeBase: RouteBase | null;
  activeOverlay: OverlayKind | null;
  overlays: OverlayContextValue;
  location: { pathname: string };
}

/**
 * A single keyboard binding.
 */
export interface KeyBinding {
  /** Stable, human-legible id used by settings overrides, e.g. `list.down`. */
  actionId: string;
  /** TanStack hotkey string, or an array of them for a sequence. */
  keys: string | readonly string[];
  description: string;
  /**
   * Activation descriptor: routes (no overlay) or overlays.
   * Empty = all routes (no overlay active).
   */
  activation: BindingActivation;
  /** Excluded from the help overlay (e.g. the repetitive 2–9 jump keys). */
  hidden?: boolean;
  action: (ctx: KeyActionContext) => void;
}

/** True when the binding is a multi-chord sequence (['g','g']). */
export function isSequenceBinding(
  binding: KeyBinding,
): binding is KeyBinding & { keys: readonly string[] } {
  return Array.isArray(binding.keys);
}

/**
 * Determine if a binding is active given the current route and overlay state.
 *
 * Semantics:
 *  • When an overlay is active, ONLY bindings whose `activation.overlays`
 *    includes the active overlay fire.
 *  • When no overlay is active, only route-gated (non-overlay-only) bindings
 *    fire:
 *    – `routes === undefined`  → active on every non-null routeBase.
 *    – `routes.length === 0`   → NEVER active (used by login bindings).
 *    – `routes.includes(r)`    → active only on those routes.
 *    – Overlay-only bindings (`overlays` set, `routes` absent) → never fire
 *      when no overlay is active.
 */
export function isActive(
  binding: KeyBinding,
  {
    routeBase,
    activeOverlay,
  }: { routeBase: RouteBase | null; activeOverlay: OverlayKind | null },
): boolean {
  if (activeOverlay !== null) {
    return binding.activation.overlays?.includes(activeOverlay) === true;
  }
  // No overlay active.
  // Overlay-only bindings (have overlays but no routes) never fire here.
  if (
    binding.activation.overlays !== undefined &&
    binding.activation.routes === undefined
  ) {
    return false;
  }
  const routes = binding.activation.routes;
  // routes: [] means "never active on any route" (e.g. login bindings).
  if (routes !== undefined && routes.length === 0) return false;
  return (
    routeBase !== null && (routes === undefined || routes.includes(routeBase))
  );
}

// ─── Global bindings ─────────────────────────────────────────────────────────

const ALL_OVERLAYS: OverlayKind[] = [
  'command',
  'help',
  'search',
  'filter',
  'repoPicker',
];

const globalBindings: KeyBinding[] = [
  {
    actionId: 'global.help',
    // The matcher requires exact modifier state, so shifted punctuation is
    // written as Shift + the physical base key ('?' = Shift+/ on US layouts).
    keys: 'Shift+/',
    description: 'Help',
    activation: { routes: ['list', 'detail'] },
    action: (ctx) => {
      const source = ctx.routeBase === 'detail' ? 'detail' : 'list';
      ctx.overlays.openHelp(source);
    },
  },
  {
    actionId: 'global.escape',
    keys: 'Escape',
    description: 'Back / Cancel',
    activation: {
      routes: ['list', 'detail', 'diff', 'settings'],
      overlays: ALL_OVERLAYS,
    },
    action: (ctx) => {
      const { activeOverlay, routeBase } = ctx;
      if (activeOverlay === 'search') {
        ctx.overlays.closeSearch({ clearQuery: true });
      } else if (activeOverlay !== null) {
        ctx.overlays.closeOverlay();
      } else if (routeBase === 'diff') {
        // Navigate back to /prs/$prId
        const pathname = router.state.location.pathname;
        const m = /^\/prs\/([^/]+)\/diff$/.exec(pathname);
        if (m) {
          void router.navigate({
            to: '/prs/$prId',
            params: { prId: m[1]! },
          });
        }
      } else if (routeBase === 'detail') {
        void router.navigate({
          to: '/prs',
          search: (prev: Record<string, unknown>) => prev,
        });
      } else if (routeBase === 'settings') {
        void router.navigate({
          to: '/prs',
          search: (prev: Record<string, unknown>) => prev,
        });
      } else if (routeBase === 'list') {
        ctx.store.setState((s) => ({
          ...s,
          confirmingAction: null,
        }));
      }
    },
  },
  {
    actionId: 'global.command',
    keys: 'Shift+;', // ':' on US layouts
    description: 'Commands',
    activation: { routes: ['list', 'detail'] },
    action: (ctx) => {
      ctx.overlays.openCommand();
    },
  },
  {
    actionId: 'global.settings',
    keys: 'Mod+,',
    description: 'Settings',
    activation: { routes: ['list', 'detail'] },
    action: (_ctx) => {
      void router.navigate({ to: '/settings' });
    },
  },
  {
    actionId: 'global.quit.mac',
    keys: 'Meta+Q',
    description: 'Quit',
    // Active on all base routes AND while help/search overlays are open
    // (matches old behaviour: quit worked in list/detail/help/search modes).
    activation: {
      routes: ['list', 'detail', 'diff', 'settings'] as RouteBase[],
      overlays: ['help', 'search'] as OverlayKind[],
    },
    action: (_ctx) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.quit.win',
    keys: 'Alt+F4',
    description: 'Quit',
    activation: {
      routes: ['list', 'detail', 'diff', 'settings'] as RouteBase[],
      overlays: ['help', 'search'] as OverlayKind[],
    },
    action: (_ctx) => {
      window.electronAPI?.quitApp?.();
    },
  },
  {
    actionId: 'global.search',
    // 'Mod' resolves to Meta on macOS and Control elsewhere — one binding
    // covers all platforms, and formatForDisplay renders it natively.
    keys: 'Mod+/',
    description: 'Search',
    activation: { routes: ['list', 'detail'] },
    action: (ctx) => {
      ctx.overlays.openSearch();
    },
  },
];

// ─── Login bindings (derived from PROVIDER_META) ────────────────────────────

const loginBindings: KeyBinding[] = PROVIDERS.map((provider) => ({
  actionId: `login.provider.${provider.id}`,
  keys: provider.key,
  description: `Login with ${provider.label}`,
  // Login provider keys (g/l/t) are active ONLY on /login —
  // they must never fire on /prs, /settings, etc.
  activation: { routes: ['login'] as RouteBase[] },
  action: (ctx: KeyActionContext) => {
    ctx.store.setState((s) => ({
      ...s,
      confirmingAction: authAction(provider),
    }));
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
    activation: { routes: ['list'] as RouteBase[] },
    hidden: digit !== 1,
    action: (ctx: KeyActionContext) => {
      ctx.store.setState((s) => ({ ...s, selectedPRIndex: digit - 1 }));
    },
  };
});

const listBindings: KeyBinding[] = [
  {
    actionId: 'list.down',
    keys: 'j',
    description: 'Next',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: s.selectedPRIndex + 1,
      }));
    },
  },
  {
    actionId: 'list.up',
    keys: 'k',
    description: 'Previous',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: Math.max(0, s.selectedPRIndex - 1),
      }));
    },
  },
  {
    actionId: 'list.downArrow',
    keys: 'ArrowDown',
    description: 'Next',
    activation: { routes: ['list'] },
    hidden: true,
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: s.selectedPRIndex + 1,
      }));
    },
  },
  {
    actionId: 'list.upArrow',
    keys: 'ArrowUp',
    description: 'Previous',
    activation: { routes: ['list'] },
    hidden: true,
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: Math.max(0, s.selectedPRIndex - 1),
      }));
    },
  },
  {
    actionId: 'list.top',
    keys: ['g', 'g'],
    description: 'Jump to top',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, selectedPRIndex: 0 }));
    },
  },
  {
    actionId: 'list.bottom',
    keys: 'Shift+G',
    description: 'Jump to bottom',
    activation: { routes: ['list'] },
    action: (ctx) => {
      // Set cursor to the last visible row; row index is only a cursor, not PR identity.
      const len = ctx.store.state.visiblePRIds.length;
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: len > 0 ? len - 1 : 0,
      }));
    },
  },
  {
    actionId: 'list.open',
    keys: 'Enter',
    description: 'Open',
    activation: { routes: ['list'] },
    action: (ctx) => {
      // Navigate by stable PR id after clamping the cursor to the visible list.
      const { selectedPRIndex, visiblePRIds } = ctx.store.state;
      const clampedIdx = Math.min(
        Math.max(0, selectedPRIndex),
        Math.max(0, visiblePRIds.length - 1),
      );
      const prId = visiblePRIds[clampedIdx];
      if (prId !== undefined) {
        void router.navigate({ to: '/prs/$prId', params: { prId } });
      }
    },
  },
  {
    actionId: 'list.search',
    keys: '/',
    description: 'Search',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.overlays.openSearch();
    },
  },
  {
    actionId: 'list.filter',
    keys: 'f',
    description: 'Filter',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.overlays.toggleFilter();
    },
  },
  {
    actionId: 'list.pageDown',
    keys: 'Control+D',
    description: 'Down (5 PRs)',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: s.selectedPRIndex + 5,
      }));
    },
  },
  {
    actionId: 'list.pageUp',
    keys: 'Control+U',
    description: 'Up (5 PRs)',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: Math.max(0, s.selectedPRIndex - 5),
      }));
    },
  },
  {
    actionId: 'list.openBrowser',
    keys: 'o',
    description: 'Open (in Browser)',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'open-browser' }));
    },
  },
  {
    actionId: 'list.preview',
    keys: 'Space',
    description: 'Preview',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'preview' }));
    },
  },
  {
    actionId: 'list.draft',
    keys: 'd',
    description: 'Toggle draft',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'draft' }));
    },
  },
  {
    actionId: 'list.mute',
    keys: 'm',
    description: 'Mute',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'mute' }));
    },
  },
  {
    actionId: 'list.star',
    keys: 's',
    description: 'Star',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'star' }));
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
    activation: { routes: ['detail', 'diff'] },
    action: (ctx) => {
      const { diffFileKeys, selectedFileIndex } = ctx.store.state;
      const newIdx = Math.min(
        selectedFileIndex + 1,
        Math.max(0, diffFileKeys.length - 1),
      );
      ctx.store.setState((s) => ({
        ...s,
        selectedFileIndex: newIdx,
      }));
      // In diff route, also update the ?file route param.
      if (ctx.routeBase === 'diff' && diffFileKeys.length > 0) {
        const newKey = diffFileKeys[newIdx];
        if (newKey !== undefined) {
          const pathname = router.state.location.pathname;
          const m = /^\/prs\/([^/]+)\/diff$/.exec(pathname);
          if (m) {
            void router.navigate({
              to: '/prs/$prId/diff',
              params: { prId: m[1]! },
              search: { file: newKey, side: 'right' },
              replace: true,
            });
          }
        }
      }
    },
  },
  {
    actionId: 'detail.prevFile',
    keys: 'p',
    description: 'Previous file',
    activation: { routes: ['detail', 'diff'] },
    action: (ctx) => {
      const { diffFileKeys, selectedFileIndex } = ctx.store.state;
      const newIdx = Math.max(0, selectedFileIndex - 1);
      ctx.store.setState((s) => ({
        ...s,
        selectedFileIndex: newIdx,
      }));
      // In diff route, also update the ?file route param.
      if (ctx.routeBase === 'diff' && diffFileKeys.length > 0) {
        const newKey = diffFileKeys[newIdx];
        if (newKey !== undefined) {
          const pathname = router.state.location.pathname;
          const m = /^\/prs\/([^/]+)\/diff$/.exec(pathname);
          if (m) {
            void router.navigate({
              to: '/prs/$prId/diff',
              params: { prId: m[1]! },
              search: { file: newKey, side: 'right' },
              replace: true,
            });
          }
        }
      }
    },
  },
  {
    actionId: 'detail.viewDiff',
    keys: 'Space',
    description: 'View diff',
    activation: { routes: ['detail'] },
    action: (ctx) => {
      // Derive prId from the current route pathname (/prs/$prId).
      const pathname = router.state.location.pathname;
      const m = /^\/prs\/([^/]+)$/.exec(pathname);
      if (!m) return;
      const prId = m[1]!;
      void router.navigate({
        to: '/prs/$prId/diff',
        params: { prId },
        search: { file: '', side: 'right' },
      });
    },
  },
  {
    actionId: 'detail.approve',
    keys: ['a', 'a'],
    description: 'Approve (double-tap)',
    activation: { routes: ['detail'] },
    action: (ctx) => {
      const { confirmingAction } = ctx.store.state;
      if (confirmingAction === 'approve') {
        ctx.store.setState((s) => ({
          ...s,
          confirmingAction: 'approve_confirmed',
        }));
      } else {
        ctx.store.setState((s) => ({ ...s, confirmingAction: 'approve' }));
      }
    },
  },
  {
    actionId: 'detail.comment',
    keys: 'c',
    description: 'Comment',
    activation: { routes: ['detail'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'comment' }));
    },
  },
  {
    actionId: 'detail.reply',
    keys: 'r',
    description: 'Reply',
    activation: { routes: ['detail'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'reply' }));
    },
  },
];

// ─── Filter overlay bindings (active when filter overlay is open) ─────────────
// Selecting a filter applies it AND closes the overlay, so `f a` behaves like a
// sequence: `f` reveals the options in the footer legend, the next key picks
// one and returns to the list.

function applyFilter(
  ctx: KeyActionContext,
  filterState: UIState['filterState'],
): void {
  ctx.store.setState((s) => ({
    ...s,
    filterState,
  }));
  ctx.overlays.closeOverlay();
}

const filterBindings: KeyBinding[] = [
  {
    actionId: 'filter.needsAction',
    keys: 'n',
    description: 'Needs Action',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      applyFilter(ctx, 'needs_action');
    },
  },
  {
    actionId: 'filter.open',
    keys: 'o',
    description: 'Open',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      applyFilter(ctx, 'open');
    },
  },
  {
    actionId: 'filter.draft',
    keys: 'd',
    description: 'Draft',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      applyFilter(ctx, 'draft');
    },
  },
  {
    actionId: 'filter.all',
    keys: 'a',
    description: 'All',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      applyFilter(ctx, 'all');
    },
  },
  {
    actionId: 'filter.close',
    keys: 'Escape',
    description: 'Close',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      ctx.overlays.closeOverlay();
    },
  },
  {
    actionId: 'filter.toggle',
    keys: 'f',
    description: 'Close',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      ctx.overlays.closeOverlay();
    },
  },
];

// ─── Detail View scroll bindings ─────────────────────────────────────────────

const detailScrollBindings: KeyBinding[] = [
  {
    actionId: 'detail.scrollLeft',
    keys: 'h',
    description: 'Scroll left',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
    },
  },
  {
    actionId: 'detail.scrollDown',
    keys: 'j',
    description: 'Scroll down',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUp',
    keys: 'k',
    description: 'Scroll up',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.scrollRight',
    keys: 'l',
    description: 'Scroll right',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
    },
  },
  {
    actionId: 'detail.scrollDownArrow',
    keys: 'ArrowDown',
    description: 'Scroll down',
    activation: { routes: ['detail', 'diff'] },
    hidden: true,
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUpArrow',
    keys: 'ArrowUp',
    description: 'Scroll up',
    activation: { routes: ['detail', 'diff'] },
    hidden: true,
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.scrollLeftArrow',
    keys: 'ArrowLeft',
    description: 'Scroll left',
    activation: { routes: ['detail', 'diff'] },
    hidden: true,
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: -80 });
    },
  },
  {
    actionId: 'detail.scrollRightArrow',
    keys: 'ArrowRight',
    description: 'Scroll right',
    activation: { routes: ['detail', 'diff'] },
    hidden: true,
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ left: 80 });
    },
  },
  {
    actionId: 'detail.pageDown',
    keys: 'Control+D',
    description: 'Page down',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 300 });
    },
  },
  {
    actionId: 'detail.pageUp',
    keys: 'Control+U',
    description: 'Page up',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -300 });
    },
  },
  {
    actionId: 'detail.scrollTop',
    keys: '0',
    description: 'Jump to top',
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
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
    activation: { routes: ['detail', 'diff'] },
    action: (_ctx) => {
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
