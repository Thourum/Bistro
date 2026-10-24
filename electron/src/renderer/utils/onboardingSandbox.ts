export type SandboxPane = 'list' | 'detail' | 'diff';
export type SandboxStage = 'navigate' | 'review' | 'diff';
export type SandboxTaskId =
  | 'moveDown'
  | 'moveUp'
  | 'top'
  | 'bottom'
  | 'open'
  | 'foldBots'
  | 'undoFold'
  | 'comment'
  | 'requestChanges'
  | 'approve'
  | 'viewDiff'
  | 'nextFile'
  | 'nextComment'
  | 'replyThread'
  | 'sinceLastReview'
  | 'back';

export interface SandboxState {
  readonly pane: SandboxPane;
  readonly cursor: number;
  readonly downPresses: number;
  readonly fileIndex: number;
  readonly commentIndex: number;
  readonly approved: boolean;
  readonly botsFolded: boolean;
  readonly composer: 'comment' | 'requestChanges' | null;
  readonly inlineReplyOpen: boolean;
  readonly sinceLastReview: boolean;
  readonly completed: readonly SandboxTaskId[];
}

export interface SandboxTask {
  readonly id: SandboxTaskId;
  readonly stage: SandboxStage;
  readonly label: string;
  /** Checklist shows the first the active style binds. */
  readonly actionIds: readonly string[];
  /** What the real action does, where the fake one can't show it. */
  readonly reason?: string;
}

export const SANDBOX_PR_COUNT = 5;
export const SANDBOX_FILE_COUNT = 2;
export const SANDBOX_COMMENT_COUNT = 2;
const DOWN_PRESSES_TO_COMPLETE = 3;

export const SANDBOX_STAGES: readonly {
  id: SandboxStage;
  label: string;
  pane: SandboxPane;
}[] = [
  { id: 'navigate', label: 'Navigate', pane: 'list' },
  { id: 'review', label: 'Review PR', pane: 'detail' },
  { id: 'diff', label: 'In diff', pane: 'diff' },
];

export const SANDBOX_TASKS: readonly SandboxTask[] = [
  {
    id: 'moveDown',
    stage: 'navigate',
    label: 'Move down 3 times',
    actionIds: ['list.down', 'list.downArrow'],
  },
  {
    id: 'moveUp',
    stage: 'navigate',
    label: 'Move up',
    actionIds: ['list.up', 'list.upArrow'],
  },
  {
    id: 'top',
    stage: 'navigate',
    label: 'Jump to the top',
    actionIds: ['list.top'],
  },
  {
    id: 'bottom',
    stage: 'navigate',
    label: 'Jump to the bottom',
    actionIds: ['list.bottom'],
  },
  {
    id: 'open',
    stage: 'navigate',
    label: 'Open a PR that needs you',
    actionIds: ['list.open'],
    reason:
      'Needs Action includes requested reviews and follow-ups that need your attention.',
  },
  {
    id: 'foldBots',
    stage: 'review',
    label: 'Fold bot comments',
    actionIds: ['detail.dismissBots'],
  },
  {
    id: 'undoFold',
    stage: 'review',
    label: 'Undo the fold',
    actionIds: ['detail.undoDismiss', 'detail.undoDismissAlt'],
  },
  {
    id: 'comment',
    stage: 'review',
    label: 'Leave a general comment',
    actionIds: ['detail.comment'],
  },
  {
    id: 'requestChanges',
    stage: 'review',
    label: 'Request changes',
    actionIds: ['detail.requestChanges'],
    reason:
      'Sends a changes-requested review. Whether it blocks merging depends on branch rules.',
  },
  {
    id: 'approve',
    stage: 'review',
    label: 'Approve instead',
    actionIds: ['detail.approve'],
    reason: 'The real app asks for confirmation before sending a review.',
  },
  {
    id: 'viewDiff',
    stage: 'review',
    label: 'Open the diff',
    actionIds: ['detail.viewDiff'],
  },
  {
    id: 'nextFile',
    stage: 'diff',
    label: 'Next file',
    actionIds: ['diff.nextFile'],
  },
  {
    id: 'nextComment',
    stage: 'diff',
    label: 'Next comment',
    actionIds: ['diff.nextComment'],
  },
  {
    id: 'replyThread',
    stage: 'diff',
    label: 'Reply to the thread',
    actionIds: ['detail.inlineReply'],
  },
  {
    id: 'sinceLastReview',
    stage: 'diff',
    label: 'See changes since last review',
    actionIds: ['diff.sinceLastReview'],
    reason: 'Bistro remembers the commit you last reviewed.',
  },
  {
    id: 'back',
    stage: 'diff',
    label: 'Back to the PR',
    actionIds: ['global.escape', 'global.cancel'],
  },
];

/** Fake-PR fields that reset whenever a PR is opened. */
const FRESH_PR = {
  fileIndex: 0,
  commentIndex: 0,
  approved: false,
  botsFolded: false,
  composer: null,
  inlineReplyOpen: false,
  sinceLastReview: false,
} as const satisfies Partial<SandboxState>;

export const INITIAL_SANDBOX_STATE: SandboxState = {
  pane: 'list',
  cursor: 0,
  downPresses: 0,
  ...FRESH_PR,
  completed: [],
};

const clamp = (value: number, count: number): number =>
  Math.min(Math.max(value, 0), count - 1);

function complete(state: SandboxState, id: SandboxTaskId): SandboxState {
  return state.completed.includes(id)
    ? state
    : { ...state, completed: [...state.completed, id] };
}

