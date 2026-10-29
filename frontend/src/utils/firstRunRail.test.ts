import { expect, it } from 'vitest';
import { INITIAL_FIRST_RUN_RAIL, reduceFirstRunRail } from './firstRunRail';

it('completes each live action once, even out of order', () => {
  const actions = [
    'detail.comment',
    'detail.approve',
    'diff.sinceLastReview',
    'list.open',
    'list.open',
  ];
  const state = actions.reduce(reduceFirstRunRail, INITIAL_FIRST_RUN_RAIL);
  expect(state.completed).toEqual(['review', 'range', 'open']);
  expect(INITIAL_FIRST_RUN_RAIL.completed).toEqual([]);
  expect(reduceFirstRunRail(state, 'detail.viewDiff')).toBe(state);
  expect(reduceFirstRunRail(state, 'detail.requestChanges')).toBe(state);
  expect(reduceFirstRunRail(state, 'detail.inlineReply')).toBe(state);
});

it('completes the diff step from a plain diff view', () => {
  const state = reduceFirstRunRail(INITIAL_FIRST_RUN_RAIL, 'detail.viewDiff');
  expect(state.completed).toEqual(['range']);
});
