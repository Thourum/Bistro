import type {
  KeybindingOverrides,
  NavigationStyle,
} from '@renderer/types/settings';
import type { OverlayKind } from '../overlays/OverlayContext';
import { PROVIDERS } from '../providers';
import { KEYMAPS } from './keymaps';

export type RouteBase =
  | 'list'
  | 'detail'
  | 'diff'
  | 'settings'
  | 'login'
  | 'onboarding'
  | 'connecting';

export type HelpSourceRoute = 'list' | 'detail' | 'diff';

/** Other routes use the inbox help sections. */
export function helpSourceFor(routeBase: RouteBase | null): HelpSourceRoute {
  return routeBase === 'detail' || routeBase === 'diff' ? routeBase : 'list';
}

export interface KeyBinding {
  /** Stable id used by settings overrides. */
  actionId: string;
  keys: string | readonly string[];
  description: string;
  activation: {
    /** Omitted routes apply to every non-null route when no overlay is open. */
    routes?: readonly RouteBase[];
    /** Overlay bindings fire exclusively when their overlay is open. */
    overlays?: readonly OverlayKind[];
  };
  hidden?: boolean;
}

export type BindingDefinition = Omit<KeyBinding, 'keys'>;

export type { KeyActionContext } from './keybindingActions';
export {
  isActive,
  isBindingEnabled,
  routeBaseFromPath,
} from './keybindingActivation';

export function isSequenceBinding(
  binding: KeyBinding,
): binding is KeyBinding & { keys: readonly string[] } {
  return Array.isArray(binding.keys);
}

/** Submits a focused comment textarea; handled locally, not via useAppHotkeys. */
export const TEXTAREA_SUBMIT_HOTKEY = 'Mod+Enter';

export function isTextareaSubmit(event: {
  metaKey: boolean;
  ctrlKey: boolean;
  key: string;
}): boolean {
  return (event.metaKey || event.ctrlKey) && event.key === 'Enter';
}

/** Leaves a practice comment in the onboarding sandbox; handled locally like TEXTAREA_SUBMIT_HOTKEY. */
export const SANDBOX_COMPOSER_SUBMIT_HOTKEY = 'Shift+Space';

export function isSandboxComposerSubmit(event: {
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  key: string;
}): boolean {
  return (event.shiftKey && event.key === ' ') || isTextareaSubmit(event);
}

export const ALL_ROUTES: readonly RouteBase[] = [
  'list',
  'detail',
  'diff',
  'settings',
  'login',
  'onboarding',
  'connecting',
];
const ALL_OVERLAYS: readonly OverlayKind[] = [
  'command',
  'help',
  'search',
  'filter',
  'repoPicker',
  'addRepo',
  'threads',
  'lineComment',
];
const LIST = { routes: ['list'] } as const;
const DETAIL = { routes: ['detail'] } as const;
const DIFF = { routes: ['diff'] } as const;
const SCROLL = { routes: ['detail', 'diff'] } as const;
const FILTER = { overlays: ['filter'] } as const;
const THREADS = { overlays: ['threads'] } as const;
const LINE_COMMENT = { overlays: ['lineComment'] } as const;

const GLOBAL_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'global.help',
    description: 'Help',
    activation: { routes: ['list', 'detail', 'diff'] },
  },
  {
    actionId: 'global.escape',
    description: 'Back / Cancel',
    activation: {
      routes: ['list', 'detail', 'diff', 'settings', 'onboarding'],
      overlays: ALL_OVERLAYS,
    },
  },
  {
    actionId: 'global.cancel',
    description: 'Back / Cancel',
    activation: {
      routes: ['list', 'detail', 'diff', 'settings', 'onboarding'],
      overlays: ALL_OVERLAYS,
    },
    hidden: true,
  },
  {
    actionId: 'global.command',
    description: 'Commands',
    activation: { routes: ['list', 'detail', 'diff'] },
  },
  {
    actionId: 'global.settings',
    description: 'Settings',
    activation: { routes: ['list', 'detail'] },
  },
  {
    actionId: 'global.inbox',
    description: 'Go to Inbox',
    activation: { routes: ['list', 'detail', 'diff', 'settings'] },
  },
  {
    actionId: 'global.quit.mac',
    description: 'Quit',
    activation: { routes: ALL_ROUTES, overlays: ALL_OVERLAYS },
  },
  {
    actionId: 'global.quit.win',
    description: 'Quit',
    activation: { routes: ALL_ROUTES, overlays: ALL_OVERLAYS },
  },
  {
    actionId: 'global.search',
    description: 'Search',
    activation: { routes: ['list', 'detail'] },
  },
];

