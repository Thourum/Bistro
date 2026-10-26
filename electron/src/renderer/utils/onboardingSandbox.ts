export type SandboxPane = 'list' | 'detail' | 'diff';
export type SandboxTaskId =
  | 'moveDown'
  | 'bottom'
  | 'open'
  | 'approve'
  | 'viewDiff'
  | 'nextFile'
  | 'nextComment'
  | 'back';

export interface SandboxState {
  readonly pane: SandboxPane;
  readonly cursor: number;
  readonly downPresses: number;
  readonly fileIndex: number;
  readonly commentIndex: number;
  readonly approved: boolean;
  readonly completed: readonly SandboxTaskId[];
}

export interface SandboxTask {
  readonly id: SandboxTaskId;
  readonly label: string;
  /** Checklist shows the first of these that the active style binds. */
  readonly actionIds: readonly string[];
}

export const SANDBOX_PR_COUNT = 5;
export const SANDBOX_FILE_COUNT = 2;
export const SANDBOX_COMMENT_COUNT = 2;
const DOWN_PRESSES_TO_COMPLETE = 3;

export const SANDBOX_TASKS: readonly SandboxTask[] = [
  {
    id: 'moveDown',
    label: 'Move down 3 times',
    actionIds: ['list.down', 'list.downArrow'],
  },
  { id: 'bottom', label: 'Jump to the bottom', actionIds: ['list.bottom'] },
  { id: 'open', label: 'Open a PR', actionIds: ['list.open'] },
  { id: 'approve', label: 'Approve it', actionIds: ['detail.approve'] },
  { id: 'viewDiff', label: 'View the diff', actionIds: ['detail.viewDiff'] },
  { id: 'nextFile', label: 'Next file', actionIds: ['diff.nextFile'] },
  { id: 'nextComment', label: 'Next comment', actionIds: ['diff.nextComment'] },
  {
    id: 'back',
    label: 'Go back',
    actionIds: ['global.escape', 'global.cancel'],
  },
];

export const INITIAL_SANDBOX_STATE: SandboxState = {
  pane: 'list',
  cursor: 0,
  downPresses: 0,
  fileIndex: 0,
  commentIndex: 0,
  approved: false,
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

const moveUp = (state: SandboxState): SandboxState => ({
  ...state,
  cursor: clamp(state.cursor - 1, SANDBOX_PR_COUNT),
});

const PANE_BACK: Readonly<Record<SandboxPane, SandboxPane | null>> = {
  list: null,
  detail: 'list',
  diff: 'detail',
};

function goBack(state: SandboxState): SandboxState {
  const previous = PANE_BACK[state.pane];
  return previous === null
    ? state
    : complete({ ...state, pane: previous }, 'back');
}

const TRANSITIONS: Readonly<
  Record<string, (state: SandboxState) => SandboxState>
> = {
  'list.down': moveDown,
  'list.downArrow': moveDown,
  'list.up': moveUp,
  'list.upArrow': moveUp,
  'list.top': (state) => ({ ...state, cursor: 0 }),
  'list.bottom': (state) =>
    complete({ ...state, cursor: SANDBOX_PR_COUNT - 1 }, 'bottom'),
  'list.open': (state) =>
    state.pane !== 'list'
      ? state
      : complete(
          {
            ...state,
            pane: 'detail',
            fileIndex: 0,
            commentIndex: 0,
            approved: false,
          },
          'open',
        ),
  'detail.approve': (state) =>
    complete({ ...state, approved: true }, 'approve'),
  'detail.viewDiff': (state) =>
    state.pane !== 'detail'
      ? state
      : complete({ ...state, pane: 'diff' }, 'viewDiff'),
  'diff.nextFile': (state) =>
    complete(
      { ...state, fileIndex: clamp(state.fileIndex + 1, SANDBOX_FILE_COUNT) },
      'nextFile',
    ),
  'diff.prevFile': (state) => ({
    ...state,
    fileIndex: clamp(state.fileIndex - 1, SANDBOX_FILE_COUNT),
  }),
  'diff.nextComment': (state) =>
    complete(
      {
        ...state,
        commentIndex: clamp(state.commentIndex + 1, SANDBOX_COMMENT_COUNT),
      },
      'nextComment',
    ),
  'diff.prevComment': (state) => ({
    ...state,
    commentIndex: clamp(state.commentIndex - 1, SANDBOX_COMMENT_COUNT),
  }),
  'global.escape': goBack,
  'global.cancel': goBack,
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
  return transition === undefined ? state : transition(state);
}
