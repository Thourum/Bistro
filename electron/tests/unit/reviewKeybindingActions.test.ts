// @vitest-environment jsdom
import { errAsync, Fault, okAsync } from '@thourum/fault';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { shortcutFor } from '../../src/renderer/components/commandPaletteCommands';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';
import {
  getEffectiveBindings,
  isActive,
} from '../../src/renderer/utils/keybindings';
import { invokeAction, makeCtx, makeStore } from './keybindingTestHelpers';

const mockNavigate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('../../src/renderer/router', () => ({
  router: { navigate: mockNavigate },
}));
const request = vi.hoisted(() => vi.fn());
vi.mock('../../src/renderer/services/ws/connection', () => ({
  wsClient: { send: vi.fn(), request },
}));

const draft = {
  id: 'D1',
  pr_id: 'pr-test-id',
  path: 'a.ts',
  side: 'RIGHT' as const,
  start_line: 0,
  line: 1,
  body: 'Check this',
  commit_sha: 'sha',
  created_at: '',
};

afterEach(() => {
  appStore.setState(() => initialUIState);
  request.mockReset();
  mockNavigate.mockClear();
});

describe('review shortcuts', () => {
  it('on diff hides the PR comment chip and shows the line-comment chip', () => {
    const bindings = getEffectiveBindings('vim', {});
    expect(shortcutFor('detail.comment', bindings, 'diff')).toBeUndefined();
    expect(shortcutFor('diff.lineComment', bindings, 'diff')).toBe('C');
    expect(shortcutFor('detail.comment', bindings, 'detail')).toBe('C');
  });

  it('shows effective sequence and remapped chips; unbound actions have none', () => {
    const defaults = getEffectiveBindings('vim', {});
    expect(shortcutFor('detail.approve', defaults, 'detail')).toBe('A A');
    expect(shortcutFor('review.toggleResolve', defaults, 'diff')).toBe('X');
    expect(shortcutFor('review.submit', defaults, 'detail')).toBeUndefined();
    const remapped = getEffectiveBindings('vim', {
      'detail.approve': ['q', 'r'],
      'review.toggleResolve': [],
    });
    expect(shortcutFor('detail.approve', remapped, 'detail')).toBe('Q R');
    expect(
      shortcutFor('review.toggleResolve', remapped, 'diff'),
    ).toBeUndefined();
    const resolve = defaults.find(
      (binding) => binding.actionId === 'review.toggleResolve',
    );
    expect(resolve).toBeDefined();
    if (resolve === undefined) return;
    expect(
      isActive(resolve, { routeBase: 'detail', activeOverlay: null }),
    ).toBe(true);
    expect(isActive(resolve, { routeBase: 'diff', activeOverlay: null })).toBe(
      true,
    );
    expect(isActive(resolve, { routeBase: 'list', activeOverlay: null })).toBe(
      false,
    );
    expect(
      isActive(resolve, { routeBase: 'diff', activeOverlay: 'command' }),
    ).toBe(false);
  });
});

describe('review commands', () => {
  it('x warns for detail, no selection, and drafts; emits only for a diff thread', () => {
    const events: string[] = [];
    const record = (event: Event): void => {
      events.push(event.type);
    };
    document.addEventListener('bistro:diff:toggle-resolve', record);
    for (const context of [
      makeCtx({}, 'detail'),
      makeCtx({}, 'diff'),
      makeCtx({ activeInlineCommentThreadId: 'draft:D1' }, 'diff'),
    ]) {
      invokeAction('review.toggleResolve', context);
    }
    expect(events).toEqual([]);
    expect(appStore.state.toasts.map((toast) => toast.type)).toEqual([
      'warning',
      'warning',
      'warning',
    ]);
    invokeAction(
      'review.toggleResolve',
      makeCtx({ activeInlineCommentThreadId: 'T1' }, 'diff'),
    );
    document.removeEventListener('bistro:diff:toggle-resolve', record);
    expect(events).toEqual(['bistro:diff:toggle-resolve']);
  });

  it('submit warns without drafts, then sends one COMMENT review and settles feedback', async () => {
    invokeAction('review.submit', makeCtx({}, 'diff'));
    expect(appStore.state.toasts[0]?.type).toBe('warning');
    expect(request).not.toHaveBeenCalled();
    request.mockReturnValue(okAsync({ success: true }));
    invokeAction('review.submit', makeCtx({ reviewDrafts: [draft] }, 'diff'));
    await vi.waitFor(() =>
      expect(appStore.state.actionResult?.success).toBe(true),
    );
    expect(request).toHaveBeenCalledExactlyOnceWith('SUBMIT_ACTION', {
      pr_id: 'pr-test-id',
      action: 'submit_review',
      review_event: 'COMMENT',
      body: '',
    });
  });

  it.each(['detail.comment', 'detail.requestChanges'])(
    '%s navigates from diff to its detail composer',
    (action) => {
      const store = makeStore();
      invokeAction(action, makeCtx(store, 'diff'));
      expect(store.state.confirmingAction).toBe(
        action === 'detail.comment' ? 'comment' : 'request_changes',
      );
      expect(mockNavigate).toHaveBeenCalledExactlyOnceWith({
        to: '/prs/$prId',
        params: { prId: 'pr-test-id' },
      });
    },
  );

  it('discard sends all draft ids and reports success', async () => {
    request.mockReturnValue(okAsync({ deleted: 2 }));
    invokeAction(
      'review.discardDrafts',
      makeCtx(
        {
          reviewDrafts: [draft, { ...draft, id: 'D2' }],
        },
        'diff',
      ),
    );
    await vi.waitFor(() => expect(appStore.state.toasts).toHaveLength(1));
    expect(request).toHaveBeenCalledExactlyOnceWith('DRAFT_DELETE', {
      pr_id: 'pr-test-id',
      draft_ids: ['D1', 'D2'],
    });
    expect(appStore.state.toasts[0]).toMatchObject({
      type: 'success',
      message: 'Discarded 2 drafts',
      dismissible: false,
    });
  });

  it('discard failure reports an error and keeps the drafts intact', async () => {
    request.mockReturnValue(errAsync(new Fault('Offline')));
    const store = makeStore({ reviewDrafts: [draft] });
    invokeAction('review.discardDrafts', makeCtx(store, 'detail'));
    await vi.waitFor(() => expect(appStore.state.toasts).toHaveLength(1));
    expect(appStore.state.toasts[0]?.type).toBe('error');
    expect(store.state.reviewDrafts).toEqual([draft]);
  });

  it('submit and discard do nothing without a PR; discard does nothing without drafts', () => {
    const context = makeCtx({ reviewDrafts: [draft] }, 'detail');
    delete context.prId;
    invokeAction('review.submit', context);
    invokeAction('review.discardDrafts', context);
    invokeAction('review.discardDrafts', makeCtx({}, 'detail'));
    expect(request).not.toHaveBeenCalled();
    expect(appStore.state.toasts).toEqual([]);
  });
});
