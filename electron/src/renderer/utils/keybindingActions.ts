import type { Store } from '@tanstack/store';
import type {
  OverlayContextValue,
  OverlayKind,
} from '../overlays/OverlayContext';
import { authAction, PROVIDERS } from '../providers';
import { router } from '../router';
import { toggleRepoGroup, type UIState } from '../store/ui';
import type { RouteBase } from './keybindings';
import { createLogger } from './logger';
import { openUrlInBrowser } from './openUrl';

export interface KeyActionContext {
  store: Store<UIState>;
  routeBase: RouteBase | null;
  activeOverlay: OverlayKind | null;
  overlays: OverlayContextValue;
  location: { pathname: string };
  prId?: string;
}

type KeyAction = (context: KeyActionContext) => void;
const LIST_PAGE_STEP = 5;
const SCROLL_LINE_PX = 40;
const SCROLL_HORIZONTAL_PX = 80;
const SCROLL_PAGE_PX = 300;
const log = createLogger('KeybindingActions');

function emitAppEvent(name: string): void {
  if (typeof document === 'undefined') return;
  document.dispatchEvent(new CustomEvent(name));
}

function selectedVisibleIndex(context: KeyActionContext): number {
  const { selectedPRIndex, visiblePRIds } = context.store.state;
  return Math.min(
    Math.max(0, selectedPRIndex),
    Math.max(0, visiblePRIds.length - 1),
  );
}

function movedPRIndex(state: UIState, delta: number): number {
  const next = Math.max(0, state.selectedPRIndex + delta);
  const length = state.visiblePRIds.length;
  return length > 0 ? Math.min(next, length - 1) : next;
}

function moveInlineDiffComment(context: KeyActionContext, delta: 1 | -1): void {
  if (context.routeBase !== 'diff') return;
  const ids = context.store.state.inlineCommentThreadIds;
  if (ids.length === 0) return;
  const current = context.store.state.activeInlineCommentThreadId;
  const currentIndex = current === null ? -1 : ids.indexOf(current);
  const nextIndex =
    currentIndex < 0
      ? delta > 0
        ? 0
        : ids.length - 1
      : Math.min(Math.max(currentIndex + delta, 0), ids.length - 1);
  const nextId = ids[nextIndex];
  if (nextId === undefined) return;
  context.store.setState((state) => ({
    ...state,
    activeInlineCommentThreadId: nextId,
    replyingInlineCommentThreadId: null,
  }));
}

function startInlineDiffReply(context: KeyActionContext): void {
  if (context.routeBase !== 'diff') return;
  const active = context.store.state.activeInlineCommentThreadId;
  if (
    active === null ||
    !context.store.state.inlineCommentThreadIds.includes(active)
  )
    return;
  context.store.setState((state) => ({
    ...state,
    replyingInlineCommentThreadId: active,
  }));
}

function closeInlineDiffReply(context: KeyActionContext): boolean {
  if (
    context.routeBase !== 'diff' ||
    context.store.state.replyingInlineCommentThreadId === null
  )
    return false;
  context.store.setState((state) => ({
    ...state,
    replyingInlineCommentThreadId: null,
  }));
  return true;
}

function openHelp(context: KeyActionContext): void {
  const source =
    context.routeBase === 'detail' || context.routeBase === 'diff'
      ? context.routeBase
      : 'list';
  context.overlays.openHelp(source);
}

function handleEscape(context: KeyActionContext): void {
  const { activeOverlay, routeBase } = context;
  if (activeOverlay === 'search') {
    context.overlays.closeSearch({ clearQuery: true });
    return;
  }
  if (activeOverlay !== null) {
    context.overlays.closeOverlay();
    return;
  }
  if (routeBase === 'diff') {
    if (closeInlineDiffReply(context)) return;
    if (context.prId !== undefined) {
      void router.navigate({
        to: '/prs/$prId',
        params: { prId: context.prId },
      });
    }
    return;
  }
  if (routeBase === 'detail' || routeBase === 'settings') {
    void router.navigate({
      to: '/prs',
      search: (previous: Record<string, unknown>) => previous,
    });
    return;
  }
  if (routeBase === 'list') {
    context.store.setState((state) => ({ ...state, confirmingAction: null }));
  }
}

