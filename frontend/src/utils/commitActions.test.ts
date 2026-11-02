// @vitest-environment jsdom
import {
  afterEach,
  assert,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';

vi.mock('../router', () => ({ router: { navigate: vi.fn() } }));

import type { OverlayContextValue } from '../overlays/OverlayContext';
import { router } from '../router';
import { setCommitList } from '../services/commits';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import {
  COMMIT_TOGGLE_VIEWED_EVENT,
  COMMIT_VIEWED_AND_NEXT_EVENT,
} from './commitNavigation';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import { getEffectiveBindings, isActive, type KeyBinding } from './keybindings';

function binding(actionId: string): KeyBinding {
  const found = getEffectiveBindings('vim', {}).find(
    (entry) => entry.actionId === actionId,
  );
  assert(found !== undefined, `Expected binding ${actionId}`);
  return found;
}

describe('commit keybindings', () => {
  const context: KeyActionContext = {
    store: appStore,
    routeBase: 'commits',
    activeOverlay: null,
    overlays: {} as OverlayContextValue,
    location: { pathname: '/prs/p/commits' },
    prId: 'p',
  };
  const commitContext: KeyActionContext = {
    ...context,
    routeBase: 'commit',
    location: { pathname: '/prs/p/commits/b' },
  };

  beforeEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
    vi.mocked(router.navigate).mockClear();
  });
  afterEach(() => appStore.setState(() => ({ ...initialUIState })));

  it('clamps list movement and opens only a selected commit with a PR', () => {
    getBindingAction('commits.down')?.(context);
    expect(appStore.state.selectedCommitIndex).toBe(0);
    getBindingAction('commits.open')?.(context);
    expect(router.navigate).not.toHaveBeenCalled();
    setCommitList(['a', 'b'], 'b');
    getBindingAction('commits.down')?.(context);
    expect(appStore.state.selectedCommitIndex).toBe(1);
    getBindingAction('commits.up')?.(context);
    getBindingAction('commits.up')?.(context);
    expect(appStore.state.selectedCommitIndex).toBe(0);
    const { prId: _prId, ...withoutPR } = context;
    getBindingAction('commits.open')?.(withoutPR);
    expect(router.navigate).not.toHaveBeenCalled();
    getBindingAction('commits.open')?.(context);
    expect(router.navigate).toHaveBeenLastCalledWith({
      to: '/prs/$prId/commits/$sha',
      params: { prId: 'p', sha: 'a' },
      search: { file: '', side: 'right' },
    });
  });

  it('steps by the route SHA, stays at ends, and ignores stale SHAs', () => {
    setCommitList(['a', 'b', 'c'], 'a');
    getBindingAction('commit.next')?.(commitContext);
    expect(router.navigate).toHaveBeenLastCalledWith({
      to: '/prs/$prId/commits/$sha',
      params: { prId: 'p', sha: 'c' },
      search: { file: '', side: 'right' },
    });
    getBindingAction('commit.prev')?.(commitContext);
    expect(router.navigate).toHaveBeenLastCalledWith({
      to: '/prs/$prId/commits/$sha',
      params: { prId: 'p', sha: 'a' },
      search: { file: '', side: 'right' },
    });
    vi.mocked(router.navigate).mockClear();
    for (const [actionId, sha] of [
      ['commit.prev', 'a'],
      ['commit.next', 'c'],
      ['commit.next', 'stale'],
    ]) {
      getBindingAction(actionId ?? '')?.({
        ...commitContext,
        location: { pathname: `/prs/p/commits/${sha}` },
      });
    }
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('preserves commit SHA while changing files and unwinds the escape chain', () => {
    appStore.setState((state) => ({
      ...state,
      diffFileKeys: ['one', 'two'],
      selectedFileIndex: 0,
    }));
    getBindingAction('diff.nextFile')?.(commitContext);
    expect(router.navigate).toHaveBeenLastCalledWith({
      to: '/prs/$prId/commits/$sha',
      params: { prId: 'p', sha: 'b' },
      search: { file: 'two', side: 'right' },
      replace: true,
    });
    getBindingAction('global.escape')?.(commitContext);
    expect(router.navigate).toHaveBeenLastCalledWith({
      to: '/prs/$prId/commits',
      params: { prId: 'p' },
    });
    getBindingAction('global.escape')?.(context);
    expect(router.navigate).toHaveBeenLastCalledWith({
      to: '/prs/$prId',
      params: { prId: 'p' },
    });
    for (const actionId of ['detail.commits', 'diff.commits']) {
      getBindingAction(actionId)?.(context);
      expect(router.navigate).toHaveBeenLastCalledWith({
        to: '/prs/$prId/commits',
        params: { prId: 'p' },
      });
    }
  });

  it('emits viewed intents consumed by the commit pages', () => {
    const toggle = vi.fn();
    const next = vi.fn();
    document.addEventListener(COMMIT_TOGGLE_VIEWED_EVENT, toggle);
    document.addEventListener(COMMIT_VIEWED_AND_NEXT_EVENT, next);
    getBindingAction('commit.toggleViewed')?.(context);
    getBindingAction('commit.viewedAndNext')?.(commitContext);
    document.removeEventListener(COMMIT_TOGGLE_VIEWED_EVENT, toggle);
    document.removeEventListener(COMMIT_VIEWED_AND_NEXT_EVENT, next);
    expect(toggle).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledTimes(1);
  });

  it('allows read-only diff controls but never review or range actions on commits', () => {
    for (const routeBase of ['commits', 'commit'] as const) {
      const activation = { routeBase, activeOverlay: null };
      for (const actionId of [
        'global.escape',
        'global.help',
        'global.command',
        'commit.toggleViewed',
      ])
        expect(isActive(binding(actionId), activation), actionId).toBe(true);
      for (const actionId of [
        'detail.approve',
        'detail.comment',
        'detail.requestChanges',
        'diff.lineComment',
        'diff.deleteDraft',
        'diff.markViewed',
        'diff.sinceLastReview',
        'diff.interdiff',
        'review.toggleResolve',
        'diff.nextComment',
        'detail.inlineReply',
      ])
        expect(isActive(binding(actionId), activation), actionId).toBe(false);
    }
    for (const actionId of [
      'detail.scrollDown',
      'detail.pageDown',
      'diff.nextFile',
      'diff.prevFile',
      'diff.toggleStyle',
      'diff.toggleCollapse',
      'diff.toggleWhitespace',
      'diff.openFileBrowser',
    ])
      expect(
        isActive(binding(actionId), {
          routeBase: 'commit',
          activeOverlay: null,
        }),
        actionId,
      ).toBe(true);
  });
});
