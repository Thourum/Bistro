import { afterEach, describe, expect, it, vi } from 'vitest';
import { createGoReadyWaiter } from './go-spawn';

describe('createGoReadyWaiter', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('keeps waiting for a ready signal after reporting a slow startup', async () => {
    vi.useFakeTimers();
    const onStillStarting = vi.fn();
    const waiter = createGoReadyWaiter(15_000, onStillStarting);

    const readyPort = waiter.wait();
    await vi.advanceTimersByTimeAsync(15_000);

    expect(onStillStarting).toHaveBeenCalledOnce();
    waiter.markReady(64568);

    expect((await readyPort)._unsafeUnwrap()).toBe(64568);
  });

  it('returns err when the process exits before it becomes ready', async () => {
    const waiter = createGoReadyWaiter(15_000, vi.fn());
    const readyPort = waiter.wait();

    waiter.markFailed(new Error('Go process exited prematurely'));

    expect((await readyPort)._unsafeUnwrapErr().message).toBe(
      'Go process exited prematurely',
    );
  });
});
