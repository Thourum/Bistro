import { mkdir } from 'node:fs/promises';
import { basename, dirname } from 'node:path';
import { app, BrowserWindow, shell } from 'electron';
import type { Logger } from 'electron-log';
import ElectronStore, {
  type Options as ElectronStoreOptions,
} from 'electron-store';
import { fromPromise, fromThrowable, type Result } from 'neverthrow';
import { z } from 'zod';
import {
  type AppSettings,
  appSettingsSchema,
  DEFAULT_SETTINGS,
  SETTINGS_SCHEMA_VERSION,
} from '../types/settings';

/** Keep known fields; reject invalid leaf values without losing valid siblings. */
function mergeSettings(base: AppSettings, patch: unknown): AppSettings {
  const shape = appSettingsSchema.shape;
  const parsed = z.record(z.string(), z.unknown()).safeParse(patch);
  if (!parsed.success) return base;

  const merged = Object.fromEntries(
    Object.entries(shape).map(([section, schema]) => {
      const current = base[section as keyof AppSettings];
      const incoming = parsed.data[section];
      if (incoming === undefined || section === '$schema')
        return [section, current];
      if (section === 'keybindings') {
        const validated = schema.safeParse(incoming);
        return [section, validated.success ? validated.data : current];
      }
      const entries = z.record(z.string(), z.unknown()).safeParse(incoming);
      if (!entries.success || typeof current !== 'object') {
        return [section, current];
      }
      const sectionShape = (schema as typeof appSettingsSchema.shape.appearance)
        .shape;
      const known = Object.fromEntries(
        Object.entries(sectionShape).map(([key, field]) => {
          const value = entries.data[key];
          const validated = field.safeParse(value);
          return [
            key,
            validated.success
              ? validated.data
              : current[key as keyof typeof current],
          ];
        }),
      );
      return [section, known];
    }),
  );
  const validated = appSettingsSchema.safeParse(merged);
  return validated.success ? validated.data : base;
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
    const cwd =
      filePath === undefined ? app.getPath('userData') : dirname(filePath);
    const name =
      filePath === undefined ? 'settings' : basenameWithoutJson(filePath);
    const storeOptions = {
      cwd,
      name,
      defaults: DEFAULT_SETTINGS,
      migrations: {
        '>=0.0.0': (store) => {
          store.store = normalise(store.store);
        },
      } satisfies ElectronStoreOptions<AppSettings>['migrations'],
      clearInvalidConfig: true,
      serialize: (value: AppSettings) =>
        `${JSON.stringify(normalise(value), null, 2)}\n`,
      deserialize: (value: string) =>
        fromThrowable(
          () => JSON.parse(value) as unknown,
          () => null,
        )().match(normalise, () => DEFAULT_SETTINGS),
      watch: true,
    };
    this.store = new ElectronStore<AppSettings>(storeOptions);
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
  load(): Promise<AppSettings> {
    return this.loadResult().match(
      (settings) => Promise.resolve(settings),
      (message) => Promise.reject(new Error(message)),
    );
  }

  loadResult(): Result<AppSettings, string> {
    return this.writeNormalisedStore()
      .andThen(() =>
        fromThrowable(() => normalise(this.store.store), errorMessage)(),
      )
      .map((settings) => {
        this.current = settings;
        return settings;
      });
  }

  /** Persist a deep-merge of `patch`, then update and broadcast the snapshot. */
  set(patch: Partial<AppSettings>): Promise<AppSettings> {
    const result = this.setResult(patch);
    return 'ok' in result
      ? Promise.resolve(result.ok)
      : Promise.reject(new Error(result.err));
  }

  setResult(
    patch: Partial<AppSettings>,
  ): { ok: AppSettings } | { err: string } {
    const next = mergeSettings(this.current, patch);
    const written = this.writeStore(next);
    if (written.isErr()) return { err: errorMessage(written.error) };
    this.current = next;
    this.broadcast(next);
    return { ok: next };
  }

  /** Reset to defaults. */
  reset(): Promise<AppSettings> {
    const result = this.resetResult();
    return 'ok' in result
      ? Promise.resolve(result.ok)
      : Promise.reject(new Error(result.err));
  }

  resetResult(): { ok: AppSettings } | { err: string } {
    const written = this.writeStore(DEFAULT_SETTINGS);
    if (written.isErr()) return { err: errorMessage(written.error) };
    this.current = DEFAULT_SETTINGS;
    this.broadcast(DEFAULT_SETTINGS);
    return { ok: this.current };
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

    const writeResult = this.writeStore(this.current);
    if (writeResult.isErr()) return { err: errorMessage(writeResult.error) };

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

  private writeNormalisedStore(): Result<void, string> {
    this.isLoading = true;
    const result = fromThrowable(
      () => normalise(this.store.store),
      errorMessage,
    )().andThen((state) => this.writeStore(state));
    this.isLoading = false;
    return result;
  }

  private writeStore(state: AppSettings): Result<void, string> {
    this.isWriting = true;
    const result = fromThrowable(() => {
      this.store.store = state;
    }, errorMessage)();
    this.isWriting = false;
    return result;
  }

  private broadcast(state: AppSettings): void {
    for (const win of BrowserWindow.getAllWindows()) {
      if (!win.isDestroyed()) {
        fromThrowable(() =>
          win.webContents.send('settings:changed', state),
        )().mapErr((error) =>
          this.logger.warn('Settings broadcast failed', error),
        );
      }
    }
    for (const listener of this.changeListeners) {
      fromThrowable(() => listener(state))().mapErr((error) =>
        this.logger.warn('Settings listener failed', error),
      );
    }
  }
}
