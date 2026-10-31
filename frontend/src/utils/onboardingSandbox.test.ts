import { describe, expect, it } from 'vitest';
import {
  firstUnfinishedSandboxStage,
  INITIAL_SANDBOX_STATE,
  reduceSandbox,
  SANDBOX_COMMENT_COUNT,
  SANDBOX_FILE_COUNT,
  SANDBOX_PR_COUNT,
  SANDBOX_TASKS,
  type SandboxState,
  selectSandboxPane,
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

  it('tracks 5 navigate, 6 review, 5 diff tasks through actual action IDs', () => {
    const state = run([
      'list.down',
      'list.downArrow',
      'list.down',
      'list.upArrow',
      'list.top',
      'list.bottom',
      'list.open',
      'detail.dismissBots',
      'detail.undoDismissAlt',
      'detail.comment',
      'detail.requestChanges',
      'detail.approve',
      'detail.viewDiff',
      'diff.nextFile',
      'diff.nextComment',
      'detail.inlineReply',
      'diff.sinceLastReview',
      'global.cancel',
    ]);
    const inStage = (stage: string) =>
      SANDBOX_TASKS.filter((task) => task.stage === stage);
    expect(SANDBOX_TASKS).toHaveLength(16);
    expect(inStage('navigate')).toHaveLength(5);
    expect(inStage('review')).toHaveLength(6);
    expect(inStage('diff')).toHaveLength(5);
    expect(new Set(state.completed).size).toBe(16);
    expect(state.approved).toBe(true);
    expect(state.botsFolded).toBe(false);
    expect(state.sinceLastReview).toBe(true);
    expect(state.pane).toBe('detail');
    expect(firstUnfinishedSandboxStage(state)).toBeNull();
    expect(INITIAL_SANDBOX_STATE.completed).toEqual([]);
  });

  it('completes nothing on pane tabs and ignores wrong-pane actions', () => {
    const diff = selectSandboxPane(INITIAL_SANDBOX_STATE, 'diff');
    expect(diff.completed).toEqual([]);
    expect(reduceSandbox(INITIAL_SANDBOX_STATE, 'diff.nextFile')).toBe(
      INITIAL_SANDBOX_STATE,
    );
    expect(reduceSandbox(diff, 'detail.approve')).toBe(diff);
    expect(
      firstUnfinishedSandboxStage(reduceSandbox(diff, 'diff.nextFile')),
    ).toBe('navigate');
  });

  it('ticks undoFold only after a fold', () => {
    const detail = selectSandboxPane(INITIAL_SANDBOX_STATE, 'detail');
    expect(run(['detail.undoDismiss'], detail).completed).not.toContain(
      'undoFold',
    );
    const undone = run(['detail.dismissBots', 'detail.undoDismiss'], detail);
    expect(undone.botsFolded).toBe(false);
    expect(undone.completed).toEqual(['foldBots', 'undoFold']);
  });

  it('ticks back only on diff → detail, not on PR → list', () => {
    const detail = selectSandboxPane(INITIAL_SANDBOX_STATE, 'detail');
    const toList = run(['global.escape'], detail);
    expect(toList.pane).toBe('list');
    expect(toList.completed).not.toContain('back');
    const diff = selectSandboxPane(detail, 'diff');
    expect(run(['global.escape'], diff).completed).toContain('back');
  });

  it('moves stage at each boundary and keeps review decisions exclusive', () => {
    const navigated = run([
      'list.down',
      'list.down',
      'list.down',
      'list.up',
      'list.top',
      'list.bottom',
    ]);
    expect(firstUnfinishedSandboxStage(navigated)).toBe('navigate');
    const opened = run(['list.open'], navigated);
    expect(firstUnfinishedSandboxStage(opened)).toBe('review');
    const approved = run(['detail.approve'], opened);
    const requested = run(['detail.requestChanges'], approved);
    expect(requested.approved).toBe(false);
    expect(requested.composer).toBe('requestChanges');
    const reapproved = run(['detail.approve'], requested);
    expect(reapproved.approved).toBe(true);
    expect(reapproved.composer).toBeNull();
    const reviewed = run(
      ['detail.dismissBots', 'detail.undoDismiss', 'detail.comment'],
      reapproved,
    );
    expect(firstUnfinishedSandboxStage(reviewed)).toBe('review');
    expect(
      firstUnfinishedSandboxStage(run(['detail.viewDiff'], reviewed)),
    ).toBe('diff');
  });

  it('ticks sinceLastReview on entering the range, not on leaving it', () => {
    const inDiff = selectSandboxPane(INITIAL_SANDBOX_STATE, 'diff');
    const toggledTwice = run(
      ['diff.sinceLastReview', 'diff.sinceLastReview'],
      inDiff,
    );
    expect(toggledTwice.sinceLastReview).toBe(false);
    expect(toggledTwice.completed).toEqual(['sinceLastReview']);
  });

  it('ignores Escape on the list pane and open outside it', () => {
    expect(run(['global.escape'])).toBe(INITIAL_SANDBOX_STATE);
    const inDetail = run(['list.open']);
    expect(run(['list.open'], inDetail)).toBe(inDetail);
  });

  it('clamps up aliases and top at the first PR without ticking moveDown', () => {
    const state = run(['list.bottom', 'list.up', 'list.upArrow', 'list.top']);
    expect(state.cursor).toBe(0);
    expect(state.completed).toEqual(['bottom', 'moveUp', 'top']);
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
      'detail.dismissBots',
      'detail.approve',
      'detail.comment',
      'detail.viewDiff',
      'diff.nextFile',
      'diff.nextComment',
      'detail.inlineReply',
      'diff.sinceLastReview',
      'global.cancel',
      'global.escape',
    ]);
    const reopened = run(['list.open'], reviewed);
    expect(reviewed.approved).toBe(true);
    expect(reopened).toEqual({
      ...reviewed,
      pane: 'detail',
      approved: false,
      botsFolded: false,
      composer: null,
      inlineReplyOpen: false,
      sinceLastReview: false,
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
