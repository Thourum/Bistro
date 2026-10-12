// @vitest-environment jsdom

import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { useActionDispatch } from './useActionDispatch';

const send = vi.hoisted(() => vi.fn(() => undefined));
vi.mock('../services/ws/connection', () => ({
  wsClient: { send, request: vi.fn() },
}));

function CaptureActions({
  onReady,
}: {
  onReady: (actions: ReturnType<typeof useActionDispatch>) => void;
}) {
  onReady(useActionDispatch('pr-1'));
  return null;
}

describe('useActionDispatch', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    send.mockClear();
  });

  it.each([
    [
      'approve',
      (actions: ReturnType<typeof useActionDispatch>) =>
        actions.dispatchApprove(),
    ],
    [
      'request_changes',
      (actions: ReturnType<typeof useActionDispatch>) =>
        actions.dispatchRequestChanges('Please revise'),
    ],
  ] as const)('clears pending %s when the WebSocket cannot send', async (type, dispatch) => {
    const element = document.createElement('div');
    const root = createRoot(element);
    let actions: ReturnType<typeof useActionDispatch> | null = null;
    await act(async () => {
      root.render(
        <CaptureActions
          onReady={(value) => {
            actions = value;
          }}
        />,
      );
    });

    if (actions === null) throw new Error('Action hook did not render');
    const renderedActions: ReturnType<typeof useActionDispatch> = actions;
    act(() => dispatch(renderedActions));

    expect(appStore.state.pendingAction).toBeNull();
    expect(appStore.state.actionResult).toMatchObject({
      type,
      prId: 'pr-1',
      success: false,
      message: 'WebSocket not connected',
    });
    await act(async () => root.unmount());
  });
});
