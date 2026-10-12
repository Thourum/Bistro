import type { KeybindingOverrides } from '../../types/settings';
import type { OverlayKind } from '../overlays/OverlayContext';
import { PROVIDERS } from '../providers';

export type RouteBase =
  | 'list'
  | 'detail'
  | 'diff'
  | 'settings'
  | 'login'
  | 'onboarding';

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

export type { KeyActionContext } from './keybindingActions';
export { isActive, routeBaseFromPath } from './keybindingActivation';

export function isSequenceBinding(
  binding: KeyBinding,
): binding is KeyBinding & { keys: readonly string[] } {
  return Array.isArray(binding.keys);
}

const ALL_ROUTES: readonly RouteBase[] = [
  'list',
  'detail',
  'diff',
  'settings',
  'login',
  'onboarding',
];
const ALL_OVERLAYS: readonly OverlayKind[] = [
  'command',
  'help',
  'search',
  'filter',
  'repoPicker',
  'addRepo',
  'threads',
  'tour',
];
const LIST = { routes: ['list'] } as const;
const DETAIL = { routes: ['detail'] } as const;
const DIFF = { routes: ['diff'] } as const;
const SCROLL = { routes: ['detail', 'diff'] } as const;
const FILTER = { overlays: ['filter'] } as const;
const THREADS = { overlays: ['threads'] } as const;

const GLOBAL_BINDINGS: readonly KeyBinding[] = [
  {
    actionId: 'global.help',
    keys: 'Shift+/',
    description: 'Help',
    activation: { routes: ['list', 'detail', 'diff'] },
  },
  {
    actionId: 'global.escape',
    keys: 'Escape',
    description: 'Back / Cancel',
    activation: {
      routes: ['list', 'detail', 'diff', 'settings'],
      overlays: ALL_OVERLAYS,
    },
  },
  {
    actionId: 'global.command',
    keys: 'Shift+;',
    description: 'Commands',
    activation: { routes: ['list', 'detail', 'diff'] },
  },
  {
    actionId: 'global.settings',
    keys: 'Mod+,',
    description: 'Settings',
    activation: { routes: ['list', 'detail'] },
  },
  {
    actionId: 'global.quit.mac',
    keys: 'Meta+Q',
    description: 'Quit',
    activation: { routes: ALL_ROUTES, overlays: ALL_OVERLAYS },
  },
  {
    actionId: 'global.quit.win',
    keys: 'Alt+F4',
    description: 'Quit',
    activation: { routes: ALL_ROUTES, overlays: ALL_OVERLAYS },
  },
  {
    actionId: 'global.search',
    keys: 'Mod+/',
    description: 'Search',
    activation: { routes: ['list', 'detail'] },
  },
];

const LOGIN_BINDINGS: readonly KeyBinding[] = PROVIDERS.map((provider) => ({
  actionId: `login.provider.${provider.id}`,
  keys: provider.key,
  description: `Login with ${provider.label}`,
  activation: { routes: ['login'] },
}));

const JUMP_BINDINGS: readonly KeyBinding[] = Array.from(
  { length: 9 },
  (_, index) => {
    const digit = index + 1;
    return {
      actionId: `list.jumpTo${digit}`,
      keys: String(digit),
      description: digit === 1 ? 'Jump to PR (1–9)' : `Jump to PR #${digit}`,
      activation: LIST,
      hidden: digit !== 1,
    };
  },
);

const LANE_JUMP_BINDINGS: readonly KeyBinding[] = Array.from(
  { length: 6 },
  (_, index) => {
    const laneNumber = index + 1;
    return {
      actionId: `inbox.lane${laneNumber}`,
      keys: ['g', String(laneNumber)],
      description: `Jump to lane ${laneNumber}`,
      activation: LIST,
      hidden: laneNumber !== 1,
    };
  },
);

