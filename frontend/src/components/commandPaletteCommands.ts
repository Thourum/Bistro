import { permitsBenchAction } from '../utils/benchSafety';
import { isActive } from '../utils/keybindingActivation';
import type { KeyBinding, RouteBase } from '../utils/keybindings';
import { findActionKeys } from '../utils/legendPresets';

type Icon = string;

export interface CommandItem {
  readonly id: string;
  readonly name: string;
  readonly description?: string;
  readonly icon: Icon;
  /** Action whose effective keys render as the row's shortcut hint. */
  readonly shortcutActionId?: string;
  readonly dev?: boolean;
  /** Only listed on the diff route — DiffPage is the sole consumer of the intent. */
  readonly diffOnly?: boolean;
  /** Dispatched like its key; its keys also render as the shortcut hint. */
  readonly actionId?: string;
  /** Only listed on PR detail and diff routes. */
  readonly prOnly?: boolean;
}

/** Runs both bound and palette-only commands through the same dry-dispatch boundary. */
export function executePaletteCommand(
  command: CommandItem,
  execute: () => void,
): void {
  if (permitsBenchAction(command.actionId ?? `palette.${command.id}`))
    execute();
}

/** Display keys only for an effective binding active on the underlying route. */
export function shortcutFor(
  actionId: string | undefined,
  bindings: readonly KeyBinding[],
  routeBase: RouteBase | null,
): string | undefined {
  if (actionId === undefined) return undefined;
  const routeBindings = bindings.filter((entry) =>
    isActive(entry, { routeBase, activeOverlay: null }),
  );
  return findActionKeys(routeBindings, [actionId]);
}

export const COMMANDS: readonly CommandItem[] = [
  {
    id: 'help',
    name: 'Show keyboard shortcuts',
    icon: 'keyboard',
    shortcutActionId: 'global.help',
  },
  {
    id: 'tour',
    name: ':tour',
    description: 'Show the first-review guide again',
    icon: 'school',
  },
  {
    id: 'settings',
    name: 'Open project settings',
    icon: 'settings',
    shortcutActionId: 'global.settings',
  },
  {
    id: 'inbox',
    name: 'Go to Inbox',
    icon: 'inbox',
    actionId: 'global.inbox',
  },
  {
    id: 'sync',
    name: 'Sync now',
    description: 'Force sync all watched repos',
    icon: 'sync',
  },
  {
    id: 'clear',
    name: 'Clear all filters and search',
    icon: 'filter_alt_off',
  },
  {
    id: 'view-lanes',
    name: ':view lanes',
    description: 'Organize the PR inbox by review lanes',
    icon: 'view_agenda',
  },
  {
    id: 'view-groups',
    name: ':view groups',
    description: 'Organize the PR inbox by repository',
    icon: 'account_tree',
  },
  {
    id: 'open-browser',
    name: ':open in browser',
    description: 'Open the focused PR on GitHub',
    icon: 'open_in_new',
    shortcutActionId: 'list.openBrowser',
  },
  {
    id: 'diff-split',
    name: ':diff split',
    description: 'Show diffs side by side',
    icon: 'vertical_split',
    shortcutActionId: 'diff.toggleStyle',
  },
  {
    id: 'diff-unified',
    name: ':diff unified',
    description: 'Show diffs in a single column',
    icon: 'view_stream',
    shortcutActionId: 'diff.toggleStyle',
  },
  {
    id: 'open-editor',
    name: 'Open file in editor',
    description:
      "Opens in $EDITOR at the selected comment's line when one is selected",
    icon: 'edit_document',
    diffOnly: true,
  },
  {
    id: 'review-approve',
    name: 'Approve',
    description: 'Includes pending drafts',
    icon: 'check_circle',
    actionId: 'detail.approve',
    prOnly: true,
  },
  {
    id: 'review-request-changes',
    name: 'Request changes',
    description: 'Includes pending drafts',
    icon: 'rate_review',
    actionId: 'detail.requestChanges',
    prOnly: true,
  },
  {
    id: 'review-comment',
    name: 'Comment on PR',
    icon: 'chat',
    actionId: 'detail.comment',
    prOnly: true,
  },
  {
    id: 'review-submit',
    name: 'Submit review',
    description: 'Send pending drafts as one comment review',
    icon: 'send',
    actionId: 'review.submit',
    prOnly: true,
  },
  {
    id: 'review-discard-drafts',
    name: 'Discard drafts',
    icon: 'delete_sweep',
    actionId: 'review.discardDrafts',
    prOnly: true,
  },
  {
    id: 'review-line-comment',
    name: 'Start line comment',
    icon: 'add_comment',
    actionId: 'diff.lineComment',
    diffOnly: true,
  },
  {
    id: 'review-dismiss-nits',
    name: 'Dismiss nits and info',
    icon: 'filter_list_off',
    actionId: 'detail.dismissNits',
    prOnly: true,
  },
  {
    id: 'review-dismiss-bots',
    name: 'Dismiss bot comments',
    icon: 'smart_toy',
    actionId: 'detail.dismissBots',
    prOnly: true,
  },
  {
    id: 'review-undo-dismiss',
    name: 'Undo dismissal',
    icon: 'undo',
    actionId: 'detail.undoDismiss',
    prOnly: true,
  },
  {
    id: 'review-toggle-resolve',
    name: 'Resolve / unresolve thread',
    icon: 'task_alt',
    actionId: 'review.toggleResolve',
    prOnly: true,
  },
  {
    id: 'repo-picker',
    name: 'Open repository picker',
    description: 'Discover and watch repositories',
    icon: 'source_environment',
  },
  {
    id: 'add-repo-by-url',
    name: 'Add repository by URL',
    description: 'Clone or link a repository from a Git URL',
    icon: 'add_link',
  },
  { id: 'expertise-enable', name: 'Enable expertise', icon: 'psychology' },
  { id: 'expertise-disable', name: 'Disable expertise', icon: 'psychology' },
  { id: 'focus-list', name: 'Focus PR list', icon: 'list_alt' },
  { id: 'focus-detail', name: 'Focus PR detail', icon: 'article' },
  { id: 'about', name: 'About BISTRO', icon: 'info' },
  {
    id: 'reset-store',
    name: 'Reset store to initial state',
    icon: 'restart_alt',
    description: 'DEV ONLY',
    dev: true,
  },
];
