import { afterEach, expect, it, vi } from 'vitest';

vi.mock('@bindings/services', () => ({ Shell: { OpenExternal: vi.fn() } }));
vi.mock('@wailsio/runtime', () => ({ CancelError: class extends Error {} }));

vi.mock('./logger', () => ({
  createLogger: () => ({
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  }),
}));

afterEach(() => vi.unstubAllGlobals());

it('ignores the removed Electron benchmark flag without the URL opt-in', async () => {
  vi.stubGlobal('window', { electron: { isKeysBench: true } });
  vi.stubGlobal('location', { search: '' });
  vi.resetModules();
  const { isKeysBench, guardBenchWrite } = await import('./benchSafety');
  expect(isKeysBench()).toBe(false);
  expect(guardBenchWrite('engine-call').isOk()).toBe(true);
});

it('blocks editor, browser and quit IPC only for bench_keys=1', async () => {
  const openExternal = vi.fn();
  const launchEditor = vi.fn();
  const open = vi.fn();
  const dispatchEvent = vi.fn();
  vi.stubGlobal('window', {
    electron: { openExternal, openInEditor: launchEditor },
    open,
  });
  vi.stubGlobal('document', { dispatchEvent });
  vi.stubGlobal('location', { search: '?bench_keys=1' });
  vi.resetModules();
  const { guardBenchWrite, isKeysBench } = await import('./benchSafety');
  const { openInEditor } = await import('../services/editor');
  const { openUrlInBrowser } = await import('./openUrl');

  dispatchEvent.mockClear();

  const browser = await openUrlInBrowser('https://example.com');
  const editor = await openInEditor({
    repoRoot: '/tmp',
    relativePath: 'file.ts',
    line: 1,
  });
  const quit = guardBenchWrite('app:quit');
  for (const response of [browser, editor, quit]) {
    expect(response._unsafeUnwrapErr().tag).toBe('BENCH_WRITE_BLOCKED');
  }
  expect(openExternal).not.toHaveBeenCalled();
  expect(launchEditor).not.toHaveBeenCalled();
  expect(open).not.toHaveBeenCalled();
  expect(dispatchEvent.mock.calls.map(([event]) => event.detail.type)).toEqual([
    'engine-call',
    'engine-call',
    'app:quit',
  ]);

  vi.stubGlobal('location', { search: '?bench_keys=0' });
  expect(isKeysBench()).toBe(true);
  expect(guardBenchWrite('app:quit').isErr()).toBe(true);
  vi.resetModules();
  const ordinary = await import('./benchSafety');
  expect(ordinary.isKeysBench()).toBe(false);
  expect(ordinary.guardBenchWrite('app:quit').isOk()).toBe(true);
});
