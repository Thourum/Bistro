import { mkdir } from 'node:fs/promises';
import { basename, dirname } from 'node:path';
import { app, BrowserWindow, shell } from 'electron';
import type { Logger } from 'electron-log';
import ElectronStore from 'electron-store';
import type Conf from 'conf';
import { fromPromise, fromThrowable } from 'neverthrow';
import { merge } from 'ts-deepmerge';
import {
  type AppearanceSettings,
  type AppSettings,
  type ClassificationSettings,
  DEFAULT_SETTINGS,
  type KeybindingOverrides,
  type PRListSettings,
  type PrivacySettings,
  type RepositoriesSettings,
  SETTINGS_SCHEMA_VERSION,
  type SyncSettings,
} from '../types/settings';

/** Match prior behaviour: nested objects merge; arrays are replaced wholesale. */
function mergeKnownFields<T extends object>(base: T, patch: unknown): T {
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) {
    return base;
  }
  const patchRecord = patch as Record<string, unknown>;
  return Object.fromEntries(
    Object.entries(base).map(([key, baseValue]) => {
      const patchValue = patchRecord[key];
      return [key, patchValue === undefined ? baseValue : patchValue];
    }),
  ) as T;
}

/** devFolderPath must be string | null; anything else keeps the base value. */
function normaliseRepositories(
  base: RepositoriesSettings,
  merged: RepositoriesSettings,
): RepositoriesSettings {
  const { devFolderPath } = merged;
  if (typeof devFolderPath === 'string' || devFolderPath === null) {
    return merged;
  }
  return { ...merged, devFolderPath: base.devFolderPath };
}

function mergeSettings(base: AppSettings, patch: unknown): AppSettings {
  const knownPatch = mergeKnownFields(base, patch);
  const merged = merge.withOptions(
    { mergeArrays: false, allowUndefinedOverrides: false },
    base,
    knownPatch,
  ) as unknown as AppSettings;
  return {
    $schema: merged.$schema,
    appearance: mergeKnownFields<AppearanceSettings>(
      base.appearance,
      merged.appearance,
    ),
    sync: mergeKnownFields<SyncSettings>(base.sync, merged.sync),
    prList: mergeKnownFields<PRListSettings>(base.prList, merged.prList),
    privacy: mergeKnownFields<PrivacySettings>(base.privacy, merged.privacy),
    repositories: normaliseRepositories(
      base.repositories,
      mergeKnownFields<RepositoriesSettings>(
        base.repositories,
        merged.repositories,
      ),
    ),
    classification: mergeKnownFields<ClassificationSettings>(
      base.classification,
      merged.classification,
    ),
    keybindings: merged.keybindings as KeybindingOverrides,
  };
}

/**
 * Normalise arbitrary JSON read from disk into a valid AppSettings.
 * Missing fields are filled from defaults; unknown fields are discarded.
 * Exported for tests: new settings sections must merge in for old files
 * without a schema bump.
 */
export function normalise(raw: unknown): AppSettings {
  const merged = mergeSettings(DEFAULT_SETTINGS, raw);
  return {
    ...merged,
    $schema: SETTINGS_SCHEMA_VERSION,
  };
}

function basenameWithoutJson(filePath: string): string {
  const fileName = basename(filePath);
  return fileName.endsWith('.json') ? fileName.slice(0, -5) : fileName;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export class SettingsService {
  private current: AppSettings = DEFAULT_SETTINGS;
  private readonly store: ElectronStore<AppSettings>;
  private readonly unsubscribeFromStore: () => void;
  private isLoading = false;
  private isWriting = false;
  private changeListeners: Array<(s: AppSettings) => void> = [];

  constructor(
    private readonly logger: Logger,
    filePath?: string,
  ) {
    const cwd = filePath === undefined ? app.getPath('userData') : dirname(filePath);
    const name = filePath === undefined ? 'settings' : basenameWithoutJson(filePath);
    const storeOptions = {

      cwd,
      name,
      defaults: DEFAULT_SETTINGS,
      migrations: {
        '>=0.0.0': (store: Conf<AppSettings>) => {
          store.store = normalise(store.store);
        },
      },
      clearInvalidConfig: true,
      serialize: (value: AppSettings) => `${JSON.stringify(normalise(value), null, 2)}\n`,
      deserialize: (value: string) => normalise(JSON.parse(value)),
      watch: true,
    };
    this.store = new ElectronStore<AppSettings>(
      {
        ...storeOptions,
        projectVersion: app.getVersion(),
      } as unknown as ConstructorParameters<typeof ElectronStore<AppSettings>>[0],
    );
    this.writeNormalisedStore();
    this.unsubscribeFromStore = this.store.onDidAnyChange((next) => {
      if (this.isLoading || this.isWriting) return;
      const normalised = normalise(next);
      this.current = normalised;
      this.broadcast(normalised);
      this.logger.info('Settings reloaded from disk (external edit)');
    });
  }

  get path(): string {
    return this.store.path;
  }

  get snapshot(): AppSettings {
    return this.current;
  }

  /** Read settings from disk, or write defaults if missing/corrupt. */
  async load(): Promise<AppSettings> {
    this.writeNormalisedStore();
    this.current = normalise(this.store.store);
    return this.current;
  }

  /**
   * Replace the in-memory snapshot with a deep-merge of `patch` onto the
   * current state, persist to disk, and broadcast to renderers.
   */
  async set(patch: Partial<AppSettings>): Promise<AppSettings> {
    const next = normalise(mergeSettings(this.current, patch));
    this.current = next;
    this.writeStore(next);
    this.broadcast(next);
    return next;
  }

  /** Reset to defaults. */
  async reset(): Promise<AppSettings> {
    this.current = DEFAULT_SETTINGS;
    this.writeStore(DEFAULT_SETTINGS);
    this.broadcast(DEFAULT_SETTINGS);
    return this.current;
  }

  /** Reveal settings.json in the system's default editor / file manager. */
  async openFile(): Promise<{ ok: true } | { err: string }> {
    const mkdirError = await fromPromise(
      mkdir(dirname(this.store.path), { recursive: true }),
      errorMessage,
    ).match(
      () => null,
      (err) => err,
    );
    if (mkdirError !== null) return { err: mkdirError };

    const writeResult = fromThrowable(
      () => {
        this.writeStore(this.current);
      },
      errorMessage,
    )();
    if (writeResult.isErr()) return { err: writeResult.error };

    const openResult = await fromPromise(
      shell.openPath(this.store.path),
      errorMessage,
    ).match(
      (err) => err,
      (err) => err,
    );
    if (openResult) return { err: openResult };
    return { ok: true };
  }

  /** Register a callback invoked whenever settings change (set, reset, or external edit). */
  addChangeListener(fn: (s: AppSettings) => void): void {
    this.changeListeners.push(fn);
  }

  stop(): void {
    this.unsubscribeFromStore();
    this.store._closeWatcher();
  }

  // ─── internals ───────────────────────────────────────────────

  private writeNormalisedStore(): void {
    this.isLoading = true;
    this.writeStore(normalise(this.store.store));
    this.isLoading = false;
  }

  private writeStore(state: AppSettings): void {
    this.isWriting = true;
    this.store.store = state;
    this.isWriting = false;
  }

  private broadcast(state: AppSettings): void {
    const windows = BrowserWindow.getAllWindows();
    for (const win of windows) {
      if (!win.isDestroyed()) {
        win.webContents.send('settings:changed', state);
      }
    }
    for (const fn of this.changeListeners) {
      fn(state);
    }
  }
}