const LOGIN_BINDINGS: readonly BindingDefinition[] = PROVIDERS.filter(
  (provider) => provider.isAvailable,
).map((provider) => ({
  actionId: `login.provider.${provider.id}`,
  description: `Login with ${provider.label}`,
  activation: { routes: ['login'] },
}));

const SIGN_IN_ACTIONS: readonly BindingDefinition[] = [
  {
    actionId: 'login.reopen',
    description: 'Reopen browser',
    activation: { routes: ['login'] },
  },
  {
    actionId: 'login.copyLink',
    description: 'Copy sign-in link',
    activation: { routes: ['login'] },
  },
  {
    actionId: 'login.cancel',
    description: 'Cancel / Back to hosts',
    activation: { routes: ['login'] },
  },
  {
    actionId: 'login.retry',
    description: 'Try again',
    activation: { routes: ['login'] },
  },
];

const CONNECTING_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'connecting.openLogs',
    description: 'Open log folder',
    activation: { routes: ['connecting'] },
  },
];

const ONBOARDING_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'onboarding.repoDown',
    description: 'Next repository',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.repoUp',
    description: 'Previous repository',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.repoToggle',
    description: 'Toggle repository sync',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.repoAll',
    description: 'Sync all repositories',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.repoNone',
    description: 'Sync no repositories',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.repoFilter',
    description: 'Filter repositories',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.chooseFolder',
    description: 'Choose clone folder',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.stopScan',
    description: 'Stop folder scan',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'firstRun.hide',
    description: 'Hide first-run rail',
    activation: LIST,
  },
  {
    actionId: 'onboarding.continue',
    description: 'Continue',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.nextStyle',
    description: 'Next navigation style',
    activation: { routes: ['onboarding'] },
  },
  {
    actionId: 'onboarding.prevStyle',
    description: 'Previous navigation style',
    activation: { routes: ['onboarding'] },
  },
];

const LIST_JUMP_COUNT = 9;
const LANE_COUNT = 6;

const JUMP_BINDINGS: readonly BindingDefinition[] = Array.from(
  { length: LIST_JUMP_COUNT },
  (_, index) => {
    const digit = index + 1;
    return {
      actionId: `list.jumpTo${digit}`,
      description: digit === 1 ? 'Jump to PR (1–9)' : `Jump to PR #${digit}`,
      activation: LIST,
      hidden: digit !== 1,
    };
  },
);

const LANE_JUMP_BINDINGS: readonly BindingDefinition[] = Array.from(
  { length: LANE_COUNT },
  (_, index) => {
    const laneNumber = index + 1;
    return {
      actionId: `inbox.lane${laneNumber}`,
      description: `Jump to lane ${laneNumber}`,
      activation: LIST,
      hidden: laneNumber !== 1,
    };
  },
);

const LIST_BINDINGS: readonly BindingDefinition[] = [
  { actionId: 'list.down', description: 'Next', activation: LIST },
  { actionId: 'list.up', description: 'Previous', activation: LIST },
  {
    actionId: 'list.downArrow',
    description: 'Next',
    activation: LIST,
    hidden: true,
  },
  {
    actionId: 'list.upArrow',
    description: 'Previous',
    activation: LIST,
    hidden: true,
  },
  {
    actionId: 'list.top',
    description: 'Jump to top',
    activation: LIST,
  },
  {
    actionId: 'list.bottom',
    description: 'Jump to bottom',
    activation: LIST,
  },
  {
    actionId: 'list.open',
    description: 'Open',
    activation: LIST,
  },
  {
    actionId: 'list.search',
    description: 'Search',
    activation: LIST,
  },
  {
    actionId: 'list.filter',
    description: 'Filter',
    activation: LIST,
  },
  {
    actionId: 'list.pageDown',
    description: 'Down (5 PRs)',
    activation: LIST,
  },
  {
    actionId: 'list.pageUp',
    description: 'Up (5 PRs)',
    activation: LIST,
  },
  {
    actionId: 'inbox.markLaneRead',
    description: 'Mark lane read',
    activation: LIST,
  },
  {
    actionId: 'list.openBrowser',
    description: 'Open (in Browser)',
    activation: LIST,
  },
  {
    actionId: 'list.toggleGroup',
    description: 'Toggle group',
    activation: LIST,
  },
  {
    actionId: 'list.draft',
    description: 'Toggle draft',
    activation: LIST,
  },
  { actionId: 'list.mute', description: 'Mute', activation: LIST },
  { actionId: 'list.star', description: 'Star', activation: LIST },
  ...JUMP_BINDINGS,
  ...LANE_JUMP_BINDINGS,
];

