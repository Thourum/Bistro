import { err, Fault, ok, type Result, ResultAsync } from '@thourum/fault';
import { expect, it, vi } from 'vitest';
import { createMenuActionReporter } from './menu';

it('serializes availability and sends only the newest pending snapshot after failure', async () => {
  let finishFirst: (result: Result<void, Fault>) => void = () => undefined;
  const firstFinished = new Promise<Result<void, Fault>>((resolve) => {
    finishFirst = resolve;
  });
  let finishNewest: (result: Result<void, Fault>) => void = () => undefined;
  const newestFinished = new Promise<Result<void, Fault>>((resolve) => {
    finishNewest = resolve;
  });
  const snapshots: string[][] = [];
  const failure = new Fault('report failed');
  const onError = vi.fn();
  let inFlight = 0;
  let maximumInFlight = 0;
  const reportLatest = createMenuActionReporter({
    send: (enabled) => {
      snapshots.push([...enabled]);
      inFlight += 1;
      maximumInFlight = Math.max(maximumInFlight, inFlight);
      const finished = snapshots.length === 1 ? firstFinished : newestFinished;
      return new ResultAsync(
        finished.then((result) => {
          inFlight -= 1;
          return result;
        }),
      );
    },
    onError,
  });
  reportLatest(['global.settings']);
  reportLatest([]);
  reportLatest(['global.inbox']);
  expect(snapshots).toEqual([['global.settings']]);
  expect(inFlight).toBe(1);

  finishFirst(err(failure));
  await vi.waitFor(() =>
    expect(snapshots).toEqual([['global.settings'], ['global.inbox']]),
  );
  expect(onError).toHaveBeenCalledExactlyOnceWith(failure);
  expect(maximumInFlight).toBe(1);
  finishNewest(ok(undefined));
  await vi.waitFor(() => expect(inFlight).toBe(0));
  expect(snapshots.at(-1)).toEqual(['global.inbox']);
  expect(onError).toHaveBeenCalledOnce();
});