const LIST_BINDINGS: readonly KeyBinding[] = [
  { actionId: 'list.down', keys: 'j', description: 'Next', activation: LIST },
  { actionId: 'list.up', keys: 'k', description: 'Previous', activation: LIST },
  {
    actionId: 'list.downArrow',
    keys: 'ArrowDown',
    description: 'Next',
    activation: LIST,
    hidden: true,
  },
  {
    actionId: 'list.upArrow',
    keys: 'ArrowUp',
    description: 'Previous',
    activation: LIST,
    hidden: true,
  },
  {
    actionId: 'list.top',
    keys: ['g', 'g'],
    description: 'Jump to top',
    activation: LIST,
  },
  {
    actionId: 'list.bottom',
    keys: 'Shift+G',
    description: 'Jump to bottom',
    activation: LIST,
  },
  {
    actionId: 'list.open',
    keys: 'Enter',
    description: 'Open',
    activation: LIST,
  },
  {
    actionId: 'list.search',
    keys: '/',
    description: 'Search',
    activation: LIST,
  },
  {
    actionId: 'list.filter',
    keys: 'f',
    description: 'Filter',
    activation: LIST,
  },
  {
    actionId: 'list.pageDown',
    keys: 'Control+D',
    description: 'Down (5 PRs)',
    activation: LIST,
  },
  {
    actionId: 'list.pageUp',
    keys: 'Control+U',
    description: 'Up (5 PRs)',
    activation: LIST,
  },
  {
    actionId: 'inbox.markLaneRead',
    keys: 'Shift+R',
    description: 'Mark lane read',
    activation: LIST,
  },
  {
    actionId: 'list.openBrowser',
    keys: 'o',
    description: 'Open (in Browser)',
    activation: LIST,
  },
  {
    actionId: 'list.toggleGroup',
    keys: 'Space',
    description: 'Toggle group',
    activation: LIST,
  },
  {
    actionId: 'list.draft',
    keys: 'd',
    description: 'Toggle draft',
    activation: LIST,
  },
  { actionId: 'list.mute', keys: 'm', description: 'Mute', activation: LIST },
  { actionId: 'list.star', keys: 's', description: 'Star', activation: LIST },
  ...JUMP_BINDINGS,
  ...LANE_JUMP_BINDINGS,
];

const DETAIL_BINDINGS: readonly KeyBinding[] = [
  {
    actionId: 'detail.nextFile',
    keys: 'n',
    description: 'Next file',
    activation: DETAIL,
  },
  {
    actionId: 'detail.prevFile',
    keys: 'p',
    description: 'Previous file',
    activation: DETAIL,
  },
  {
    actionId: 'detail.viewDiff',
    keys: 'Space',
    description: 'View diff',
    activation: DETAIL,
  },
  {
    actionId: 'detail.threads',
    keys: ['g', 'u'],
    description: 'Review threads',
    activation: DETAIL,
  },
  {
    actionId: 'detail.dismissNits',
    keys: ['d', 'n'],
    description: 'Dismiss nits + info',
    activation: DETAIL,
  },
  {
    actionId: 'detail.dismissBots',
    keys: ['d', 'b'],
    description: 'Dismiss bot comments',
    activation: DETAIL,
  },
  {
    actionId: 'detail.undoDismiss',
    keys: 'u',
    description: 'Undo last dismissal',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterCritical',
    keys: ['f', 'c'],
    description: 'Filter ≥ critical',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterWarning',
    keys: ['f', 'w'],
    description: 'Filter ≥ warning',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterSuggestion',
    keys: ['f', 's'],
    description: 'Filter ≥ suggestion',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterNit',
    keys: ['f', 'n'],
    description: 'Filter ≥ nit',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterHuman',
    keys: ['f', 'h'],
    description: 'Human comments only',
    activation: DETAIL,
  },
  {
    actionId: 'detail.filterClear',
    keys: ['f', 'f'],
    description: 'Clear comment filters',
    activation: DETAIL,
  },
  {
    actionId: 'detail.approve',
    keys: ['a', 'a'],
    description: 'Approve (double-tap)',
    activation: DETAIL,
  },
  {
    actionId: 'detail.comment',
    keys: 'c',
    description: 'Comment',
    activation: DETAIL,
  },
  {
    actionId: 'detail.requestChanges',
    keys: 'Shift+R',
    description: 'Request changes',
    activation: DETAIL,
  },
  {
    actionId: 'diff.sinceLastReview',
    keys: 's',
    description: 'Since last review',
    activation: DIFF,
  },
  {
    actionId: 'diff.toggleStyle',
    keys: 'Shift+S',
    description: 'Toggle side-by-side',
    activation: DIFF,
  },
  {
    actionId: 'diff.openFileBrowser',
    keys: 'o',
    description: 'Open file (in Browser)',
    activation: DIFF,
  },
  {
    actionId: 'diff.toggleCollapse',
    keys: 'z',
    description: 'Collapse / expand file',
    activation: DIFF,
  },
  {
    actionId: 'diff.toggleWhitespace',
    keys: 'w',
    description: 'Hide whitespace-only hunks',
    activation: DIFF,
  },
  {
    actionId: 'diff.markViewed',
    keys: 'v',
    description: 'Mark file viewed',
    activation: DIFF,
  },
  {
    actionId: 'detail.reply',
    keys: 'r',
    description: 'Reply',
    activation: DETAIL,
  },
];

