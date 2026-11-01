import { CancelError, CancellablePromise } from '@wailsio/runtime';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cancellable } from '../../test/cancellable';
import { call, DEFAULT_TIMEOUT_MS, faultReason } from './call';

const TEN_MINUTES_MS = 10 * 60 * 1_000;

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe('call', () => {
  it('maps the service code and reason from RuntimeError.cause', async () => {
    const failure = new Error('runtime failure', {
      cause: { code: 'not_found', message: 'gone', reason: 'pr_missing' },
    });
    const fault = (
      await call(() => cancellable(Promise.reject(failure)))
    )._unsafeUnwrapErr();
    expect(fault.tag).toBe('NOT_FOUND');
    expect(fault.message).toBe('gone');
    expect(faultReason(fault)).toBe('pr_missing');
  });

  it('maps non-conforming causes to INTERNAL_ERROR', async () => {
    const failure = new Error('unexpected', {
      cause: { code: 'unknown', message: 'bad' },
    });
    const fault = (
      await call(() => cancellable(Promise.reject(failure)))
    )._unsafeUnwrapErr();
    expect(fault.tag).toBe('INTERNAL_ERROR');
    expect(faultReason(fault)).toBeNull();
  });

  it('maps CancelError to CANCELLED', async () => {
    const fault = (
      await call(() => cancellable(Promise.reject(new CancelError())))
    )._unsafeUnwrapErr();
    expect(fault.tag).toBe('CANCELLED');
  });

  it('cancels once at the default deadline and returns TIMEOUT_ERROR', async () => {
    vi.useFakeTimers();
    let rejectOnCancel: (error: CancelError) => void = () => undefined;
    const promise = cancellable(
      new Promise<never>((_resolve, reject) => {
        rejectOnCancel = reject;
      }),
    );
    promise.cancel.mockImplementation(() => rejectOnCancel(new CancelError()));
    const response = call(() => promise);
    await vi.advanceTimersByTimeAsync(DEFAULT_TIMEOUT_MS);
    expect(promise.cancel).toHaveBeenCalledTimes(1);
    expect((await response)._unsafeUnwrapErr().tag).toBe('TIMEOUT_ERROR');
  });

  it('also times out a real Wails CancellablePromise', async () => {
    vi.useFakeTimers();
    const promise = new CancellablePromise<never>(() => {});
    const cancel = vi.spyOn(promise, 'cancel');
    const response = call(() => promise);
    await vi.advanceTimersByTimeAsync(DEFAULT_TIMEOUT_MS);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect((await response)._unsafeUnwrapErr().tag).toBe('TIMEOUT_ERROR');
  });

  it('disables timeout for null and passes the promise to onStart', async () => {
    vi.useFakeTimers();
    const promise = cancellable(
      new Promise<string>((resolve) => {
        setTimeout(() => resolve('complete'), TEN_MINUTES_MS);
      }),
    );
    const onStart = vi.fn();
    const response = call(() => promise, { timeoutMs: null, onStart });
    await vi.advanceTimersByTimeAsync(TEN_MINUTES_MS);
    expect(promise.cancel).not.toHaveBeenCalled();
    expect(onStart).toHaveBeenCalledWith(promise);
    expect((await response)._unsafeUnwrap()).toBe('complete');
  });

  it('contains startup and onStart exceptions as service faults', async () => {
    expect(
      (await call(() => JSON.parse('invalid')))._unsafeUnwrapErr().tag,
    ).toBe('INTERNAL_ERROR');
    const promise = new CancellablePromise<never>(() => {});
    const cancel = vi.spyOn(promise, 'cancel');
    const response = call(() => promise, {
      onStart: () => JSON.parse('invalid'),
    });
    expect((await response)._unsafeUnwrapErr().tag).toBe('INTERNAL_ERROR');
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it('clears the deadline when the binding completes', async () => {
    vi.useFakeTimers();
    const promise = cancellable(Promise.resolve('complete'));
    expect((await call(() => promise))._unsafeUnwrap()).toBe('complete');
    await vi.advanceTimersByTimeAsync(DEFAULT_TIMEOUT_MS);
    expect(promise.cancel).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe('callWrite', () => {
  it('refuses to start the binding in a keys benchmark', async () => {
    vi.resetModules();
    vi.stubGlobal('location', { search: '?bench_keys=1' });
    const { callWrite } = await import('./call');
    const start = vi.fn(() => cancellable(Promise.resolve('write')));
    const fault = (await callWrite(start))._unsafeUnwrapErr();
    expect(start).not.toHaveBeenCalled();
    expect(fault.tag).toBe('BENCH_WRITE_BLOCKED');
  });
});