const DETAIL_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'detail.nextFile',
    description: 'Next file',
    activation: DETAIL,
  },
  {
    actionId: 'detail.prevFile',
    description: 'Previous file',
    activation: DETAIL,
  },
  {
    actionId: 'detail.viewDiff',
    description: 'View diff',
    activation: DETAIL,
  },
  {
    actionId: 'detail.threads',
    description: 'Review threads',
    activation: DETAIL,
  },
  {
    actionId: 'detail.dismissNits',
    description: 'Dismiss nits + info',
    activation: DETAIL,
  },
  {
    actionId: 'detail.dismissBots',
    description: 'Dismiss bot comments',
    activation: DETAIL,
  },
  {
    actionId: 'detail.undoDismiss',
    description: 'Undo last dismissal',
    activation: DETAIL,
  },
  {
    actionId: 'detail.undoDismissAlt',
    description: 'Undo last dismissal',
    activation: DETAIL,
    hidden: true,
  },
  {
    actionId: 'detail.filterCritical',
    description: 'Filter ≥ critical',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterWarning',
    description: 'Filter ≥ warning',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterSuggestion',
    description: 'Filter ≥ suggestion',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterNit',
    description: 'Filter ≥ nit',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterHuman',
    description: 'Human comments only',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterClear',
    description: 'Clear comment filters',
    activation: DETAIL,
  },
  {
    actionId: 'detail.approve',
    description: 'Approve (double-tap)',
    activation: DETAIL,
  },
  {
    actionId: 'detail.comment',
    description: 'Comment',
    activation: DETAIL,
  },
  {
    actionId: 'detail.requestChanges',
    description: 'Request changes',
    activation: DETAIL,
  },
  {
    actionId: 'diff.sinceLastReview',
    description: 'Since last review: changed files',
    activation: DIFF,
  },
  {
    actionId: 'diff.interdiff',
    description: 'Interdiff: this file since last review',
    activation: DIFF,
  },
  {
    actionId: 'diff.toggleStyle',
    description: 'Toggle side-by-side',
    activation: DIFF,
  },
  {
    actionId: 'diff.openFileBrowser',
    description: 'Open file (in Browser)',
    activation: DIFF,
  },
  {
    actionId: 'diff.toggleCollapse',
    description: 'Collapse / expand file',
    activation: DIFF,
  },
  {
    actionId: 'diff.toggleWhitespace',
    description: 'Hide whitespace-only hunks',
    activation: DIFF,
  },
  {
    actionId: 'diff.markViewed',
    description: 'Mark file viewed',
    activation: DIFF,
  },
  {
    actionId: 'detail.reply',
    description: 'Reply',
    activation: DETAIL,
  },
  {
    actionId: 'diff.lineComment',
    description: 'Comment on line',
    activation: DIFF,
  },
  {
    actionId: 'diff.deleteDraft',
    description: 'Delete draft',
    activation: DIFF,
  },
  {
    actionId: 'review.toggleResolve',
    description: 'Resolve / unresolve thread',
    activation: SCROLL,
  },
];

const LINE_COMMENT_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'lineComment.down',
    description: 'Next line',
    activation: LINE_COMMENT,
  },
  {
    actionId: 'lineComment.up',
    description: 'Previous line',
    activation: LINE_COMMENT,
  },
  {
    actionId: 'lineComment.extendDown',
    description: 'Extend range down',
    activation: LINE_COMMENT,
  },
  {
    actionId: 'lineComment.extendUp',
    description: 'Extend range up',
    activation: LINE_COMMENT,
  },
  {
    actionId: 'lineComment.left',
    description: 'Old side (split view)',
    activation: LINE_COMMENT,
  },
  {
    actionId: 'lineComment.right',
    description: 'New side (split view)',
    activation: LINE_COMMENT,
  },
  {
    actionId: 'lineComment.compose',
    description: 'Write comment',
    activation: LINE_COMMENT,
  },
];

const FILTER_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'filter.needsAction',
    description: 'Needs Action',
    activation: FILTER,
  },
  {
    actionId: 'filter.touchesYou',
    description: 'Touches Your Code',
    activation: FILTER,
  },
  {
    actionId: 'filter.open',
    description: 'Open',
    activation: FILTER,
  },
  {
    actionId: 'filter.draft',
    description: 'Draft',
    activation: FILTER,
  },
  { actionId: 'filter.all', description: 'All', activation: FILTER },
  {
    actionId: 'filter.close',
    description: 'Close',
    activation: FILTER,
  },
  {
    actionId: 'filter.toggle',
    description: 'Close',
    activation: FILTER,
  },
];

