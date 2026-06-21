import { unwatchFile, watchFile } from 'node:fs';
import { access, mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { app, BrowserWindow, shell } from 'electron';
import type { Logger } from 'electron-log';
import { merge } from 'ts-deepmerge';
import {
  type AppSettings,
  DEFAULT_SETTINGS,
  SETTINGS_SCHEMA_VERSION,
} from '../types/settings';

/** Match prior behaviour: nested objects merge; arrays are replaced wholesale. */
function mergeSettings(base: AppSettings, patch: unknown): AppSettings {
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) {
    return base;
  }
  return merge.withOptions(
    { mergeArrays: false, allowUndefinedOverrides: false },
    base,
    patch as Record<string, unknown>,
  ) as unknown as AppSettings;
}

/**
 * Normalise arbitrary JSON read from disk into a valid AppSettings.
 * Missing fields are filled from defaults; unknown fields are discarded.
 */
function normalise(raw: unknown): AppSettings {
  const merged = mergeSettings(DEFAULT_SETTINGS, raw);
  return {
    ...merged,
    $schema: SETTINGS_SCHEMA_VERSION,
  };
}

export class SettingsService {
  private current: AppSettings = DEFAULT_SETTINGS;
  private readonly filePath: string;
  private writing = false;
  private watchHandle: ReturnType<typeof watchFile> | null = null;
  private changeListeners: Array<(s: AppSettings) => void> = [];

  constructor(
    private readonly logger: Logger,
    filePath?: string,
  ) {
    this.filePath = filePath ?? join(app.getPath('userData'), 'settings.json');
  }

  get path(): string {
    return this.filePath;
  }

  get snapshot(): AppSettings {
    return this.current;
  }

  /** Read settings from disk, or write defaults if missing/corrupt. */
  async load(): Promise<AppSettings> {
    try {
      await access(this.filePath);
    } catch {
      this.logger.info(
        `Settings file missing; writing defaults to ${this.filePath}`,
      );
      await this.persist(DEFAULT_SETTINGS);
      this.current = DEFAULT_SETTINGS;
      this.startWatching();
      return this.current;
    }
    try {
      const raw = await readFile(this.filePath, 'utf-8');
      const parsed = JSON.parse(raw);
      this.current = normalise(parsed);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      this.logger.warn(
        `Failed to parse settings (${msg}); reverting to defaults`,
      );
      this.current = DEFAULT_SETTINGS;
      await this.persist(DEFAULT_SETTINGS);
    }
    this.startWatching();
    return this.current;
  }

  /**
   * Replace the in-memory snapshot with a deep-merge of `patch` onto the
   * current state, persist to disk, and broadcast to renderers.
   */
  async set(patch: Partial<AppSettings>): Promise<AppSettings> {
    const next = normalise(mergeSettings(this.current, patch));
    this.current = next;
    await this.persist(next);
    this.broadcast(next);
    return next;
  }

  /** Reset to defaults. */
  async reset(): Promise<AppSettings> {
    this.current = DEFAULT_SETTINGS;
    await this.persist(DEFAULT_SETTINGS);
    this.broadcast(DEFAULT_SETTINGS);
    return this.current;
  }

  /** Reveal settings.json in the system's default editor / file manager. */
  async openFile(): Promise<{ ok: true } | { err: string }> {
    try {
      await mkdir(dirname(this.filePath), { recursive: true });
      // Ensure the file exists before opening
      await access(this.filePath).catch(() => this.persist(this.current));
      const err = await shell.openPath(this.filePath);
      if (err) return { err };
      return { ok: true };
    } catch (err) {
      return { err: err instanceof Error ? err.message : String(err) };
    }
  }

  /** Register a callback invoked whenever settings change (set, reset, or external edit). */
  addChangeListener(fn: (s: AppSettings) => void): void {
    this.changeListeners.push(fn);
  }

  stop(): void {
    if (this.watchHandle !== null) {
      unwatchFile(this.filePath);
      this.watchHandle = null;
    }
  }

  // ─── internals ───────────────────────────────────────────────

  private async persist(state: AppSettings): Promise<void> {
    this.writing = true;
    try {
      await mkdir(dirname(this.filePath), { recursive: true });
      const body = `${JSON.stringify(state, null, 2)}\n`;
      await writeFile(this.filePath, body, 'utf-8');
    } finally {
      // Debounce: clear the "our own write" flag shortly after so external
      // edits land again. `watchFile` polls every ~1s, so 500ms is safe.
      setTimeout(() => {
        this.writing = false;
      }, 500);
    }
  }

  private startWatching(): void {
    if (this.watchHandle !== null) return;
    this.watchHandle = watchFile(
      this.filePath,
      { interval: 1000 },
      (curr, prev) => {
        if (this.writing) return;
        if (curr.mtimeMs === prev.mtimeMs) return;
        this.reloadFromDisk().catch((err) => {
          this.logger.error(
            `External settings reload failed: ${
              err instanceof Error ? err.message : String(err)
            }`,
          );
        });
      },
    );
  }

  private async reloadFromDisk(): Promise<void> {
    try {
      const raw = await readFile(this.filePath, 'utf-8');
      const next = normalise(JSON.parse(raw));
      this.current = next;
      this.broadcast(next);
      this.logger.info('Settings reloaded from disk (external edit)');
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      this.logger.warn(`External settings reload: ${msg}`);
    }
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
