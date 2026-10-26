import { mkdtemp, open, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { okAsync } from '@thourum/fault';
import { afterEach, expect, it, vi } from 'vitest';
import type { EngineStatus } from '../types/engine';
import { registerEngineIPC } from './engineIpc';
import { getStderrTail } from './go-spawn';

const ipc = vi.hoisted(() => ({
  handlers: new Map<string, (event: unknown, input?: unknown) => unknown>(),
}));
vi.mock('electron', () => ({
  ipcMain: {
    handle: (
      channel: string,
      handler: (event: unknown, input?: unknown) => unknown,
    ) => ipc.handlers.set(channel, handler),
  },
  app: { getVersion: () => 'test' },
  shell: {},
}));
vi.mock('./go-spawn', () => ({ getStderrTail: vi.fn(() => []) }));
vi.mock('./updater', () => ({ BISTRO_REPO: 'itterno/bistro' }));
vi.mock('node:fs/promises', async (original) => {
  const actual = await original<typeof import('node:fs/promises')>();
  return {
    ...actual,
    open: vi.fn(actual.open),
    readFile: vi.fn(actual.readFile),
  };
});
const status: EngineStatus = {
  state: 'crashed',
  startedAt: 0,
  isSpawned: true,
  code: 1,
  signal: null,
  error: null,
};
const temporaryDirectories: string[] = [];
async function createLog(text: string): Promise<string> {
  const directory = await mkdtemp(join(tmpdir(), 'bistro-log-tail-'));
  temporaryDirectories.push(directory);
  const logFile = join(directory, 'engine.log');
  await writeFile(logFile, text);
  registerEngineIPC({
    logFile,
    getStatus: () => status,
    restart: () => okAsync(undefined),
  });
  return logFile;
}
async function requestTail(lines: number): Promise<unknown> {
  return ipc.handlers.get('engine:log-tail')?.(null, { lines });
}
afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true })),
  );
  ipc.handlers.clear();
  vi.clearAllMocks();
  vi.mocked(getStderrTail).mockReturnValue([]);
});

it('includes captured fatal stderr when the engine log already has normal output', async () => {
  await createLog('sync started\nsync finished\n');
  vi.mocked(getStderrTail).mockReturnValue([
    'panic: fatal engine failure',
    'goroutine stack',
  ]);
  expect(await requestTail(3)).toEqual({
    ok: ['sync finished', 'panic: fatal engine failure', 'goroutine stack'],
  });
});

it('reads only a bounded suffix of a large log and omits the cut first line', async () => {
  const historicalBytes = 1024 * 1024;
  const file = await createLog(
    `${'x'.repeat(historicalBytes)}\nfirst tail\r\nsecond tail\nlast tail`,
  );
  const fileHandle = await open(file, 'r');
  const read = vi.spyOn(fileHandle, 'read');
  const close = vi.spyOn(fileHandle, 'close');
  vi.mocked(open).mockResolvedValueOnce(fileHandle);
  expect(await requestTail(60)).toEqual({
    ok: ['first tail', 'second tail', 'last tail'],
  });
  expect(readFile).not.toHaveBeenCalled();
  expect(read).toHaveBeenCalledOnce();
  const buffer: unknown = read.mock.calls[0]?.[0];
  expect(
    Buffer.isBuffer(buffer) ? buffer.byteLength : historicalBytes,
  ).toBeLessThan(historicalBytes);
  expect(close).toHaveBeenCalledOnce();
});
