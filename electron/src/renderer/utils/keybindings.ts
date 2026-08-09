import type { Store } from '@tanstack/store';
import type { KeybindingOverrides } from '../../types/settings';
import type {
  OverlayContextValue,
  OverlayKind,
} from '../overlays/OverlayContext';
import { authAction, PROVIDERS } from '../providers';
import { router } from '../router';
import type { UIState } from '../store/ui';
import { toggleRepoGroup } from '../store/ui';

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
interface BindingActivation {
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
  prId?: string;
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
  'addRepo',
  'threads',
  'tour',
];

const ALL_ROUTES: RouteBase[] = [
  'list',
  'detail',
  'diff',
  'settings',
  'login',
  'onboarding',
];

function moveInlineDiffComment(ctx: KeyActionContext, delta: 1 | -1): boolean {
  if (ctx.routeBase !== 'diff') return false;
  const ids = ctx.store.state.inlineCommentThreadIds;
  if (ids.length === 0) return false;

  const current = ctx.store.state.activeInlineCommentThreadId;
  const currentIndex = current === null ? -1 : ids.indexOf(current);
  const nextIndex =
    currentIndex < 0
      ? delta > 0
        ? 0
        : ids.length - 1
      : Math.min(Math.max(currentIndex + delta, 0), ids.length - 1);
  const nextId = ids[nextIndex];
  if (nextId === undefined) return false;

  ctx.store.setState((s) => ({
    ...s,
    activeInlineCommentThreadId: nextId,
    replyingInlineCommentThreadId: null,
  }));
  return true;
}

function startInlineDiffReply(ctx: KeyActionContext): boolean {
  if (ctx.routeBase !== 'diff') return false;
  const active = ctx.store.state.activeInlineCommentThreadId;
  if (active === null) return false;
  if (!ctx.store.state.inlineCommentThreadIds.includes(active)) return false;

  ctx.store.setState((s) => ({
    ...s,
    replyingInlineCommentThreadId: active,
  }));
  return true;
}

function closeInlineDiffReply(ctx: KeyActionContext): boolean {
  if (ctx.routeBase !== 'diff') return false;
  if (ctx.store.state.replyingInlineCommentThreadId === null) return false;

  ctx.store.setState((s) => ({
    ...s,
    replyingInlineCommentThreadId: null,
  }));
  return true;
}

function selectedVisibleIndex(ctx: KeyActionContext): number {
  const { selectedPRIndex, visiblePRIds } = ctx.store.state;
  return Math.min(
    Math.max(0, selectedPRIndex),
    Math.max(0, visiblePRIds.length - 1),
  );
}

/**
 * Move the list cursor by `delta`, clamped to the visible rows so the cursor
 * can never point past the last rendered row (which left the list with no
 * highlighted row). With an empty/unsynced list only the lower bound applies.
 */
function movedPRIndex(s: UIState, delta: number): number {
  const next = Math.max(0, s.selectedPRIndex + delta);
  const len = s.visiblePRIds.length;
  return len > 0 ? Math.min(next, len - 1) : next;
}