const FILTER_BINDINGS: readonly KeyBinding[] = [
  {
    actionId: 'filter.needsAction',
    keys: 'n',
    description: 'Needs Action',
    activation: FILTER,
  },
  {
    actionId: 'filter.touchesYou',
    keys: 'y',
    description: 'Touches Your Code',
    activation: FILTER,
  },
  {
    actionId: 'filter.open',
    keys: 'o',
    description: 'Open',
    activation: FILTER,
  },
  {
    actionId: 'filter.draft',
    keys: 'd',
    description: 'Draft',
    activation: FILTER,
  },
  { actionId: 'filter.all', keys: 'a', description: 'All', activation: FILTER },
  {
    actionId: 'filter.close',
    keys: 'Escape',
    description: 'Close',
    activation: FILTER,
  },
  {
    actionId: 'filter.toggle',
    keys: 'f',
    description: 'Close',
    activation: FILTER,
  },
];

const SCROLL_BINDINGS: readonly KeyBinding[] = [
  {
    actionId: 'detail.scrollLeft',
    keys: 'h',
    description: 'Scroll left',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollDown',
    keys: 'j',
    description: 'Scroll down',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollUp',
    keys: 'k',
    description: 'Scroll up',
    activation: SCROLL,
  },
  {
    actionId: 'diff.nextFile',
    keys: 'Shift+J',
    description: 'Next file',
    activation: DIFF,
  },
  {
    actionId: 'diff.prevFile',
    keys: 'Shift+K',
    description: 'Previous file',
    activation: DIFF,
  },
  {
    actionId: 'diff.nextComment',
    keys: 'n',
    description: 'Next inline comment',
    activation: DIFF,
  },
  {
    actionId: 'diff.prevComment',
    keys: 'p',
    description: 'Previous inline comment',
    activation: DIFF,
  },
  {
    actionId: 'detail.inlineReply',
    keys: 'Enter',
    description: 'Reply to inline comment',
    activation: DIFF,
  },
  {
    actionId: 'detail.scrollRight',
    keys: 'l',
    description: 'Scroll right',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollDownArrow',
    keys: 'ArrowDown',
    description: 'Scroll down',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.scrollUpArrow',
    keys: 'ArrowUp',
    description: 'Scroll up',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.scrollLeftArrow',
    keys: 'ArrowLeft',
    description: 'Scroll left',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.scrollRightArrow',
    keys: 'ArrowRight',
    description: 'Scroll right',
    activation: SCROLL,
    hidden: true,
  },
  {
    actionId: 'detail.pageDown',
    keys: 'Control+D',
    description: 'Page down',
    activation: SCROLL,
  },
  {
    actionId: 'detail.pageUp',
    keys: 'Control+U',
    description: 'Page up',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollTop',
    keys: '0',
    description: 'Jump to top',
    activation: SCROLL,
  },
  {
    actionId: 'detail.scrollBottom',
    keys: 'Shift+4',
    description: 'Jump to bottom',
    activation: SCROLL,
  },
];

const THREAD_BINDINGS: readonly KeyBinding[] = [
  {
    actionId: 'threads.next',
    keys: 'j',
    description: 'Next thread',
    activation: THREADS,
  },
  {
    actionId: 'threads.prev',
    keys: 'k',
    description: 'Previous thread',
    activation: THREADS,
  },
  {
    actionId: 'threads.resolve',
    keys: 'r',
    description: 'Resolve thread',
    activation: THREADS,
  },
  {
    actionId: 'threads.unresolve',
    keys: 'u',
    description: 'Unresolve thread',
    activation: THREADS,
  },
];

/** Default bindings; settings overrides replace only keys, not actions. */
export const DEFAULT_KEYBINDINGS: readonly KeyBinding[] = [
  ...GLOBAL_BINDINGS,
  ...LOGIN_BINDINGS,
  ...LIST_BINDINGS,
  ...DETAIL_BINDINGS,
  ...SCROLL_BINDINGS,
  ...THREAD_BINDINGS,
  ...FILTER_BINDINGS,
];

export function getEffectiveBindings(
  overrides: KeybindingOverrides,
): KeyBinding[] {
  return DEFAULT_KEYBINDINGS.flatMap((binding) => {
    const override = overrides[binding.actionId];
    if (override === undefined) return [binding];
    if (override.length === 0) return [];
    const keys = override.length === 1 ? override[0] : override;
    return keys === undefined ? [] : [{ ...binding, keys }];
  });
}
