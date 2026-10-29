import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { cancellable } from '../test/cancellable';
import { approvePR, postComment, requestChanges } from './prActions';

const SubmitAction = vi.hoisted(() => vi.fn());
vi.mock('@bindings/services', () => ({ Threads: { SubmitAction } }));

describe('first-run milestones from PR services', () => {
  const previousAppearance = settingsStore.state.appearance;

  beforeEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
    settingsStore.setState((state) => ({
      ...state,
      appearance: {
        ...previousAppearance,
        onboardingComplete: true,
        tourComplete: false,
      },
    }));
  });

  afterEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
    settingsStore.setState((state) => ({
      ...state,
      appearance: previousAppearance,
    }));
    vi.clearAllMocks();
  });

  it('records a dispatched approval', () => {
    SubmitAction.mockReturnValueOnce(
      cancellable(Promise.resolve({ pr_id: 'pr-1', action: 'approve' })),
    );
    approvePR('pr-1');
    expect(appStore.state.firstRunRail.completed).toEqual(['review']);
  });

  it('records only a non-empty PR comment', () => {
    postComment('pr-1', ' \n\t ');
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    SubmitAction.mockReturnValueOnce(
      cancellable(Promise.resolve({ pr_id: 'pr-1', action: 'post_comment' })),
    );
    postComment('pr-1', 'Looks good');
    expect(appStore.state.firstRunRail.completed).toEqual(['review']);
  });
});

describe('PR actions', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    SubmitAction.mockReset();
  });

  it.each([
    ['approve', () => approvePR('pr-1')],
    ['request_changes', () => requestChanges('pr-1', 'Please revise')],
  ] as const)(
    'clears pending %s when the binding rejects',
    async (type, dispatch) => {
      SubmitAction.mockReturnValue(
        cancellable(Promise.reject(new Error('Binding unavailable'))),
      );
      dispatch();
      await vi.waitFor(() => expect(appStore.state.pendingAction).toBeNull());

      expect(appStore.state.actionResult).toMatchObject({
        type,
        prId: 'pr-1',
        success: false,
        message: 'Binding unavailable',
      });
    },
  );

  it('submits approval and settles the engine acknowledgement', async () => {
    SubmitAction.mockReturnValue(
      cancellable(Promise.resolve({ pr_id: 'pr-1', action: 'approve' })),
    );
    approvePR('pr-1');
    await vi.waitFor(() => expect(appStore.state.pendingAction).toBeNull());
    expect(SubmitAction).toHaveBeenCalledWith(
      expect.objectContaining({ pr_id: 'pr-1', action: 'approve' }),
    );
    expect(appStore.state.actionResult).toMatchObject({
      success: true,
      message: 'Approved',
    });
  });

  it('surfaces the Go action_failed message through action feedback', async () => {
    SubmitAction.mockReturnValue(
      cancellable(
        Promise.reject(
          new Error('runtime rejection', {
            cause: {
              code: 'internal',
              reason: 'action_failed',
              message: 'boom',
            },
          }),
        ),
      ),
    );
    approvePR('pr-1');
    await vi.waitFor(() => expect(appStore.state.pendingAction).toBeNull());
    expect(appStore.state.actionResult).toEqual({
      type: 'approve',
      prId: 'pr-1',
      success: false,
      message: 'boom',
    });
  });
});
