// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import { shortcutFor } from '../../src/components/commandPaletteCommands';
import { appStore } from '../../src/store';
import { settingsStore } from '../../src/store/settings';
import { initialUIState } from '../../src/store/ui';
import { cancellable } from '../../src/test/cancellable';
import { getEffectiveBindings, isActive } from '../../src/utils/keybindings';
import { findActionKeys } from '../../src/utils/legendPresets';
import { isLineCommentCancel } from '../../src/utils/lineCommentActions';
import { invokeAction, makeCtx, makeStore } from './keybindingTestHelpers';

const mockNavigate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('../../src/router', () => ({
  router: { navigate: mockNavigate },
}));
const { Delete, SubmitAction } = vi.hoisted(() => ({
  Delete: vi.fn(),
  SubmitAction: vi.fn(),
}));
vi.mock('@bindings/services', () => ({
  Drafts: { Delete },
  Threads: { SubmitAction },
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

const initialSettings = settingsStore.state;

afterEach(() => {
  settingsStore.setState(() => initialSettings);
  appStore.setState(() => initialUIState);
  Delete.mockReset();
  SubmitAction.mockReset();
  mockNavigate.mockClear();
});

describe('review shortcuts', () => {
  it('bubbles only effective cancel keys from the line composer', () => {
    const defaults = getEffectiveBindings('emacs', {});
    const cancel = new KeyboardEvent('keydown', { key: 'g', ctrlKey: true });
    expect(isLineCommentCancel(cancel, defaults)).toBe(true);
    expect(
      isLineCommentCancel(
        new KeyboardEvent('keydown', { key: 'Escape' }),
        defaults,
      ),
    ).toBe(true);
    expect(
      isLineCommentCancel(new KeyboardEvent('keydown', { key: 'g' }), defaults),
    ).toBe(false);
    const remapped = getEffectiveBindings('emacs', {
      'global.cancel': ['Control+Q'],
      'global.escape': [],
    });
    expect(isLineCommentCancel(cancel, remapped)).toBe(false);
    expect(
      isLineCommentCancel(
        new KeyboardEvent('keydown', { key: 'Escape' }),
        remapped,
      ),
    ).toBe(false);
    expect(
      isLineCommentCancel(
        new KeyboardEvent('keydown', { key: 'q', ctrlKey: true }),
        remapped,
      ),
    ).toBe(true);
    expect(
      isLineCommentCancel(
        new KeyboardEvent('keydown', { key: 'Enter', metaKey: true }),
        remapped,
      ),
    ).toBe(false);
  });

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

  it('uses effective comment navigation in resolve warnings and omits unbound keys', () => {
    settingsStore.setState((settings) => ({
      ...settings,
      navigation: { ...settings.navigation, style: 'emacs' },
      keybindings: {
        'diff.nextComment': ['Control+Q'],
        'diff.prevComment': [],
      },
    }));
    invokeAction('review.toggleResolve', makeCtx({}, 'diff'));
    const keys = findActionKeys(
      getEffectiveBindings('emacs', settingsStore.state.keybindings),
      ['diff.nextComment'],
    );
    expect(appStore.state.toasts.at(-1)?.message).toBe(
      `Select a review thread in the diff (${keys}) to resolve it`,
    );
    settingsStore.setState((settings) => ({
      ...settings,
      keybindings: { 'diff.nextComment': [], 'diff.prevComment': [] },
    }));
    invokeAction('review.toggleResolve', makeCtx({}, 'diff'));
    expect(appStore.state.toasts.at(-1)?.message).toBe(
      'Select a review thread in the diff to resolve it',
    );
  });

  it('submit warns without drafts, then sends one COMMENT review and settles feedback', async () => {
    invokeAction('review.submit', makeCtx({}, 'diff'));
    expect(appStore.state.toasts[0]?.type).toBe('warning');
    expect(Delete).not.toHaveBeenCalled();
    expect(SubmitAction).not.toHaveBeenCalled();
    SubmitAction.mockReturnValue(
      cancellable(
        Promise.resolve({ pr_id: 'pr-test-id', action: 'submit_review' }),
      ),
    );
    invokeAction('review.submit', makeCtx({ reviewDrafts: [draft] }, 'diff'));
    await vi.waitFor(() =>
      expect(appStore.state.actionResult?.success).toBe(true),
    );
    expect(SubmitAction).toHaveBeenCalledExactlyOnceWith({
      pr_id: 'pr-test-id',
      action: 'submit_review',
      review_event: 'COMMENT',
      body: '',
      thread_id: '',
      comment_id: '',
      in_reply_to: '',
      line: 0,
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

  it.each([
    {
      reviewDrafts: [draft],
      draftIds: ['D1'],
      message: 'Discarded 1 draft',
    },
    {
      reviewDrafts: [draft, { ...draft, id: 'D2' }],
      draftIds: ['D1', 'D2'],
      message: 'Discarded 2 drafts',
    },
  ])(
    'discard sends all draft ids and reports "$message"',
    async ({ reviewDrafts, draftIds, message }) => {
      Delete.mockReturnValue(
        cancellable(
          Promise.resolve({ pr_id: 'pr-test-id', deleted: draftIds.length }),
        ),
      );
      invokeAction('review.discardDrafts', makeCtx({ reviewDrafts }, 'diff'));
      await vi.waitFor(() => expect(appStore.state.toasts).toHaveLength(1));
      expect(Delete).toHaveBeenCalledExactlyOnceWith({
        pr_id: 'pr-test-id',
        draft_ids: draftIds,
      });
      expect(appStore.state.toasts[0]).toMatchObject({
        type: 'success',
        message,
        dismissible: false,
      });
    },
  );

  it('discard failure reports an error and keeps the drafts intact', async () => {
    Delete.mockReturnValue(cancellable(Promise.reject(new Error('Offline'))));
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
    expect(Delete).not.toHaveBeenCalled();
    expect(SubmitAction).not.toHaveBeenCalled();
    expect(appStore.state.toasts).toEqual([]);
  });
});