function openSelectedPR(context: KeyActionContext): void {
  const index = selectedVisibleIndex(context);
  const prId = context.store.state.visiblePRIds[index];
  if (prId === undefined) return;
  if (prId === '') {
    const repoKey = context.store.state.visiblePRRepoKeys[index];
    if (repoKey !== undefined)
      context.store.setState((state) => toggleRepoGroup(state, repoKey));
    return;
  }
  void router.navigate({ to: '/prs/$prId', params: { prId } });
}

function openSelectedPRInBrowser(context: KeyActionContext): void {
  const url =
    context.store.state.visiblePRBrowserUrls[selectedVisibleIndex(context)];
  if (url === undefined || url === '') return;
  void openUrlInBrowser(url).mapErr((error) =>
    log.error({ error }, 'failed to open selected PR in browser'),
  );
}

function toggleSelectedGroup(context: KeyActionContext): void {
  const repoKey =
    context.store.state.visiblePRRepoKeys[selectedVisibleIndex(context)];
  if (repoKey !== undefined)
    context.store.setState((state) => toggleRepoGroup(state, repoKey));
}

function selectDetailFile(context: KeyActionContext, delta: 1 | -1): void {
  const { diffFileKeys, selectedFileIndex } = context.store.state;
  const selectedFile =
    delta > 0
      ? Math.min(selectedFileIndex + 1, Math.max(0, diffFileKeys.length - 1))
      : Math.max(0, selectedFileIndex - 1);
  context.store.setState((state) => ({
    ...state,
    selectedFileIndex: selectedFile,
  }));
}

function selectDiffFile(context: KeyActionContext, delta: 1 | -1): void {
  const { diffFileKeys, selectedFileIndex } = context.store.state;
  const nextIndex =
    delta > 0
      ? Math.min(
          selectedFileIndex + delta,
          Math.max(0, diffFileKeys.length - 1),
        )
      : Math.max(0, selectedFileIndex + delta);
  context.store.setState((state) => ({
    ...state,
    selectedFileIndex: nextIndex,
  }));
  if (context.prId === undefined) return;
  const file = diffFileKeys[nextIndex];
  if (file === undefined) return;
  void router.navigate({
    to: '/prs/$prId/diff',
    params: { prId: context.prId },
    search: { file, side: 'right' },
    replace: true,
  });
}

function viewDiff(context: KeyActionContext): void {
  if (context.prId === undefined) return;
  void router.navigate({
    to: '/prs/$prId/diff',
    params: { prId: context.prId },
    search: { file: '', side: 'right' },
  });
}

function toggleApproval(context: KeyActionContext): void {
  const confirmingAction =
    context.store.state.confirmingAction === 'approve'
      ? 'approve_confirmed'
      : 'approve';
  context.store.setState((state) => ({ ...state, confirmingAction }));
}

function toggleSinceLastReview(context: KeyActionContext): void {
  const isActive = context.store.state.confirmingAction === 'since-last-review';
  context.store.setState((state) => ({
    ...state,
    confirmingAction: isActive ? null : 'since-last-review',
  }));
}

function toggleDiffCollapse(context: KeyActionContext): void {
  const key =
    context.store.state.diffFileKeys[context.store.state.selectedFileIndex];
  if (key === undefined || key === '') return;
  context.store.setState((state) => ({
    ...state,
    diffCollapseOverrides: state.diffCollapseOverrides.includes(key)
      ? state.diffCollapseOverrides.filter((entry) => entry !== key)
      : [...state.diffCollapseOverrides, key],
  }));
}

function applyFilter(
  context: KeyActionContext,
  filterState: UIState['filterState'],
): void {
  context.store.setState((state) => ({ ...state, filterState }));
  context.overlays.closeOverlay();
}

function scrollDiff(offset: { top?: number; left?: number }): void {
  document.querySelector('[data-diff-viewport]')?.scrollBy(offset);
}

