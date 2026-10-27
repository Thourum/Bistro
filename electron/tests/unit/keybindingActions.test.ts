// @vitest-environment jsdom

import { errAsync, Fault, okAsync } from '@thourum/fault';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../../src/renderer/store';
import type { UIState } from '../../src/renderer/store/ui';
import { initialUIState } from '../../src/renderer/store/ui';
import { invokeAction, makeCtx, makeStore } from './keybindingTestHelpers';

const mockNavigate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('../../src/renderer/router', () => ({
  router: {
    navigate: mockNavigate,
    state: { location: { pathname: '/prs/pr-test-id' } },
  },
  prDetailRoute: {},
}));

const mockApprovePR = vi.hoisted(() => vi.fn());
vi.mock('../../src/renderer/services/prActions', () => ({
  approvePR: mockApprovePR,
}));

const request = vi.hoisted(() => vi.fn());
vi.mock('../../src/renderer/services/ws/connection', () => ({
  wsClient: { send: vi.fn(), request },
}));

// ---------------------------------------------------------------------------
// Action behavior (direct invocation)
// ---------------------------------------------------------------------------

describe('binding actions', () => {
  it('j moves to the next PR', () => {
    const store = makeStore({ selectedPRIndex: 2 });
    invokeAction('list.down', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(3);
  });

  it('j clamps at the last visible row', () => {
    const store = makeStore({
      selectedPRIndex: 2,
      visiblePRIds: ['a', 'b', 'c'],
    });
    invokeAction('list.down', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(2);
  });

  it('Control+D clamps at the last visible row', () => {
    const store = makeStore({
      selectedPRIndex: 1,
      visiblePRIds: ['a', 'b', 'c'],
    });
    invokeAction('list.pageDown', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(2);
  });

  it('Space toggles the repo group under the cursor', () => {
    const store = makeStore({
      selectedPRIndex: 1,
      visiblePRIds: ['pr-1', 'pr-2', 'pr-3'],
      visiblePRRepoKeys: ['acme/widgets', 'acme/widgets', 'acme/api'],
    });
    invokeAction('list.toggleGroup', makeCtx(store));
    expect(store.state.collapsedRepoKeys).toEqual(['acme/widgets']);
    // Cursor moves to the group's first row (the header once collapsed).
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('Space expands a collapsed group and keeps the cursor on its first row', () => {
    const store = makeStore({
      selectedPRIndex: 0,
      collapsedRepoKeys: ['acme/widgets'],
      visiblePRIds: ['', 'pr-3'],
      visiblePRRepoKeys: ['acme/widgets', 'acme/api'],
    });
    invokeAction('list.toggleGroup', makeCtx(store));
    expect(store.state.collapsedRepoKeys).toEqual([]);
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('Enter on a collapsed group header expands it instead of navigating', () => {
    mockNavigate.mockClear();
    const store = makeStore({
      selectedPRIndex: 0,
      collapsedRepoKeys: ['acme/widgets'],
      visiblePRIds: ['', 'pr-3'],
      visiblePRRepoKeys: ['acme/widgets', 'acme/api'],
    });
    invokeAction('list.open', makeCtx(store));
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(store.state.collapsedRepoKeys).toEqual([]);
  });

  it('k moves to the previous PR and clamps at 0', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    invokeAction('list.up', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('gg jumps to the top', () => {
    const store = makeStore({ selectedPRIndex: 7 });
    invokeAction('list.top', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('Shift+G jumps to the last visible PR (not MAX_SAFE_INTEGER)', () => {
    // With 3 visible PRs, cursor should jump to index 2.
    const store = makeStore({ visiblePRIds: ['a', 'b', 'c'] });
    invokeAction('list.bottom', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(2);
    // With empty list, clamps to 0.
    const emptyStore = makeStore({ visiblePRIds: [] });
    invokeAction('list.bottom', makeCtx(emptyStore));
    expect(emptyStore.state.selectedPRIndex).toBe(0);
  });

  it('Enter navigates to the PR at the cursor position by id', () => {
    mockNavigate.mockClear();
    // With a visible PR at index 0, Enter should call router.navigate.
    const store = makeStore({ visiblePRIds: ['pr-abc'], selectedPRIndex: 0 });
    invokeAction('list.open', makeCtx(store));
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs/$prId', params: { prId: 'pr-abc' } }),
    );
    // When list is empty, Enter is a no-op.
    mockNavigate.mockClear();
    const emptyStore = makeStore({ visiblePRIds: [], selectedPRIndex: 0 });
    invokeAction('list.open', makeCtx(emptyStore));
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('o opens the PR at the cursor position in the browser', () => {
    const openExternal = vi.fn().mockResolvedValue({ ok: true });
    Object.defineProperty(window, 'electron', {
      configurable: true,
      value: { openExternal },
    });
    const store = makeStore({
      selectedPRIndex: 1,
      visiblePRIds: ['pr-1', 'pr-2'],
      visiblePRBrowserUrls: [
        'https://github.com/acme/api/pull/1',
        'https://github.com/acme/api/pull/2',
      ],
    });

    invokeAction('list.openBrowser', makeCtx(store));

    expect(openExternal).toHaveBeenCalledWith(
      'https://github.com/acme/api/pull/2',
    );
    expect(store.state.confirmingAction).toBeNull();
  });

  it('/ opens search overlay', () => {
    const openSearch = vi.fn();
    invokeAction('list.search', makeCtx({}, 'list', { openSearch }));
    expect(openSearch).toHaveBeenCalled();
  });

  it('f toggles the filter overlay', () => {
    const toggleFilter = vi.fn();
    invokeAction('list.filter', makeCtx({}, 'list', { toggleFilter }));
    expect(toggleFilter).toHaveBeenCalled();
  });

  it('digit bindings jump to the Nth PR', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    invokeAction('list.jumpTo5', makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(4);
  });

  it('aa approves the shown PR on the first sequence', () => {
    const store = makeStore({});
    invokeAction('detail.approve', makeCtx(store, 'detail'));
    expect(mockApprovePR).toHaveBeenCalledExactlyOnceWith('pr-test-id');
    expect(store.state.confirmingAction).toBeNull();
  });

  it('Shift+R opens the request-changes composer on detail', () => {
    const store = makeStore({});
    invokeAction('detail.requestChanges', makeCtx(store, 'detail'));
    expect(store.state.confirmingAction).toBe('request_changes');
  });

  it('Space in detail opens the diff view (navigates to /prs/$prId/diff)', async () => {
    const store = makeStore({});
    const { router } = await import('../../src/renderer/router');
    (router.state.location as { pathname: string }).pathname =
      '/prs/pr-test-id';
    invokeAction('detail.viewDiff', makeCtx(store, 'detail'));
    // Router navigates; mode is no longer written.
    expect(router.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs/$prId/diff' }),
    );
  });

  describe('Escape is context-sensitive', () => {
    it('closes overlay when command overlay is active', () => {
      const closeOverlay = vi.fn();
      const ctx = makeCtx({}, 'list', {
        closeOverlay,
        activeOverlay: 'command',
      });
      // activeOverlay must match ctx.activeOverlay too
      Object.assign(ctx, { activeOverlay: 'command' });
      invokeAction('global.escape', ctx);
      expect(closeOverlay).toHaveBeenCalled();
    });

    it('closes search overlay and clears query on Escape', () => {
      const closeSearch = vi.fn();
      const ctx = makeCtx({}, 'list', { closeSearch });
      Object.assign(ctx, { activeOverlay: 'search' });
      invokeAction('global.escape', ctx);
      expect(closeSearch).toHaveBeenCalledWith({ clearQuery: true });
    });

    it('detail route → navigates to /prs (router.navigate called)', async () => {
      const { router } = await import('../../src/renderer/router');
      (router.state.location as { pathname: string }).pathname =
        '/prs/pr-test-id';
      const store = makeStore({});
      invokeAction('global.escape', makeCtx(store, 'detail'));
      expect(router.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ to: '/prs' }),
      );
    });

    it('diff route → navigates back to detail (router.navigate called)', async () => {
      const { router } = await import('../../src/renderer/router');
      (router.state.location as { pathname: string }).pathname =
        '/prs/pr-test-id/diff';
      const store = makeStore({});
      invokeAction('global.escape', makeCtx(store, 'diff'));
      expect(router.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ to: '/prs/$prId' }),
      );
    });

    it('diff route → closes inline reply before navigating back', async () => {
      const { router } = await import('../../src/renderer/router');
      mockNavigate.mockClear();
      const store = makeStore({
        inlineCommentThreadIds: ['T1'],
        activeInlineCommentThreadId: 'T1',
        replyingInlineCommentThreadId: 'T1',
      });
      invokeAction('global.escape', makeCtx(store, 'diff'));
      expect(store.state.replyingInlineCommentThreadId).toBeNull();
      expect(router.navigate).not.toHaveBeenCalled();
    });

    it('settings route → navigates to /prs (router.navigate called)', async () => {
      const { router } = await import('../../src/renderer/router');
      (router.state.location as { pathname: string }).pathname = '/settings';
      const store = makeStore({});
      invokeAction('global.escape', makeCtx(store, 'settings'));
      expect(router.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ to: '/prs' }),
      );
    });

    it('in list mode it cancels confirming actions', () => {
      const store = makeStore({
        confirmingAction: 'comment',
      });
      invokeAction('global.escape', makeCtx(store, 'list'));
      expect(store.state.confirmingAction).toBeNull();
    });
  });

  it('C-g cancels a pending list confirmation', () => {
    const store = makeStore({ confirmingAction: 'comment' });
    invokeAction('global.cancel', makeCtx(store, 'list'));
    expect(store.state.confirmingAction).toBeNull();
  });

  it('the alternate undo action emits the dismissal undo event', () => {
    const undoEvents: string[] = [];
    document.addEventListener(
      'bistro:comments:undo-dismiss',
      (event) => undoEvents.push(event.type),
      { once: true },
    );
    invokeAction('detail.undoDismissAlt', makeCtx({}, 'detail'));
    expect(undoEvents).toEqual(['bistro:comments:undo-dismiss']);
  });

  describe('diff route: j/k always scroll, n/p navigate inline comments', () => {
    it('j always scrolls, even when inline comments exist', () => {
      const viewport = document.createElement('div');
      viewport.setAttribute('data-diff-viewport', '');
      const scrollBy = vi.fn();
      viewport.scrollBy = scrollBy;
      document.body.appendChild(viewport);
      const store = makeStore({ inlineCommentThreadIds: ['T1', 'T2'] });

      invokeAction('detail.scrollDown', makeCtx(store, 'diff'));

      expect(store.state.activeInlineCommentThreadId).toBeNull();
      expect(scrollBy).toHaveBeenCalledWith({ top: 40 });
      viewport.remove();
    });

    it('k always scrolls, even when inline comments exist', () => {
      const viewport = document.createElement('div');
      viewport.setAttribute('data-diff-viewport', '');
      const scrollBy = vi.fn();
      viewport.scrollBy = scrollBy;
      document.body.appendChild(viewport);
      const store = makeStore({ inlineCommentThreadIds: ['T1', 'T2'] });

      invokeAction('detail.scrollUp', makeCtx(store, 'diff'));

      expect(store.state.activeInlineCommentThreadId).toBeNull();
      expect(scrollBy).toHaveBeenCalledWith({ top: -40 });
      viewport.remove();
    });

    it('n (diff.nextComment) selects the first inline comment', () => {
      const store = makeStore({ inlineCommentThreadIds: ['T1', 'T2'] });

      invokeAction('diff.nextComment', makeCtx(store, 'diff'));

      expect(store.state.activeInlineCommentThreadId).toBe('T1');
    });

    it('p (diff.prevComment) selects the previous inline comment', () => {
      const store = makeStore({
        inlineCommentThreadIds: ['T1', 'T2'],
        activeInlineCommentThreadId: 'T2',
      });

      invokeAction('diff.prevComment', makeCtx(store, 'diff'));

      expect(store.state.activeInlineCommentThreadId).toBe('T1');
    });

    it('n/p are no-ops when there are no inline comments', () => {
      const store = makeStore({ inlineCommentThreadIds: [] });

      invokeAction('diff.nextComment', makeCtx(store, 'diff'));

      expect(store.state.activeInlineCommentThreadId).toBeNull();
    });

    it('Enter starts inline reply for the active diff comment', () => {
      const store = makeStore({
        inlineCommentThreadIds: ['T1'],
        activeInlineCommentThreadId: 'T1',
      });

      invokeAction('detail.inlineReply', makeCtx(store, 'diff'));

      expect(store.state.replyingInlineCommentThreadId).toBe('T1');
    });
  });

  describe('filter overlay bindings', () => {
    const cases: Array<[string, UIState['filterState']]> = [
      ['filter.needsAction', 'needs_action'],
      ['filter.open', 'open'],
      ['filter.draft', 'draft'],
      ['filter.all', 'all'],
    ];
    for (const [actionId, expected] of cases) {
      it(`${actionId} sets filterState=${expected} and closes overlay`, () => {
        const closeOverlay = vi.fn();
        const store = makeStore();
        invokeAction(
          actionId,
          makeCtx(store, 'list', {
            closeOverlay,
            activeOverlay: 'filter' as const,
          }),
        );
        expect(store.state.filterState).toBe(expected);
        expect(closeOverlay).toHaveBeenCalled();
      });
    }
  });
});

describe('draft annotation bindings', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    request.mockReset();
  });

  it('n/p reach drafts without putting them in thread reply state', () => {
    const store = makeStore({
      inlineCommentThreadIds: ['T1', 'draft:D1', 'T2'],
      activeInlineCommentThreadId: 'T1',
      replyingInlineCommentThreadId: 'T1',
    });
    invokeAction('diff.nextComment', makeCtx(store, 'diff'));
    expect(store.state.activeInlineCommentThreadId).toBe('draft:D1');
    expect(store.state.replyingInlineCommentThreadId).toBeNull();
    invokeAction('diff.nextComment', makeCtx(store, 'diff'));
    invokeAction('diff.prevComment', makeCtx(store, 'diff'));
    expect(store.state.activeInlineCommentThreadId).toBe('draft:D1');
  });

  it('Enter requests draft editing instead of a thread reply', () => {
    const store = makeStore({
      inlineCommentThreadIds: ['draft:D1'],
      activeInlineCommentThreadId: 'draft:D1',
    });
    const events: string[] = [];
    const recordEdit = (event: Event): void => {
      events.push(event.type);
    };
    document.addEventListener('bistro:diff:edit-draft', recordEdit);
    invokeAction('detail.inlineReply', makeCtx(store, 'diff'));
    document.removeEventListener('bistro:diff:edit-draft', recordEdit);
    expect(events).toEqual(['bistro:diff:edit-draft']);
    expect(store.state.replyingInlineCommentThreadId).toBeNull();
  });

  it('d d deletes only the active draft and reports success', async () => {
    request.mockReturnValue(okAsync({ deleted: 1 }));
    const store = makeStore({ activeInlineCommentThreadId: 'draft:D1' });
    invokeAction('diff.deleteDraft', makeCtx(store, 'diff'));
    await vi.waitFor(() => expect(appStore.state.toasts).toHaveLength(1));
    expect(request).toHaveBeenCalledExactlyOnceWith('DRAFT_DELETE', {
      pr_id: 'pr-test-id',
      draft_ids: ['D1'],
    });
    expect(appStore.state.toasts[0]).toMatchObject({
      type: 'success',
      message: 'Draft deleted',
      dismissible: false,
    });
  });

  it('d d reports failure without claiming the draft was deleted', async () => {
    request.mockReturnValue(errAsync(new Fault('Offline')));
    const store = makeStore({ activeInlineCommentThreadId: 'draft:D1' });
    invokeAction('diff.deleteDraft', makeCtx(store, 'diff'));
    await vi.waitFor(() => expect(appStore.state.toasts).toHaveLength(1));
    expect(appStore.state.toasts[0]).toMatchObject({
      type: 'error',
      dismissible: true,
    });
    expect(appStore.state.toasts[0]?.message).toContain('Draft not deleted:');
    expect(store.state.activeInlineCommentThreadId).toBe('draft:D1');
  });

  it('d d is a no-op on threads, no selection, or a missing PR', async () => {
    invokeAction(
      'diff.deleteDraft',
      makeCtx({ activeInlineCommentThreadId: 'T1' }, 'diff'),
    );
    invokeAction('diff.deleteDraft', makeCtx({}, 'diff'));
    const context = makeCtx(
      { activeInlineCommentThreadId: 'draft:D1' },
      'diff',
    );
    delete context.prId;
    invokeAction('diff.deleteDraft', context);
    await Promise.resolve();
    expect(request).not.toHaveBeenCalled();
    expect(appStore.state.toasts).toEqual([]);
  });
});

describe('line comment bindings', () => {
  it('Enter keeps an already-open composer (and its draftId)', () => {
    const composer = { draftId: 'D1', initialBody: 'existing' };
    const store = makeStore({
      lineComment: {
        prId: 'pr-test-id',
        fileId: 'F1',
        path: 'a.ts',
        headSha: 'sha',
        layout: 'unified',
        lines: [],
        cursor: { side: 'RIGHT', anchor: 1, head: 1 },
        composer,
      },
    });
    invokeAction('lineComment.compose', makeCtx(store, 'diff'));
    expect(store.state.lineComment?.composer).toBe(composer);
  });
});
