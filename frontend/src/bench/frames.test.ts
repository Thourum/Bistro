import { afterEach, expect, it, vi } from 'vitest';
import { collectDiffFrames } from './frames';

const IDLE_DELTA_MS = 10;
const SCROLL_DELTA_MS = 80;
const RUN_DURATION_MS = 5000;
const SCROLL_STEP_PX = 48;

const restoreClock = (): void => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
};
afterEach(restoreClock);

it('includes the rendering interval of the final scroll in a one-frame run', async () => {
  vi.useFakeTimers();
  const viewport = { scrollTop: 0, clientHeight: 100, scrollHeight: 1000 };
  vi.stubGlobal('document', { querySelector: () => viewport });
  vi.stubGlobal('performance', { now: () => Date.now() });
  vi.stubGlobal('navigator', { userAgent: 'bench-test' });
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) =>
    setTimeout(
      () => callback(Date.now()),
      viewport.scrollTop === 0 ? IDLE_DELTA_MS : SCROLL_DELTA_MS,
    ),
  );

  const collected = collectDiffFrames('github:bench/large-diff:1', {
    frames: 1,
    stepPx: SCROLL_STEP_PX,
  });
  await vi.advanceTimersByTimeAsync(RUN_DURATION_MS);
  const result = await collected;
  expect(result.isOk()).toBe(true);
  if (result.isErr()) return;
  expect(result.value.refreshMs).toBe(IDLE_DELTA_MS);
  expect(result.value.deltas).toEqual([SCROLL_DELTA_MS]);
  expect(result.value.scrolledPx).toBe(SCROLL_STEP_PX);
});
