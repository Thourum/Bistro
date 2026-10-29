import { errAsync, okAsync, ServiceError } from '@thourum/fault';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { approvePR, postComment, requestChanges } from './prActions';
import { wsClient } from './ws/connection';

const request = vi.hoisted(() => vi.fn());
vi.mock('./ws/connection', () => ({
  wsClient: { send: vi.fn(), request },
}));

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
    request.mockReturnValueOnce(okAsync({ success: true }));
    approvePR('pr-1');
    expect(appStore.state.firstRunRail.completed).toEqual(['review']);
  });

  it('records only a non-empty PR comment', () => {
    postComment('pr-1', ' \n\t ');
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    vi.mocked(wsClient.request).mockReturnValueOnce(okAsync({ success: true }));
    postComment('pr-1', 'Looks good');
    expect(appStore.state.firstRunRail.completed).toEqual(['review']);
  });
});

describe('PR actions', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    request.mockReset();
  });

  it.each([
    ['approve', () => approvePR('pr-1')],
    ['request_changes', () => requestChanges('pr-1', 'Please revise')],
  ] as const)(
    'clears pending %s when the WebSocket cannot send',
    async (type, dispatch) => {
      request.mockReturnValue(
        errAsync(ServiceError('CONNECTION_ERROR', 'WebSocket not connected')),
      );
      dispatch();
      await vi.waitFor(() => expect(appStore.state.pendingAction).toBeNull());

      expect(appStore.state.actionResult).toMatchObject({
        type,
        prId: 'pr-1',
        success: false,
        message: 'WebSocket not connected',
      });
    },
  );

  it('submits approve through SUBMIT_ACTION so the engine can absorb drafts', () => {
    request.mockReturnValue(errAsync(ServiceError('CONNECTION_ERROR', 'x')));
    approvePR('pr-1');
    expect(request).toHaveBeenCalledWith('SUBMIT_ACTION', {
      pr_id: 'pr-1',
      action: 'approve',
    });
  });
});