function moveDown(state: SandboxState): SandboxState {
  const next = {
    ...state,
    cursor: clamp(state.cursor + 1, SANDBOX_PR_COUNT),
    downPresses: state.downPresses + 1,
  };
  return next.downPresses >= DOWN_PRESSES_TO_COMPLETE
    ? complete(next, 'moveDown')
    : next;
}

const moveUp = (state: SandboxState): SandboxState =>
  complete(
    { ...state, cursor: clamp(state.cursor - 1, SANDBOX_PR_COUNT) },
    'moveUp',
  );

const undoFold = (state: SandboxState): SandboxState =>
  state.botsFolded
    ? complete({ ...state, botsFolded: false }, 'undoFold')
    : state;

const PANE_BACK: Readonly<Record<SandboxPane, SandboxPane | null>> = {
  list: null,
  detail: 'list',
  diff: 'detail',
};

// The checklist's "Back to the PR" counts only diff → detail; detail → list
// still navigates so the user can reopen a PR.
function goBack(state: SandboxState): SandboxState {
  const previous = PANE_BACK[state.pane];
  if (previous === null) return state;
  const next = { ...state, pane: previous };
  return state.pane === 'diff' ? complete(next, 'back') : next;
}

function toggleSinceLastReview(state: SandboxState): SandboxState {
  const next = { ...state, sinceLastReview: !state.sinceLastReview };
  return next.sinceLastReview ? complete(next, 'sinceLastReview') : next;
}

interface Transition {
  /** Pane the action belongs to; null for any pane. */
  readonly pane: SandboxPane | null;
  readonly apply: (state: SandboxState) => SandboxState;
}

const inList = (apply: Transition['apply']): Transition => ({
  pane: 'list',
  apply,
});
const inDetail = (apply: Transition['apply']): Transition => ({
  pane: 'detail',
  apply,
});
const inDiff = (apply: Transition['apply']): Transition => ({
  pane: 'diff',
  apply,
});
const anyPane = (apply: Transition['apply']): Transition => ({
  pane: null,
  apply,
});

const TRANSITIONS: Readonly<Record<string, Transition>> = {
  'list.down': inList(moveDown),
  'list.downArrow': inList(moveDown),
  'list.up': inList(moveUp),
  'list.upArrow': inList(moveUp),
  'list.top': inList((state) => complete({ ...state, cursor: 0 }, 'top')),
  'list.bottom': inList((state) =>
    complete({ ...state, cursor: SANDBOX_PR_COUNT - 1 }, 'bottom'),
  ),
  'list.open': inList((state) =>
    complete({ ...state, ...FRESH_PR, pane: 'detail' }, 'open'),
  ),
  'detail.dismissBots': inDetail((state) =>
    complete({ ...state, botsFolded: true }, 'foldBots'),
  ),
  'detail.undoDismiss': inDetail(undoFold),
  'detail.undoDismissAlt': inDetail(undoFold),
  'detail.comment': inDetail((state) =>
    complete({ ...state, composer: 'comment' }, 'comment'),
  ),
  'detail.requestChanges': inDetail((state) =>
    complete(
      { ...state, composer: 'requestChanges', approved: false },
      'requestChanges',
    ),
  ),
  'detail.approve': inDetail((state) =>
    complete({ ...state, approved: true, composer: null }, 'approve'),
  ),
  'detail.viewDiff': inDetail((state) =>
    complete({ ...state, pane: 'diff' }, 'viewDiff'),
  ),
  'diff.nextFile': inDiff((state) =>
    complete(
      { ...state, fileIndex: clamp(state.fileIndex + 1, SANDBOX_FILE_COUNT) },
      'nextFile',
    ),
  ),
  'diff.prevFile': inDiff((state) => ({
    ...state,
    fileIndex: clamp(state.fileIndex - 1, SANDBOX_FILE_COUNT),
  })),
  'diff.nextComment': inDiff((state) =>
    complete(
      {
        ...state,
        commentIndex: clamp(state.commentIndex + 1, SANDBOX_COMMENT_COUNT),
      },
      'nextComment',
    ),
  ),
  'diff.prevComment': inDiff((state) => ({
    ...state,
    commentIndex: clamp(state.commentIndex - 1, SANDBOX_COMMENT_COUNT),
  })),
  'detail.inlineReply': inDiff((state) =>
    complete({ ...state, inlineReplyOpen: true }, 'replyThread'),
  ),
  'diff.sinceLastReview': inDiff(toggleSinceLastReview),
  'global.escape': anyPane(goBack),
  'global.cancel': anyPane(goBack),
};

/** Actions the onboarding sandbox handles instead of the real app. */
export const SANDBOX_ACTION_IDS: ReadonlySet<string> = new Set(
  Object.keys(TRANSITIONS),
);

/** Applies a fake PR action without changing the input state. */
export function reduceSandbox(
  state: SandboxState,
  actionId: string,
): SandboxState {
  if (!SANDBOX_ACTION_IDS.has(actionId)) return state;
  const transition = TRANSITIONS[actionId];
  if (transition === undefined) return state;
  if (transition.pane !== null && transition.pane !== state.pane) return state;
  return transition.apply(state);
}

export function firstUnfinishedSandboxStage(
  state: SandboxState,
): SandboxStage | null {
  return (
    SANDBOX_TASKS.find((task) => !state.completed.includes(task.id))?.stage ??
    null
  );
}

/** Switches the shown fake pane without completing anything. */
export function selectSandboxPane(
  state: SandboxState,
  pane: SandboxPane,
): SandboxState {
  return state.pane === pane ? state : { ...state, pane };
}
