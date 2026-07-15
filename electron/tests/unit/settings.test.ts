import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import type { Logger } from 'electron-log';
import { describe, expect, it, vi } from 'vitest';

const electronStoreMock = vi.hoisted(() => {
  class MockElectronStore<T extends Record<string, unknown>> {
    readonly path: string;
    private storeValue: T;
    private readonly listeners: Array<(next: Readonly<T>) => void> = [];

    constructor(options: { cwd: string; name: string; defaults: T }) {
      this.path = join(options.cwd, `${options.name}.json`);
      this.storeValue = existsSync(this.path)
        ? JSON.parse(readFileSync(this.path, 'utf-8'))
        : options.defaults;
      mkdirSync(dirname(this.path), { recursive: true });
      writeFileSync(this.path, `${JSON.stringify(this.storeValue, null, 2)}\n`);
    }

    get store(): T {
      return this.storeValue;
    }

    set store(value: T) {
      this.storeValue = value;
      mkdirSync(dirname(this.path), { recursive: true });
      writeFileSync(this.path, `${JSON.stringify(value, null, 2)}\n`);
      for (const listener of this.listeners) {
        listener(value);
      }
    }

    onDidAnyChange(listener: (next: Readonly<T>) => void): () => void {
      this.listeners.push(listener);
      return () => undefined;
    }

    _closeWatcher(): void {
      // Test double has no watcher to close.
    }
  }

  return { default: MockElectronStore };
});

const electronMock = vi.hoisted(() => ({
  BrowserWindow: { getAllWindows: vi.fn(() => []) },
  app: { getPath: vi.fn(() => ''), getVersion: vi.fn(() => '0.0.0') },
  ipcMain: { on: vi.fn() },
  shell: { openPath: vi.fn() },
}));

vi.mock('electron', () => electronMock);
vi.mock('electron-store', () => electronStoreMock);

import { SettingsService } from '../../src/main/settings';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

const createTestLogger = (): Logger =>
  ({
    info: () => undefined,
    warn: () => undefined,
    error: () => undefined,
  }) as unknown as Logger;

const createSettingsPath = async () =>
  join(await mkdtemp(join(tmpdir(), 'bistro-settings-')), 'settings.json');

const writeSettingsFile = async (filePath: string, value: unknown) => {
  await mkdir(dirname(filePath), { recursive: true });
  await writeFile(filePath, `${JSON.stringify(value, null, 2)}\n`, 'utf-8');
};

describe('settings defaults', () => {
  it('defaults the repositories dev folder path to null', () => {
    expect(DEFAULT_SETTINGS.repositories.devFolderPath).toBeNull();
  });
});

describe('SettingsService', () => {
  it('round-trips a complete settings object with repositories unchanged', async () => {
    const settingsPath = await createSettingsPath();
    const service = new SettingsService(createTestLogger(), settingsPath);
    await service.load();
    const completeSettings = {
      ...DEFAULT_SETTINGS,
      repositories: {
        devFolderPath: '/Users/alice/dev',
      },
    };

    const settings = await service.set(completeSettings);
    service.stop();

    const loadedService = new SettingsService(createTestLogger(), settingsPath);
    const loaded = await loadedService.load();
    loadedService.stop();

    expect(settings.repositories).toEqual(completeSettings.repositories);
    expect(loaded.repositories).toEqual(completeSettings.repositories);
    expect(loaded).toEqual(completeSettings);
  });

  it('discards unknown top-level and repositories fields when loading settings', async () => {
    const settingsPath = await createSettingsPath();
    await writeSettingsFile(settingsPath, {
      ...DEFAULT_SETTINGS,
      unknownTopLevel: 'discard me',
      repositories: {
        devFolderPath: '/Users/alice/dev',
        unknownRepositoryField: 'discard me too',
      },
    });
    const service = new SettingsService(createTestLogger(), settingsPath);

    const settings = await service.load();
    service.stop();

    expect(settings).toEqual({
      ...DEFAULT_SETTINGS,
      repositories: {
        devFolderPath: '/Users/alice/dev',
      },
    });
    expect(settings).not.toHaveProperty('unknownTopLevel');
    expect(settings.repositories).not.toHaveProperty('unknownRepositoryField');
  });

  it('discards unknown top-level and repositories fields when merging settings patches', async () => {
    const settingsPath = await createSettingsPath();
    const service = new SettingsService(createTestLogger(), settingsPath);
    await service.load();

    const settings = await service.set({
      unknownTopLevel: 'discard me',
      repositories: {
        devFolderPath: '/Users/alice/dev',
        unknownRepositoryField: 'discard me too',
      },
    } as unknown as Partial<typeof DEFAULT_SETTINGS>);
    service.stop();

    const persisted = JSON.parse(await readFile(settingsPath, 'utf-8'));
    const expected = {
      ...DEFAULT_SETTINGS,
      repositories: {
        devFolderPath: '/Users/alice/dev',
      },
    };
    expect(settings).toEqual(expected);
    expect(persisted).toEqual(expected);
    expect(settings).not.toHaveProperty('unknownTopLevel');
    expect(settings.repositories).not.toHaveProperty('unknownRepositoryField');
  });

  it('rejects a non-string devFolderPath from malformed JSON on disk', async () => {
    const settingsPath = await createSettingsPath();
    await writeSettingsFile(settingsPath, {
      ...DEFAULT_SETTINGS,
      repositories: { devFolderPath: 42 },
    });
    const service = new SettingsService(createTestLogger(), settingsPath);

    const settings = await service.load();
    service.stop();

    expect(settings.repositories.devFolderPath).toBeNull();
  });

  it('rejects a non-string devFolderPath patch and keeps the current value', async () => {
    const settingsPath = await createSettingsPath();
    const service = new SettingsService(createTestLogger(), settingsPath);
    await service.load();
    await service.set({ repositories: { devFolderPath: '/Users/alice/dev' } });

    const settings = await service.set({
      repositories: { devFolderPath: { nested: true } },
    } as unknown as Partial<typeof DEFAULT_SETTINGS>);
    service.stop();

    expect(settings.repositories.devFolderPath).toBe('/Users/alice/dev');
  });
});