const SCROLL_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'detail.scrollLeft',
    description: 'Scroll left',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollDown',
    description: 'Scroll down',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollUp',
    description: 'Scroll up',
    activation: SCROLL,
  },
  {
    actionId: 'diff.nextFile',
    description: 'Next file',
    activation: DIFF,
  },
  {
    actionId: 'diff.prevFile',
    description: 'Previous file',
    activation: DIFF,
  },
  {
    actionId: 'diff.nextComment',
    description: 'Next inline comment',
    activation: DIFF,
  },
  {
    actionId: 'diff.prevComment',
    description: 'Previous inline comment',
    activation: DIFF,
  },
  {
    actionId: 'detail.inlineReply',
    description: 'Reply to inline comment',
    activation: DIFF,
  },
  {
    actionId: 'detail.scrollRight',
    description: 'Scroll right',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollDownArrow',
    description: 'Scroll down',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.scrollUpArrow',
    description: 'Scroll up',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.scrollLeftArrow',
    description: 'Scroll left',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.scrollRightArrow',
    description: 'Scroll right',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.pageDown',
    description: 'Page down',
    activation: SCROLL,
  },
  {
    actionId: 'detail.pageUp',
    description: 'Page up',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollTop',
    description: 'Jump to top',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollBottom',
    description: 'Jump to bottom',
    activation: SCROLL,
  },
];

const THREAD_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'threads.next',
    description: 'Next thread',
    activation: THREADS,
  },
  {
    actionId: 'threads.prev',
    description: 'Previous thread',
    activation: THREADS,
  },
  {
    actionId: 'threads.resolve',
    description: 'Resolve thread',
    activation: THREADS,
  },
  {
    actionId: 'threads.unresolve',
    description: 'Unresolve thread',
    activation: THREADS,
  },
];

/** Every bindable action; keys come from the selected style's table in keymaps.ts. */
export const ACTION_CATALOG: readonly BindingDefinition[] = [
  ...GLOBAL_BINDINGS,
  ...LOGIN_BINDINGS,
  ...SIGN_IN_ACTIONS,
  ...CONNECTING_BINDINGS,
  ...ONBOARDING_BINDINGS,
  ...LIST_BINDINGS,
  ...DETAIL_BINDINGS,
  ...SCROLL_BINDINGS,
  ...THREAD_BINDINGS,
  ...FILTER_BINDINGS,
  ...LINE_COMMENT_BINDINGS,
];

/** Resolve keys: user override (empty = unbound) → style table → unbound. */
export function getEffectiveBindings(
  style: NavigationStyle,
  overrides: KeybindingOverrides,
): KeyBinding[] {
  const table = KEYMAPS[style];
  const bindings = ACTION_CATALOG.flatMap((definition) => {
    const override = overrides[definition.actionId];
    if (override !== undefined) {
      if (override.length === 0) return [];
      const keys = override.length === 1 ? override[0] : override;
      return keys === undefined ? [] : [{ ...definition, keys }];
    }
    const keys = table[definition.actionId];
    return keys === undefined ? [] : [{ ...definition, keys }];
  });
  // Arrow aliases are primary movement when the style (or override) omits hjkl.
  return bindings.map((binding) => {
    const primaryId = MOVEMENT_PRIMARY_IDS[binding.actionId];
    return primaryId === undefined
      ? binding
      : {
          ...binding,
          hidden: bindings.some(
            (candidate) =>
              candidate.actionId === primaryId && candidate.hidden !== true,
          ),
        };
  });
}

const MOVEMENT_PRIMARY_IDS: Readonly<Record<string, string>> = {
  'list.downArrow': 'list.down',
  'list.upArrow': 'list.up',
  'detail.scrollDownArrow': 'detail.scrollDown',
  'detail.scrollUpArrow': 'detail.scrollUp',
  'detail.scrollLeftArrow': 'detail.scrollLeft',
  'detail.scrollRightArrow': 'detail.scrollRight',
};

/** Resolve a primary movement action, falling back to its arrow alias. */
export function findPrimaryBinding(
  bindings: readonly KeyBinding[],
  actionId: string,
): KeyBinding | undefined {
  return (
    bindings.find((binding) => binding.actionId === actionId) ??
    bindings.find(
      (binding) => MOVEMENT_PRIMARY_IDS[binding.actionId] === actionId,
    )
  );
}
