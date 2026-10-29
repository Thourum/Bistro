import type { CancellablePromise } from '@wailsio/runtime';
import { type Mock, vi } from 'vitest';

/** Adds a controllable cancellation spy to a native promise in binding tests. */
export function cancellable<T>(
  result: Promise<T>,
): CancellablePromise<T> & { cancel: Mock } {
  // Native promises lack Wails' chained cancellation methods; tests only use cancel.
  return Object.assign(result, { cancel: vi.fn() }) as CancellablePromise<T> & {
    cancel: Mock;
  };
}
