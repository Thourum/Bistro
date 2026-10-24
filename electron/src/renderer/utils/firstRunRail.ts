/** The first-run rail's three live steps, completable in any order. */
export type FirstRunStep = 'open' | 'range' | 'review';

export interface FirstRunRailState {
  readonly completed: readonly FirstRunStep[];
}

export const INITIAL_FIRST_RUN_RAIL: FirstRunRailState = { completed: [] };

// Step 2 is "Open the diff": the first review has no baseline, so plain
// viewDiff counts as well as the since-last-review range.
const STEP_BY_ACTION: Readonly<Record<string, FirstRunStep>> = {
  'list.open': 'open',
  'detail.viewDiff': 'range',
  'diff.sinceLastReview': 'range',
  'detail.approve': 'review',
  'detail.comment': 'review',
};

/** Marks the step a dispatched action completes; unrelated or repeated actions are no-ops. */
export function reduceFirstRunRail(
  state: FirstRunRailState,
  actionId: string,
): FirstRunRailState {
  const step = STEP_BY_ACTION[actionId];
  if (step === undefined || state.completed.includes(step)) return state;
  return { completed: [...state.completed, step] };
}
