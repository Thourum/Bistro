import { describe, expect, it } from 'vitest';
import {
  INITIAL_SANDBOX_STATE,
  reduceSandbox,
  SANDBOX_COMMENT_COUNT,
  SANDBOX_FILE_COUNT,
  SANDBOX_PR_COUNT,
  type SandboxState,
} from './onboardingSandbox';

const run = (
  actionIds: readonly string[],
  from: SandboxState = INITIAL_SANDBOX_STATE,
) => actionIds.reduce(reduceSandbox, from);

const pressPastEnd = (count: number, actionId: string) =>
  Array.from({ length: count + 2 }, () => actionId);

describe('reduceSandbox', () => {
  it('ticks moveDown only after three down presses and clamps the cursor', () => {
    const twice = run(['list.down', 'list.downArrow']);
    expect(twice.completed).not.toContain('moveDown');
    expect(run(['list.down', 'list.down', 'list.down']).completed).toContain(
      'moveDown',
    );
    const many = run(pressPastEnd(SANDBOX_PR_COUNT, 'list.down'));
    expect(many.completed).toContain('moveDown');
    expect(many.cursor).toBe(SANDBOX_PR_COUNT - 1);
  });

  it('walks list → detail → diff and back with Escape or C-g, ticking each task once', () => {
    const state = run([
      'list.bottom',
      'list.open',
      'detail.approve',
      'detail.approve',
      'detail.viewDiff',
      'diff.nextFile',
      'diff.nextComment',
      'global.escape',
      'global.cancel',
    ]);
    expect(state.pane).toBe('list');
    expect([...state.completed].sort()).toEqual(
      [
        'approve',
        'back',
        'bottom',
        'nextComment',
        'nextFile',
        'open',
        'viewDiff',
      ].sort(),
    );
  });

  it('ignores Escape on the list pane and open outside it', () => {
    expect(run(['global.escape'])).toBe(INITIAL_SANDBOX_STATE);
    const inDetail = run(['list.open']);
    expect(run(['list.open'], inDetail)).toBe(inDetail);
  });

  it('clamps up aliases and top at the first PR without ticking moveDown', () => {
    const state = run(['list.bottom', 'list.up', 'list.upArrow', 'list.top']);
    expect(state.cursor).toBe(0);
    expect(state.completed).toEqual(['bottom']);
    expect(run(['list.up', 'list.upArrow']).cursor).toBe(0);
  });

  it('clamps file and comment navigation while keeping task completion unique', () => {
    const atEnd = run([
      'list.open',
      'detail.viewDiff',
      ...pressPastEnd(SANDBOX_FILE_COUNT, 'diff.nextFile'),
      ...pressPastEnd(SANDBOX_COMMENT_COUNT, 'diff.nextComment'),
    ]);
    expect(atEnd.fileIndex).toBe(SANDBOX_FILE_COUNT - 1);
    expect(atEnd.commentIndex).toBe(SANDBOX_COMMENT_COUNT - 1);
    expect(atEnd.completed).toEqual([
      'open',
      'viewDiff',
      'nextFile',
      'nextComment',
    ]);
    const atStart = run(
      [
        ...pressPastEnd(SANDBOX_FILE_COUNT, 'diff.prevFile'),
        ...pressPastEnd(SANDBOX_COMMENT_COUNT, 'diff.prevComment'),
      ],
      atEnd,
    );
    expect(atStart.fileIndex).toBe(0);
    expect(atStart.commentIndex).toBe(0);
    expect(atStart.completed).toEqual(atEnd.completed);
  });

  it('resets the fake PR on open without losing checklist progress or mutating its input', () => {
    const reviewed = run([
      'list.open',
      'detail.approve',
      'detail.viewDiff',
      'diff.nextFile',
      'diff.nextComment',
      'global.cancel',
      'global.escape',
    ]);
    const reopened = run(['list.open'], reviewed);
    expect(reviewed.approved).toBe(true);
    expect(reopened).toEqual({
      ...reviewed,
      pane: 'detail',
      approved: false,
      fileIndex: 0,
      commentIndex: 0,
    });
    expect(INITIAL_SANDBOX_STATE.completed).toEqual([]);
  });

  it('ignores unknown actions and viewing a diff outside detail', () => {
    expect(run(['list.mute', 'detail.viewDiff'])).toBe(INITIAL_SANDBOX_STATE);
    expect(run(['toString', 'constructor', '__proto__'])).toBe(
      INITIAL_SANDBOX_STATE,
    );
  });
});
