import { afterEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { approvePR, requestChanges } from './prActions';

const send = vi.hoisted(() => vi.fn(() => undefined));
vi.mock('./ws/connection', () => ({
  wsClient: { send, request: vi.fn() },
}));

describe('PR actions', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    send.mockClear();
  });

  it.each([
    ['approve', () => approvePR('pr-1')],
    ['request_changes', () => requestChanges('pr-1', 'Please revise')],
  ] as const)('clears pending %s when the WebSocket cannot send', (type, dispatch) => {
    dispatch();

    expect(appStore.state.pendingAction).toBeNull();
    expect(appStore.state.actionResult).toMatchObject({
      type,
      prId: 'pr-1',
      success: false,
      message: 'WebSocket not connected',
    });
  });
});