function scrollDiffToTop(): void {
  const viewport = document.querySelector('[data-diff-viewport]');
  if (viewport instanceof HTMLElement) viewport.scrollTop = 0;
}

function scrollDiffToBottom(): void {
  const viewport = document.querySelector('[data-diff-viewport]');
  if (viewport instanceof HTMLElement)
    viewport.scrollTop = viewport.scrollHeight;
}

const ACTIONS: Readonly<Record<string, KeyAction>> = {
  'global.help': openHelp,
  'global.escape': handleEscape,
  'onboarding.finish': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'onboarding_finish',
    })),
  'global.command': (context) => context.overlays.openCommand(),
  'global.settings': () => {
    void router.navigate({ to: '/settings' });
  },
  'global.quit.mac': () => {
    void window.electron.quitApp();
  },
  'global.quit.win': () => {
    void window.electron.quitApp();
  },
  'global.search': (context) => context.overlays.openSearch(),
  'list.down': (context) =>
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: movedPRIndex(state, 1),
    })),
  'list.up': (context) =>
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: Math.max(0, state.selectedPRIndex - 1),
    })),
  'list.downArrow': (context) =>
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: movedPRIndex(state, 1),
    })),
  'list.upArrow': (context) =>
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: Math.max(0, state.selectedPRIndex - 1),
    })),
  'list.top': (context) =>
    context.store.setState((state) => ({ ...state, selectedPRIndex: 0 })),
  'list.bottom': (context) => {
    const length = context.store.state.visiblePRIds.length;
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: length > 0 ? length - 1 : 0,
    }));
  },
  'list.open': openSelectedPR,
  'list.search': (context) => context.overlays.openSearch(),
  'list.filter': (context) => context.overlays.toggleFilter(),
  'list.pageDown': (context) =>
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: movedPRIndex(state, LIST_PAGE_STEP),
    })),
  'list.pageUp': (context) =>
    context.store.setState((state) => ({
      ...state,
      selectedPRIndex: Math.max(0, state.selectedPRIndex - LIST_PAGE_STEP),
    })),
  'inbox.markLaneRead': () => emitAppEvent('bistro:inbox:mark-lane-read'),
  'list.openBrowser': openSelectedPRInBrowser,
  'list.toggleGroup': toggleSelectedGroup,
  'list.draft': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'draft',
    })),
  'list.mute': (context) =>
    context.store.setState((state) => ({ ...state, confirmingAction: 'mute' })),
  'list.star': (context) =>
    context.store.setState((state) => ({ ...state, confirmingAction: 'star' })),
  'detail.nextFile': (context) => selectDetailFile(context, 1),
  'detail.prevFile': (context) => selectDetailFile(context, -1),
  'detail.viewDiff': viewDiff,
  'detail.threads': (context) => context.overlays.openThreads(),
  'detail.dismissNits': () => emitAppEvent('bistro:comments:dismiss-nits'),
  'detail.dismissBots': () => emitAppEvent('bistro:comments:dismiss-bots'),
  'detail.undoDismiss': () => emitAppEvent('bistro:comments:undo-dismiss'),
  'detail.filterCritical': (context) =>
    context.store.setState((state) => ({
      ...state,
      commentMinSeverity: 'critical',
    })),
  'detail.filterWarning': (context) =>
    context.store.setState((state) => ({
      ...state,
      commentMinSeverity: 'warning',
    })),
  'detail.filterSuggestion': (context) =>
    context.store.setState((state) => ({
      ...state,
      commentMinSeverity: 'suggestion',
    })),
  'detail.filterNit': (context) =>
    context.store.setState((state) => ({
      ...state,
      commentMinSeverity: 'nit',
    })),
  'detail.filterHuman': (context) =>
    context.store.setState((state) => ({
      ...state,
      commentHumanOnly: !state.commentHumanOnly,
    })),
  'detail.filterClear': (context) =>
    context.store.setState((state) => ({
      ...state,
      commentMinSeverity: null,
      commentHumanOnly: false,
    })),
  'detail.approve': toggleApproval,
  'detail.comment': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'comment',
    })),
  'detail.requestChanges': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'request_changes',
    })),
  'diff.sinceLastReview': toggleSinceLastReview,
  'diff.toggleStyle': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'toggle-diff-style',
    })),
  'diff.openFileBrowser': () => emitAppEvent('bistro:diff:open-file-browser'),
  'diff.toggleCollapse': toggleDiffCollapse,
  'diff.toggleWhitespace': (context) =>
    context.store.setState((state) => ({
      ...state,
      hideWhitespaceHunks: !state.hideWhitespaceHunks,
    })),
  'diff.markViewed': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'mark-viewed',
    })),
  'detail.reply': (context) =>
    context.store.setState((state) => ({
      ...state,
      confirmingAction: 'reply',
    })),
  'filter.needsAction': (context) => applyFilter(context, 'needs_action'),
  'filter.touchesYou': (context) => applyFilter(context, 'touches_you'),
  'filter.open': (context) => applyFilter(context, 'open'),
  'filter.draft': (context) => applyFilter(context, 'draft'),
  'filter.all': (context) => applyFilter(context, 'all'),
  'filter.close': (context) => context.overlays.closeOverlay(),
  'filter.toggle': (context) => context.overlays.closeOverlay(),
  'detail.scrollLeft': () => scrollDiff({ left: -SCROLL_HORIZONTAL_PX }),
  'detail.scrollDown': () => scrollDiff({ top: SCROLL_LINE_PX }),
  'detail.scrollUp': () => scrollDiff({ top: -SCROLL_LINE_PX }),
  'diff.nextFile': (context) => selectDiffFile(context, 1),
  'diff.prevFile': (context) => selectDiffFile(context, -1),
  'diff.nextComment': (context) => moveInlineDiffComment(context, 1),
  'diff.prevComment': (context) => moveInlineDiffComment(context, -1),
  'detail.inlineReply': startInlineDiffReply,
  'detail.scrollRight': () => scrollDiff({ left: SCROLL_HORIZONTAL_PX }),
  'detail.scrollDownArrow': () => scrollDiff({ top: SCROLL_LINE_PX }),
  'detail.scrollUpArrow': () => scrollDiff({ top: -SCROLL_LINE_PX }),
  'detail.scrollLeftArrow': () => scrollDiff({ left: -SCROLL_HORIZONTAL_PX }),
  'detail.scrollRightArrow': () => scrollDiff({ left: SCROLL_HORIZONTAL_PX }),
  'detail.pageDown': () => scrollDiff({ top: SCROLL_PAGE_PX }),
  'detail.pageUp': () => scrollDiff({ top: -SCROLL_PAGE_PX }),
  'detail.scrollTop': scrollDiffToTop,
  'detail.scrollBottom': scrollDiffToBottom,
  'threads.next': () => emitAppEvent('bistro:threads:next'),
  'threads.prev': () => emitAppEvent('bistro:threads:prev'),
  'threads.resolve': () => emitAppEvent('bistro:threads:resolve'),
  'threads.unresolve': () => emitAppEvent('bistro:threads:unresolve'),
};

/** Find the handler for a configured binding; undefined means the action is not registered. */
export function getBindingAction(actionId: string): KeyAction | undefined {
  const jump = /^list\.jumpTo([1-9])$/.exec(actionId);
  if (jump !== null) {
    const digit = Number(jump[1]);
    return (context) =>
      context.store.setState((state) => ({
        ...state,
        selectedPRIndex: digit - 1,
      }));
  }
  const lane = /^inbox\.lane([1-6])$/.exec(actionId);
  if (lane !== null) {
    const laneIndex = Number(lane[1]) - 1;
    return (context) => {
      const section = context.store.state.laneSectionsSnapshot[laneIndex];
      if (section === undefined || section.count === 0) return;
      context.store.setState((state) => ({
        ...state,
        selectedPRIndex: section.startIndex,
      }));
    };
  }
  const provider = PROVIDERS.find(
    (entry) => `login.provider.${entry.id}` === actionId,
  );
  if (provider !== undefined) {
    return (context) =>
      context.store.setState((state) => ({
        ...state,
        confirmingAction: authAction(provider),
      }));
  }
  return ACTIONS[actionId];
}
