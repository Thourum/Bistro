import { okAsync } from '@thourum/fault';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { approvePR, postComment, requestChanges } from './prActions';
import { wsClient } from './ws/connection';

const send = vi.hoisted(() => vi.fn(() => undefined));
vi.mock('./ws/connection', () => ({
  wsClient: { send, request: vi.fn() },
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

  it('records dispatched approval even when transport is unavailable', () => {
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
    send.mockClear();
  });

  it.each([
    ['approve', () => approvePR('pr-1')],
    ['request_changes', () => requestChanges('pr-1', 'Please revise')],
  ] as const)(
    'clears pending %s when the WebSocket cannot send',
    (type, dispatch) => {
      dispatch();

      expect(appStore.state.pendingAction).toBeNull();
      expect(appStore.state.actionResult).toMatchObject({
        type,
        prId: 'pr-1',
        success: false,
        message: 'WebSocket not connected',
      });
    },
  );
});
