import type { NavigationStyle } from '../../types/settings';
import { PROVIDERS } from '../providers';

/** actionId → hotkey; a string is one hotkey, an array is a sequence. */
export type KeyTable = Readonly<Record<string, string | readonly string[]>>;

const LIST_JUMP_COUNT = 9;
const LANE_COUNT = 6;

function jumpKeys(format: (digit: number) => string): KeyTable {
  return Object.fromEntries(
    Array.from({ length: LIST_JUMP_COUNT }, (_, index) => [
      `list.jumpTo${index + 1}`,
      format(index + 1),
    ]),
  );
}

function laneKeys(
  format: (lane: number) => string | readonly string[],
): KeyTable {
  return Object.fromEntries(
    Array.from({ length: LANE_COUNT }, (_, index) => [
      `inbox.lane${index + 1}`,
      format(index + 1),
    ]),
  );
}

/** Identical in every style; see the spec's "Shared" table. */
const SHARED_KEYS: KeyTable = {
  'global.escape': 'Escape',
  'global.settings': 'Mod+,',
  'global.quit.mac': 'Meta+Q',
  'global.quit.win': 'Alt+F4',
  'onboarding.finish': 'Space',
  ...Object.fromEntries(
    PROVIDERS.map((provider) => [
      `login.provider.${provider.id}`,
      provider.key,
    ]),
  ),
  'list.downArrow': 'ArrowDown',
  'list.upArrow': 'ArrowUp',
  'list.open': 'Enter',
  'detail.scrollDownArrow': 'ArrowDown',
  'detail.scrollUpArrow': 'ArrowUp',
  'detail.scrollLeftArrow': 'ArrowLeft',
  'detail.scrollRightArrow': 'ArrowRight',
  'detail.inlineReply': 'Enter',
  'filter.needsAction': 'n',
  'filter.touchesYou': 'y',
  'filter.open': 'o',
  'filter.draft': 'd',
  'filter.all': 'a',
  'filter.close': 'Escape',
  'threads.resolve': 'r',
  'threads.unresolve': 'u',
};

const VIM_KEYS: KeyTable = {
  ...SHARED_KEYS,
  ...jumpKeys(String),
  ...laneKeys((lane) => ['g', String(lane)]),
  'global.help': 'Shift+/',
  'global.command': 'Shift+;',
  'global.search': 'Mod+/',
  'list.down': 'j',
  'list.up': 'k',
  'list.top': ['g', 'g'],
  'list.bottom': 'Shift+G',
  'list.search': '/',
  'list.filter': 'f',
  'filter.toggle': 'f',
  'list.pageDown': 'Control+D',
  'list.pageUp': 'Control+U',
  'inbox.markLaneRead': 'Shift+R',
  'list.openBrowser': 'o',
  'list.toggleGroup': 'Space',
  'list.draft': 'd',
  'list.mute': 'm',
  'list.star': 's',
  'detail.nextFile': 'n',
  'detail.prevFile': 'p',
  'detail.viewDiff': 'Space',
  'detail.threads': ['g', 'u'],
  'detail.dismissNits': ['d', 'n'],
  'detail.dismissBots': ['d', 'b'],
  'detail.undoDismiss': 'u',
  'detail.filterCritical': ['f', 'c'],
  'detail.filterWarning': ['f', 'w'],
  'detail.filterSuggestion': ['f', 's'],
  'detail.filterNit': ['f', 'n'],
  'detail.filterHuman': ['f', 'h'],
  'detail.filterClear': ['f', 'f'],
  'detail.approve': ['a', 'a'],
  'detail.comment': 'c',
  'detail.reply': 'r',
  'detail.requestChanges': 'Shift+R',
  'diff.sinceLastReview': 's',
  'diff.toggleStyle': 'Shift+S',
  'diff.openFileBrowser': 'o',
  'diff.toggleCollapse': 'z',
  'diff.toggleWhitespace': 'w',
  'diff.markViewed': 'v',
  'diff.nextFile': 'Shift+J',
  'diff.prevFile': 'Shift+K',
  'diff.nextComment': 'n',
  'diff.prevComment': 'p',
  'detail.scrollLeft': 'h',
  'detail.scrollDown': 'j',
  'detail.scrollUp': 'k',
  'detail.scrollRight': 'l',
  'detail.pageDown': 'Control+D',
  'detail.pageUp': 'Control+U',
  'detail.scrollTop': '0',
  'detail.scrollBottom': 'Shift+4',
  'threads.next': 'j',
  'threads.prev': 'k',
};

/**
 * Per-style key tables. emacs/standard point at vim until their tables land
 * in the next task; the style can't be selected in the UI before then.
 */
export const KEYMAPS: Readonly<Record<NavigationStyle, KeyTable>> = {
  vim: VIM_KEYS,
  emacs: VIM_KEYS,
  standard: VIM_KEYS,
} satisfies Record<NavigationStyle, KeyTable>;

/** Sequence steps must follow each other within this window. */
export const SEQUENCE_TIMEOUT_MS: Readonly<Record<NavigationStyle, number>> = {
  vim: 500,
  emacs: 500,
  standard: 500,
} satisfies Record<NavigationStyle, number>;
