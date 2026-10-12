import { mkdir, mkdtemp, readFile, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Logger } from 'electron-log';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { setupIPC } from './ipc';
import type { SettingsService } from './settings';

const handlers = vi.hoisted(
  () => new Map<string, (...args: unknown[]) => unknown>(),
);
vi.mock('electron', () => ({
  ipcMain: {
    handle: (channel: string, handler: (...args: unknown[]) => unknown) =>
      handlers.set(channel, handler),
  },
  dialog: {},
}));

describe('fs:read-patch', () => {
  const originalCacheHome = process.env.XDG_CACHE_HOME;

  afterEach(() => {
    if (originalCacheHome === undefined) delete process.env.XDG_CACHE_HOME;
    else process.env.XDG_CACHE_HOME = originalCacheHome;
    handlers.clear();
  });

  it('reads cached patches but rejects an in-cache symlink escaping the cache', async () => {
    const cacheHome = await mkdtemp(join(tmpdir(), 'bistro-patch-cache-'));
    process.env.XDG_CACHE_HOME = cacheHome;
    const cacheDir = join(cacheHome, 'bistro');
    await mkdir(cacheDir);
    const patchPath = join(cacheDir, 'allowed.patch');
    await writeFile(patchPath, 'patch contents');
    const outsidePath = join(cacheHome, 'private.txt');
    await writeFile(outsidePath, 'private contents');
    const linkPath = join(cacheDir, 'leak.patch');
    await symlink(outsidePath, linkPath);

    setupIPC(
      '',
      { info: vi.fn(), warn: vi.fn() } as unknown as Logger,
      () => null,
      {} as SettingsService,
      () => null,
    );
    const readPatch = handlers.get('fs:read-patch');
    expect(readPatch).toBeDefined();
    expect(await readPatch?.(null, { path: patchPath })).toEqual({
      ok: 'patch contents',
    });
    expect(await readPatch?.(null, { path: linkPath })).toEqual({
      err: 'Access denied: path outside allowed directory',
    });
    expect(await readFile(outsidePath, 'utf8')).toBe('private contents');
  });
});