function openUrlInBrowser(url: string): void {
  if (window.electron?.openExternal !== undefined) {
    void window.electron.openExternal(url);
    return;
  }
  window.open(url, '_blank', 'noopener,noreferrer');
}

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
        if (closeInlineDiffReply(ctx)) return;
        // Navigate back to /prs/$prId
        if (ctx.prId !== undefined) {
          void router.navigate({
            to: '/prs/$prId',
            params: { prId: ctx.prId },
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
    // Keep quit available from every app route/overlay; the main process also
    // catches this accelerator before renderer focus/input handling can block it.
    activation: {
      routes: ALL_ROUTES,
      overlays: ALL_OVERLAYS,
    },
    action: (_ctx) => {
      void window.electron.quitApp();
    },
  },
  {
    actionId: 'global.quit.win',
    keys: 'Alt+F4',
    description: 'Quit',
    activation: {
      routes: ALL_ROUTES,
      overlays: ALL_OVERLAYS,
    },
    action: (_ctx) => {
      void window.electron.quitApp();
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

const laneJumpBindings: KeyBinding[] = Array.from({ length: 6 }, (_, i) => {
  const laneNumber = i + 1;
  return {
    actionId: `inbox.lane${laneNumber}`,
    keys: ['g', String(laneNumber)],
    description: `Jump to lane ${laneNumber}`,
    activation: { routes: ['list'] as RouteBase[] },
    hidden: laneNumber !== 1,
    action: (ctx: KeyActionContext) => {
      const section = ctx.store.state.laneSectionsSnapshot[i];
      if (section === undefined || section.count === 0) return;
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: section.startIndex,
      }));
    },
  };
});

function emitInboxEvent(name: string): void {
  if (typeof document === 'undefined') return;
  document.dispatchEvent(new CustomEvent(name));
}

const listBindings: KeyBinding[] = [
  {
    actionId: 'list.down',
    keys: 'j',
    description: 'Next',
    activation: { routes: ['list'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        selectedPRIndex: movedPRIndex(s, 1),
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
        selectedPRIndex: movedPRIndex(s, 1),
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
      const idx = selectedVisibleIndex(ctx);
      const prId = ctx.store.state.visiblePRIds[idx];
      if (prId === undefined) return;
      if (prId === '') {
        // Collapsed group header row — Enter expands it.
        const repoKey = ctx.store.state.visiblePRRepoKeys[idx];
        if (repoKey !== undefined) {
          ctx.store.setState((s) => toggleRepoGroup(s, repoKey));
        }
        return;
      }
      void router.navigate({ to: '/prs/$prId', params: { prId } });
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
        selectedPRIndex: movedPRIndex(s, 5),
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
    actionId: 'inbox.markLaneRead',
    keys: 'Shift+R',
    description: 'Mark lane read',
    activation: { routes: ['list'] },
    action: (_ctx) => emitInboxEvent('bistro:inbox:mark-lane-read'),
  },
  {
    actionId: 'list.openBrowser',
    keys: 'o',
    description: 'Open (in Browser)',
    activation: { routes: ['list'] },
    action: (ctx) => {
      const url =
        ctx.store.state.visiblePRBrowserUrls[selectedVisibleIndex(ctx)];
      if (url !== undefined && url !== '') {
        openUrlInBrowser(url);
      }
    },
  },
  {
    actionId: 'list.toggleGroup',
    keys: 'Space',
    description: 'Toggle group',
    activation: { routes: ['list'] },
    action: (ctx) => {
      const repoKey =
        ctx.store.state.visiblePRRepoKeys[selectedVisibleIndex(ctx)];
      if (repoKey !== undefined) {
        ctx.store.setState((s) => toggleRepoGroup(s, repoKey));
      }
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
  ...laneJumpBindings,
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
          if (ctx.prId !== undefined) {
            void router.navigate({
              to: '/prs/$prId/diff',
              params: { prId: ctx.prId },
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
          if (ctx.prId !== undefined) {
            void router.navigate({
              to: '/prs/$prId/diff',
              params: { prId: ctx.prId },
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
      if (ctx.prId === undefined) return;
      void router.navigate({
        to: '/prs/$prId/diff',
        params: { prId: ctx.prId },
        search: { file: '', side: 'right' },
      });
    },
  },
  {
    actionId: 'detail.threads',
    keys: ['g', 'u'],
    description: 'Review threads',
    activation: { routes: ['detail'] },
    action: (ctx) => {
      ctx.overlays.openThreads();
    },
  },
  // ── Phase 3.5 bulk dismiss + classification filters ────────────────────
  // Conflict notes: single-key `d`/`f` exist only on the list route and the
  // filter overlay; `u` exists only in the threads overlay — the detail
  // route is free for these. Dispatched as DOM events (emitThreadsEvent
  // pattern) so useDismissals can act on the loaded comment list.
  {
    actionId: 'detail.dismissNits',
    keys: ['d', 'n'],
    description: 'Dismiss nits + info',
    activation: { routes: ['detail'] },
    action: (_ctx) => emitThreadsEvent('bistro:comments:dismiss-nits'),
  },
  {
    actionId: 'detail.dismissBots',
    keys: ['d', 'b'],
    description: 'Dismiss bot comments',
    activation: { routes: ['detail'] },
    action: (_ctx) => emitThreadsEvent('bistro:comments:dismiss-bots'),
  },
  {
    actionId: 'detail.undoDismiss',
    keys: 'u',
    description: 'Undo last dismissal',
    activation: { routes: ['detail'] },
    action: (_ctx) => emitThreadsEvent('bistro:comments:undo-dismiss'),
  },
  {
    actionId: 'detail.filterCritical',
    keys: ['f', 'c'],
    description: 'Filter ≥ critical',
    activation: { routes: ['detail'] },
    action: (ctx) =>
      ctx.store.setState((s) => ({ ...s, commentMinSeverity: 'critical' })),
  },
  {
    actionId: 'detail.filterWarning',
    keys: ['f', 'w'],
    description: 'Filter ≥ warning',
    activation: { routes: ['detail'] },
    action: (ctx) =>
      ctx.store.setState((s) => ({ ...s, commentMinSeverity: 'warning' })),
  },
  {
    actionId: 'detail.filterSuggestion',
    keys: ['f', 's'],
    description: 'Filter ≥ suggestion',
    activation: { routes: ['detail'] },
    action: (ctx) =>
      ctx.store.setState((s) => ({ ...s, commentMinSeverity: 'suggestion' })),
  },
  {
    actionId: 'detail.filterNit',
    keys: ['f', 'n'],
    description: 'Filter ≥ nit',
    activation: { routes: ['detail'] },
    action: (ctx) =>
      ctx.store.setState((s) => ({ ...s, commentMinSeverity: 'nit' })),
  },
  {
    actionId: 'detail.filterHuman',
    keys: ['f', 'h'],
    description: 'Human comments only',
    activation: { routes: ['detail'] },
    action: (ctx) =>
      ctx.store.setState((s) => ({
        ...s,
        commentHumanOnly: !s.commentHumanOnly,
      })),
  },
  {
    actionId: 'detail.filterClear',
    keys: ['f', 'f'],
    description: 'Clear comment filters',
    activation: { routes: ['detail'] },
    action: (ctx) =>
      ctx.store.setState((s) => ({
        ...s,
        commentMinSeverity: null,
        commentHumanOnly: false,
      })),
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
    actionId: 'detail.requestChanges',
    keys: 'Shift+R',
    description: 'Request changes',
    activation: { routes: ['detail'] },
    action: (ctx) => {
      ctx.store.setState((s) => ({
        ...s,
        confirmingAction: 'request_changes',
      }));
    },
  },
  {
    actionId: 'diff.sinceLastReview',
    keys: 's',
    description: 'Since last review',
    activation: { routes: ['diff'] },
    action: (ctx) => {
      // Toggle the since-last-review range diff. DiffPage owns the request;
      // here we only flip the intent flag on the store.
      const active = ctx.store.state.confirmingAction === 'since-last-review';
      ctx.store.setState((s) => ({
        ...s,
        confirmingAction: active ? null : 'since-last-review',
      }));
    },
  },
  {
    actionId: 'diff.markViewed',
    keys: 'v',
    description: 'Mark file viewed',
    activation: { routes: ['diff'] },
    action: (ctx) => {
      // DiffPage performs the MARK_FILE_VIEWED request for the current file
      // and clears the flag once acknowledged.
      ctx.store.setState((s) => ({ ...s, confirmingAction: 'mark-viewed' }));
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
    actionId: 'filter.touchesYou',
    keys: 'y',
    description: 'Touches Your Code',
    activation: { overlays: ['filter'] },
    action: (ctx) => {
      applyFilter(ctx, 'touches_you');
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
    action: (ctx) => {
      if (moveInlineDiffComment(ctx, 1)) return;
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: 40 });
    },
  },
  {
    actionId: 'detail.scrollUp',
    keys: 'k',
    description: 'Scroll up',
    activation: { routes: ['detail', 'diff'] },
    action: (ctx) => {
      if (moveInlineDiffComment(ctx, -1)) return;
      document.querySelector('[data-diff-viewport]')?.scrollBy({ top: -40 });
    },
  },
  {
    actionId: 'detail.inlineReply',
    keys: 'Enter',
    description: 'Reply to inline comment',
    activation: { routes: ['diff'] },
    action: (ctx) => {
      startInlineDiffReply(ctx);
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

// ─── Review-threads bindings (active in threads mode only) ───────────────────
// These dispatch DOM custom events rather than mutating the store: the focused
// thread lives inside UnresolvedThreadsView (it owns the list + sort order), so
// the view listens for these events and acts on whichever row is focused.

function emitThreadsEvent(name: string): void {
  if (typeof document === 'undefined') return;
  document.dispatchEvent(new CustomEvent(name));
}

const threadsBindings: KeyBinding[] = [
  {
    actionId: 'threads.next',
    keys: 'j',
    description: 'Next thread',
    activation: { overlays: ['threads'] },
    action: (_ctx) => emitThreadsEvent('bistro:threads:next'),
  },
  {
    actionId: 'threads.prev',
    keys: 'k',
    description: 'Previous thread',
    activation: { overlays: ['threads'] },
    action: (_ctx) => emitThreadsEvent('bistro:threads:prev'),
  },
  {
    actionId: 'threads.resolve',
    keys: 'r',
    description: 'Resolve thread',
    activation: { overlays: ['threads'] },
    action: (_ctx) => emitThreadsEvent('bistro:threads:resolve'),
  },
  {
    actionId: 'threads.unresolve',
    keys: 'u',
    description: 'Unresolve thread',
    activation: { overlays: ['threads'] },
    action: (_ctx) => emitThreadsEvent('bistro:threads:unresolve'),
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
  ...threadsBindings,
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
