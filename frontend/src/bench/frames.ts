import { err, Fault, ok, type Result, ResultAsync } from '@thourum/fault';
import { summarizeFrames } from './stats';

const VIEWPORT_SELECTOR = '[data-diff-viewport]';
const VIEWPORT_STABLE_MS = 1000;
const VIEWPORT_TIMEOUT_MS = 30_000;
const VIEWPORT_POLL_MS = 50;
const IDLE_FRAMES = 120;

interface ScrollOptions {
  readonly frames: number;
  readonly stepPx: number;
}

interface ScrollFrames {
  readonly deltas: number[];
  readonly scrolledPx: number;
}

interface FramesEvent extends ScrollFrames {
  readonly event: 'frames';
  readonly prId: string;
  readonly refreshMs: number;
  readonly scrollHeight: number;
  readonly viewportHeight: number;
  readonly userAgent: string;
}

function benchFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause : String(cause))
    .withTag('BENCH_ERROR')
    .withCause(cause);
}

function waitForViewport(): ResultAsync<HTMLElement, Fault> {
  return new ResultAsync(
    new Promise<Result<HTMLElement, Fault>>((resolve) => {
      const deadline = performance.now() + VIEWPORT_TIMEOUT_MS;
      let stableSince = performance.now();
      let previousHeight = 0;
      let previousViewport: HTMLElement | null = null;
      const poll = (): void => {
        const now = performance.now();
        const viewport = document.querySelector<HTMLElement>(VIEWPORT_SELECTOR);
        if (now >= deadline) {
          resolve(
            err(
              new Fault(
                'Diff viewport did not stabilize within 30 seconds',
              ).withTag('TIMEOUT_ERROR'),
            ),
          );
          return;
        }
        if (
          viewport === null ||
          viewport.scrollHeight <= viewport.clientHeight
        ) {
          previousViewport = null;
          stableSince = now;
        } else if (
          viewport !== previousViewport ||
          viewport.scrollHeight !== previousHeight
        ) {
          previousViewport = viewport;
          previousHeight = viewport.scrollHeight;
          stableSince = now;
        } else if (now - stableSince >= VIEWPORT_STABLE_MS) {
          resolve(ok(viewport));
          return;
        }
        setTimeout(poll, VIEWPORT_POLL_MS);
      };
      poll();
    }),
  );
}

function nextFrame(): Promise<number> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => resolve(performance.now()));
  });
}

function measureRefresh(): ResultAsync<number, Fault> {
  return ResultAsync.fromThrowable(async () => {
    const deltas: number[] = [];
    let previous = await nextFrame();
    for (let frame = 0; frame < IDLE_FRAMES; frame++) {
      const now = await nextFrame();
      deltas.push(now - previous);
      previous = now;
    }
    const sorted = deltas.sort((left, right) => left - right);
    const middle = Math.floor(sorted.length / 2);
    return ((sorted[middle - 1] ?? 0) + (sorted[middle] ?? 0)) / 2;
  }, benchFault)();
}

function measureScroll(
  viewport: HTMLElement,
  options: ScrollOptions,
): ResultAsync<ScrollFrames, Fault> {
  return ResultAsync.fromThrowable(async () => {
    const deltas: number[] = [];
    const startTop = viewport.scrollTop;
    let previous = await nextFrame();
    for (let frame = 0; frame < options.frames; frame++) {
      if (viewport.scrollTop + viewport.clientHeight >= viewport.scrollHeight)
        break;
      viewport.scrollTop += options.stepPx;
      const now = await nextFrame();
      deltas.push(now - previous);
      previous = now;
    }
    return { deltas, scrolledPx: viewport.scrollTop - startTop };
  }, benchFault)();
}

/** Waits for layout, calibrates the display, then records the protocol's raw rAF deltas. */
export function collectDiffFrames(
  prId: string,
  options: ScrollOptions,
): ResultAsync<FramesEvent, Fault> {
  return waitForViewport().andThen((viewport) =>
    measureRefresh().andThen((refreshMs) =>
      measureScroll(viewport, options).map(({ deltas, scrolledPx }) => ({
        event: 'frames' as const,
        prId,
        refreshMs,
        deltas,
        scrolledPx,
        scrollHeight: viewport.scrollHeight,
        viewportHeight: viewport.clientHeight,
        userAgent: navigator.userAgent,
        ...summarizeFrames(deltas, refreshMs),
      })),
    ),
  );
}
