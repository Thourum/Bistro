// @vitest-environment jsdom
import { Threads } from '@bindings/services';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterAll, afterEach, expect, it, vi } from 'vitest';
import { addToast } from '../components/ToastContainer';
import { cancellable } from '../test/cancellable';
import { useThreadActions } from './useThreadActions';

vi.mock('@bindings/services', () => ({ Threads: { SubmitAction: vi.fn() } }));
vi.mock('../components/ToastContainer', () => ({ addToast: vi.fn() }));

// @wailsio/runtime's drag.js starts a 50 ms window.setInterval on import whose
// first tick calls window.clearInterval; if jsdom tears down before it fires,
// the tick throws "window is not defined". Outlive one tick.
const WAILS_DRAG_POLL_MS = 60;

afterAll(
  () => new Promise((resolve) => setTimeout(resolve, WAILS_DRAG_POLL_MS)),
);

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

it('uses the queued reason, not message text, and clears pending threads', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.useFakeTimers();
  const root = createRoot(document.createElement('div'));
  let threadActions: ReturnType<typeof useThreadActions> | undefined;
  function Host(): null {
    threadActions = useThreadActions('pr-1');
    return null;
  }
  await act(async () => root.render(<Host />));

  for (const failure of [
    {
      code: 'rate_limited',
      reason: 'action_queued',
      message: 'Saved for retry',
      expectedType: 'warning',
      expectedMessage: 'Thread resolve queued for retry',
    },
    {
      code: 'internal',
      reason: 'action_failed',
      message: 'action queued for later replay',
      expectedType: 'error',
      expectedMessage: 'action queued for later replay',
    },
  ]) {
    vi.mocked(Threads.SubmitAction).mockReturnValueOnce(
      cancellable(
        Promise.reject(new Error('runtime failure', { cause: failure })),
      ),
    );
    await act(async () => threadActions?.resolveThread('thread-1'));
    expect(addToast).toHaveBeenLastCalledWith({
      type: failure.expectedType,
      message: failure.expectedMessage,
      dismissible: true,
    });
    expect(threadActions?.isThreadPending('thread-1')).toBe(false);
  }
  await act(async () => root.unmount());
});
