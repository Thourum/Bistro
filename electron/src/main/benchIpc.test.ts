import { afterEach, expect, it, vi } from 'vitest';

const handlers = vi.hoisted(
  () => new Map<string, (...args: unknown[]) => unknown>(),
);
vi.mock('electron', () => ({
  ipcMain: {
    handle: (channel: string, handler: (...args: unknown[]) => unknown) =>
      handlers.set(channel, handler),
  },
}));
vi.mock('electron-log', () => ({ default: { warn: vi.fn() } }));
afterEach(() => {
  handlers.clear();
  vi.restoreAllMocks();
});

it('refuses every external main IPC boundary before invoking its handler', async () => {
  const originalArgv = process.argv;
  const query =
    'bench_keys=1&bench_collector=http://localhost:9000&bench_keys_pr=github:bench/large-diff:1';
  process.argv = [`--bench-query=${encodeURIComponent(query)}`];
  vi.resetModules();
  const { registerMainIpc } = await import('./benchIpc');
  process.argv = originalArgv;
  const externalLaunch = vi.fn();
  for (const channel of [
    'app:quit',
    'open-external',
    'open-in-editor',
    'engine:open-log-folder',
    'engine:report-crash',
    'settings:open-file',
    'dialog:pick-folder',
  ]) {
    registerMainIpc(channel, externalLaunch);
    expect(await handlers.get(channel)?.({}, 'untrusted')).toEqual({
      err: `Keys benchmark blocked ${channel}`,
    });
  }
  expect(externalLaunch).not.toHaveBeenCalled();
  registerMainIpc('engine:get-ws-port', () => ({ ok: 1234 }));
  expect(handlers.get('engine:get-ws-port')?.({})).toEqual({ ok: 1234 });
});
