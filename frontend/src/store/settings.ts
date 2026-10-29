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
import { z } from 'zod';

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
const LEGACY_LOCAL_KEY = 'bistro-settings';

function hasElectron(): boolean {
  return typeof window !== 'undefined' && window.electron !== undefined;
}

const legacySettingsSchema = z.object({
  darkMode: z.boolean().optional(),
  autoSync: z.boolean().optional(),
  syncInterval: z.number().finite().positive().optional(),
  notifications: z.boolean().optional(),
  prListView: z.enum(['flat', 'grouped']).optional(),
});

function readLocalJson(key: string): Result<unknown | null, Fault> {
  return fromThrowable(
    () => localStorage.getItem(key),
    (cause) =>
      new Fault(String(cause)).withTag('STORAGE_ERROR').withCause(cause),
  )().andThen((raw) =>
    raw === null
      ? ok(null)
      : fromThrowable(
          () => JSON.parse(raw) as unknown,
          (cause) =>
            new Fault(String(cause)).withTag('PARSE_ERROR').withCause(cause),
        )(),
  );
}

function migrateLegacyLocalStorage(): Result<
  Partial<AppSettings> | null,
  Fault
> {
  return readLocalJson(LEGACY_LOCAL_KEY).andThen((value) => {
    if (value === null) return ok(null);
    const parsed = legacySettingsSchema.safeParse(value);
    if (!parsed.success)
      return err(ServiceError('VALIDATION_ERROR', 'Invalid legacy settings'));
    const legacy = parsed.data;
    const patch: Partial<AppSettings> = {
      appearance: {
        ...DEFAULT_SETTINGS.appearance,
        theme: legacy.darkMode === true ? 'dark' : 'light',
      },
      sync: {
        autoSync: legacy.autoSync ?? DEFAULT_SETTINGS.sync.autoSync,
        syncInterval: legacy.syncInterval ?? DEFAULT_SETTINGS.sync.syncInterval,
        notifications:
          legacy.notifications ?? DEFAULT_SETTINGS.sync.notifications,
      },
      prList: { view: legacy.prListView ?? DEFAULT_SETTINGS.prList.view },
    };
    return ok(patch);
  });
}

function persistLocal(state: AppSettings): Result<void, Fault> {
  return fromThrowable(
    () => localStorage.setItem(LOCAL_FALLBACK_KEY, JSON.stringify(state)),
    (cause) =>
      new Fault(String(cause)).withTag('STORAGE_ERROR').withCause(cause),
  )();
}

function loadLocal(): Result<AppSettings, Fault> {
  return readLocalJson(LOCAL_FALLBACK_KEY).andThen((value) => {
    if (value === null) return ok(DEFAULT_SETTINGS);
    const saved = z.record(z.string(), z.unknown()).safeParse(value);
    if (!saved.success)
      return err(ServiceError('VALIDATION_ERROR', 'Invalid local settings'));
    const parsed = appSettingsSchema.safeParse({
      ...DEFAULT_SETTINGS,
      ...saved.data,
    });
    return parsed.success
      ? ok(parsed.data)
      : err(ServiceError('VALIDATION_ERROR', 'Invalid local settings'));
  });
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
  const legacy = migrateLegacyLocalStorage().match(
    (value) => value,
    (message) => {
      reportSettingsError(message);
      return null;
    },
  );
  if (hasElectron()) {
    if (legacy !== null) {
      const migrated = await ResultAsync.fromPromise(
        window.electron.settings.set(legacy),
        (cause) =>
          new Fault(String(cause)).withTag('SETTINGS_ERROR').withCause(cause),
      ).match(
        (response) => response,
        (message) => ({ err: message }),
      );
      if ('ok' in migrated) {
        fromThrowable(
          () => localStorage.removeItem(LEGACY_LOCAL_KEY),
          (cause) =>
            new Fault(String(cause)).withTag('STORAGE_ERROR').withCause(cause),
        )().orInspect(reportSettingsError);
      } else {
        reportSettingsError(migrated.err);
      }
    }
    await ResultAsync.fromPromise(window.electron.settings.get(), (cause) =>
      new Fault(String(cause)).withTag('SETTINGS_ERROR').withCause(cause),
    ).match((initial) => {
      if ('ok' in initial) {
        const validated = appSettingsSchema.safeParse(initial.ok);
        if (validated.success) settingsStore.setState(() => validated.data);
        else reportSettingsError('Invalid settings from main process');
      } else {
        reportSettingsError(initial.err);
      }
    }, reportSettingsError);
    window.electron.on('settings:changed', (data) => {
      const validated = appSettingsSchema.safeParse(data);
      if (validated.success) settingsStore.setState(() => validated.data);
      else reportSettingsError('Invalid settings update from main process');
    });
    return;
  }

  const current = loadLocal().match(
    (settings) => settings,
    (message) => {
      reportSettingsError(message);
      return DEFAULT_SETTINGS;
    },
  );
  const next =
    legacy === null
      ? current
      : mergeLocalSettings(current, legacy).match(
          (settings) => settings,
          (message) => {
            reportSettingsError(message);
            return current;
          },
        );
  settingsStore.setState(() => next);
  const persisted = persistLocal(next);
  persisted.orInspect(reportSettingsError);
  if (legacy !== null && persisted.isOk()) {
    fromThrowable(
      () => localStorage.removeItem(LEGACY_LOCAL_KEY),
      (cause) =>
        new Fault(String(cause)).withTag('STORAGE_ERROR').withCause(cause),
    )().orInspect(reportSettingsError);
  }
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
