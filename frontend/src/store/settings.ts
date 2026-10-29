import { Shell } from '@bindings/services';
import {
  type AppSettings,
  appSettingsSchema,
  DEFAULT_SETTINGS,
} from '@renderer/types/electron/settings';
import { Store } from '@tanstack/store';
import {
  err,
  errAsync,
  Fault,
  fromThrowable,
  ok,
  okAsync,
  type Result,
  ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { call } from '../services/engine/call';

/**
 * Settings store — mirrors the main-process `settings.json`.
 *
 * The renderer is the **source of display state** (what the UI reads), but
 * the main process is the **source of truth on disk**. Every write goes
 * through `updateSettings()` which RPCs to main and waits for main to echo
 * back via `settings:changed`.
 *
 * Falls back to `localStorage` when running outside Electron (e.g. under
 * `pnpm dev:web`) so the redesign can be previewed in a browser.
 */
export const settingsStore: Store<AppSettings> = new Store<AppSettings>(
  DEFAULT_SETTINGS,
);

const LOCAL_FALLBACK_KEY = 'bistro-settings-v1';

function hasElectron(): boolean {
  return typeof window !== 'undefined' && window.electron !== undefined;
}

function persistLocal(state: AppSettings): Result<void, Fault> {
  return fromThrowable(
    () => localStorage.setItem(LOCAL_FALLBACK_KEY, JSON.stringify(state)),
    (cause) =>
      new Fault(String(cause)).withTag('STORAGE_ERROR').withCause(cause),
  )();
}

function reportSettingsError(error: Fault | string): void {
  console.warn(
    `Settings: ${typeof error === 'string' ? error : error.message}`,
  );
}

function mergeLocalSettings(
  base: AppSettings,
  patch: Partial<AppSettings>,
): Result<AppSettings, Fault> {
  const merged = Object.fromEntries(
    Object.entries(patch).map(([section, value]) => {
      if (section === '$schema' || value === undefined)
        return [section, base[section as keyof AppSettings]];
      if (section === 'keybindings') return [section, value];
      const current = base[section as keyof AppSettings];
      return [
        section,
        typeof value === 'object' &&
        value !== null &&
        typeof current === 'object'
          ? { ...current, ...value }
          : value,
      ];
    }),
  );
  const parsed = appSettingsSchema.safeParse({ ...base, ...merged });
  return parsed.success
    ? ok(parsed.data)
    : err(ServiceError('VALIDATION_ERROR', 'Invalid settings patch'));
}

/**
 * Bootstrap the store. Call once from the app entry (before first render).
 */
export async function initSettings(): Promise<void> {
  await call(() => Shell.GetSettings()).match((settings) => {
    // Settings originate in a user-editable file, so retain file validation.
    const parsed = appSettingsSchema.safeParse(settings);
    if (parsed.success) settingsStore.setState(() => parsed.data);
    else reportSettingsError('Invalid settings from engine');
  }, reportSettingsError);
}

/**
 * Apply a patch: round-trips through main when in Electron, else writes
 * to localStorage. Resolves once the store has the new state; an IPC
 * failure surfaces as an err so callers can gate follow-up work on it.
 */
export function updateSettings(
  patch: Partial<AppSettings>,
): ResultAsync<void, Fault> {
  if (hasElectron()) {
    return ResultAsync.fromPromise(
      window.electron.settings.set(patch),
      (cause) =>
        new Fault(
          cause instanceof Error && cause.message.trim() !== ''
            ? cause.message
            : 'Unable to write settings.',
        )
          .withTag('SETTINGS_ERROR')
          .withCause(cause),
    ).andThen((res) => {
      if ('ok' in res) {
        const parsed = appSettingsSchema.safeParse(res.ok);
        if (!parsed.success)
          return err(
            ServiceError('VALIDATION_ERROR', 'Invalid settings response'),
          );
        settingsStore.setState(() => parsed.data);
        return ok(undefined);
      }
      return err(
        ServiceError(
          'SETTINGS_ERROR',
          res.err.trim() === '' ? 'Unable to write settings.' : res.err,
        ),
      );
    });
  }
  return mergeLocalSettings(settingsStore.state, patch)
    .andThen((next) => persistLocal(next).map(() => next))
    .match(
      (next) => {
        settingsStore.setState(() => next);
        return okAsync(undefined);
      },
      (message) => errAsync(message),
    );
}

export async function resetSettings(): Promise<void> {
  if (hasElectron()) {
    await ResultAsync.fromPromise(window.electron.settings.reset(), (cause) =>
      new Fault(String(cause)).withTag('SETTINGS_ERROR').withCause(cause),
    ).match((response) => {
      if ('err' in response) {
        reportSettingsError(response.err);
        return;
      }
      const parsed = appSettingsSchema.safeParse(response.ok);
      if (parsed.success) settingsStore.setState(() => parsed.data);
      else reportSettingsError('Invalid settings reset from main process');
    }, reportSettingsError);
    return;
  }
  persistLocal(DEFAULT_SETTINGS).match(
    () => settingsStore.setState(() => DEFAULT_SETTINGS),
    reportSettingsError,
  );
}

export async function openSettingsFile(): Promise<void> {
  if (!hasElectron()) return;
  await ResultAsync.fromPromise(window.electron.settings.openFile(), (cause) =>
    new Fault(String(cause)).withTag('SETTINGS_ERROR').withCause(cause),
  ).match((response) => {
    if ('err' in response) reportSettingsError(response.err);
  }, reportSettingsError);
}
