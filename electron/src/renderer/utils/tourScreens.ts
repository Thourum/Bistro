import type { KeyBinding } from './keybindings';
import { formatBindingKeys } from './legendPresets';

/** One key hint rendered on a tour screen: the formatted keys plus a label. */
export interface TourKeyHint {
  keys: string;
  label: string;
  /** Route/context qualifier shown in parens when the same key means two
   *  different things across screens (spec §5.4), e.g. "in the inbox". */
  context?: string;
}

export interface TourScreen {
  title: string;
  /** actionIds resolved against the effective bindings at render time,
   *  paired with the label to show (a binding's own description is often
   *  too terse for a guided tour). */
  keys: { actionId: string; label: string; context?: string }[];
}

/**
 * Screen content (spec §5.4). Content is static; key LABELS render from the
 * actual keybindings config via {@link resolveTourScreens} so user overrides
 * display correctly — no hardcoded key names anywhere in this file.
 */
export const TOUR_SCREENS: readonly TourScreen[] = [
  {
    title: 'Inbox & lanes',
    keys: [
      { actionId: 'inbox.lane1', label: 'Jump to a lane' },
      {
        actionId: 'inbox.markLaneRead',
        label: 'Mark lane read',
        context: 'in the inbox',
      },
    ],
  },
  {
    title: 'Navigation',
    keys: [
      { actionId: 'list.down', label: 'Move down' },
      { actionId: 'list.up', label: 'Move up' },
      { actionId: 'list.open', label: 'Open' },
      { actionId: 'list.search', label: 'Search' },
      { actionId: 'list.filter', label: 'Filter' },
    ],
  },
  {
    title: 'Diff review',
    keys: [
      { actionId: 'diff.nextComment', label: 'Next comment' },
      { actionId: 'diff.prevComment', label: 'Previous comment' },
      { actionId: 'diff.toggleStyle', label: 'Split / unified' },
      {
        actionId: 'diff.toggleWhitespace',
        label: 'Hide whitespace-only hunks',
      },
      { actionId: 'diff.markViewed', label: 'Mark file viewed' },
      { actionId: 'diff.sinceLastReview', label: 'Since last review' },
    ],
  },
  {
    title: 'Actions',
    keys: [
      { actionId: 'detail.comment', label: 'Comment' },
      { actionId: 'detail.approve', label: 'Approve (double-tap)' },
      {
        actionId: 'detail.requestChanges',
        label: 'Request changes',
        context: 'on a PR',
      },
      { actionId: 'detail.dismissNits', label: 'Dismiss nits' },
      { actionId: 'detail.dismissBots', label: 'Dismiss bot comments' },
    ],
  },
  {
    title: 'Palette & help',
    keys: [
      { actionId: 'global.command', label: 'Command palette' },
      { actionId: 'global.help', label: 'Keyboard shortcuts' },
    ],
  },
];

/** Resolved screen: key hints carry their formatted label from live bindings. */
export interface ResolvedTourScreen {
  title: string;
  keys: TourKeyHint[];
}

/**
 * Resolve {@link TOUR_SCREENS} against the effective (user-overridden)
 * bindings. Missing actionIds are silently dropped rather than rendering a
 * broken hint — the tour is a guide, not a contract.
 */
export function resolveTourScreens(
  bindings: readonly KeyBinding[],
): ResolvedTourScreen[] {
  const byId = new Map(bindings.map((b) => [b.actionId, b]));
  return TOUR_SCREENS.map((screen) => ({
    title: screen.title,
    keys: screen.keys.flatMap(({ actionId, label, context }) => {
      const binding = byId.get(actionId);
      if (binding === undefined) return [];
      return [
        {
          keys: formatBindingKeys(binding),
          label,
          ...(context !== undefined ? { context } : {}),
        },
      ];
    }),
  }));
}
