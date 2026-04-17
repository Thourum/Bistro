import { Store } from '@tanstack/store';
import {
  type AppSettings,
  DEFAULT_SETTINGS,
  SETTINGS_SCHEMA_VERSION,
} from '../../types/settings';

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

function deepMerge<T>(base: T, patch: unknown): T {
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) {
    return base;
  }
  const out = { ...(base as Record<string, unknown>) } as Record<
    string,
    unknown
  >;
  for (const [k, v] of Object.entries(patch as Record<string, unknown>)) {
    const current = out[k];
    if (
      current !== null &&
      typeof current === 'object' &&
      !Array.isArray(current) &&
      v !== null &&
      typeof v === 'object' &&
      !Array.isArray(v)
    ) {
      out[k] = deepMerge(current, v);
    } else if (v !== undefined) {
      out[k] = v;
    }
  }
  return out as T;
}

/**
 * Read-legacy `bistro-settings` localStorage key and translate its fields
 * into the new AppSettings shape. Returns a partial patch (missing keys
 * fall back to defaults).
 */
function migrateLegacyLocalStorage(): Partial<AppSettings> | null {
  try {
    const raw = localStorage.getItem(LEGACY_LOCAL_KEY);
    if (raw === null) return null;
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    localStorage.removeItem(LEGACY_LOCAL_KEY);
    const patch: Partial<AppSettings> = {
      appearance: {
        ...DEFAULT_SETTINGS.appearance,
        theme:
          parsed.darkMode === true ? ('dark' as const) : ('light' as const),
      },
      sync: {
        autoSync:
          typeof parsed.autoSync === 'boolean'
            ? parsed.autoSync
            : DEFAULT_SETTINGS.sync.autoSync,
        syncInterval:
          typeof parsed.syncInterval === 'number'
            ? parsed.syncInterval
            : DEFAULT_SETTINGS.sync.syncInterval,
        notifications:
          typeof parsed.notifications === 'boolean'
            ? parsed.notifications
            : DEFAULT_SETTINGS.sync.notifications,
      },
      prList: {
        view: parsed.prListView === 'grouped' ? 'grouped' : 'flat',
      },
    };
    return patch;
  } catch {
    return null;
  }
}

function persistLocal(state: AppSettings): void {
  try {
    localStorage.setItem(LOCAL_FALLBACK_KEY, JSON.stringify(state));
  } catch {
    // quota or serialisation error — ignore
  }
}

function loadLocal(): AppSettings {
  try {
    const raw = localStorage.getItem(LOCAL_FALLBACK_KEY);
    if (raw === null) return DEFAULT_SETTINGS;
    const parsed = JSON.parse(raw) as Partial<AppSettings>;
    return {
      ...deepMerge(DEFAULT_SETTINGS, parsed),
      $schema: SETTINGS_SCHEMA_VERSION,
    };
  } catch {
    return DEFAULT_SETTINGS;
  }
}

/**
 * Bootstrap the store. Call once from the app entry (before first render).
 */
export async function initSettings(): Promise<void> {
  if (hasElectron()) {
    // Run legacy localStorage migration once, then push result to main.
    const legacy = migrateLegacyLocalStorage();
    if (legacy !== null) {
      try {
        await window.electron.settings.set(legacy);
      } catch {
        // ignore — main will keep defaults
      }
    }
    const initial = await window.electron.settings.get();
    if ('ok' in initial) {
      settingsStore.setState(() => initial.ok);
    }
    // Subscribe to main-process changes (external edits, other windows).
    window.electron.on('settings:changed', (data) => {
      settingsStore.setState(() => data as AppSettings);
    });
    return;
  }

  // Web / dev fallback: localStorage.
  const legacy = migrateLegacyLocalStorage();
  const next = legacy ? deepMerge(loadLocal(), legacy) : loadLocal();
  settingsStore.setState(() => next);
  persistLocal(next);
}

/**
 * Apply a patch: round-trips through main when in Electron, else writes
 * to localStorage. Returns once the store has the new state.
 */
export async function updateSettings(
  patch: Partial<AppSettings>,
): Promise<void> {
  if (hasElectron()) {
    const res = await window.electron.settings.set(patch);
    if ('ok' in res) {
      settingsStore.setState(() => res.ok);
    }
    return;
  }
  const next = deepMerge(settingsStore.state, patch);
  settingsStore.setState(() => next);
  persistLocal(next);
}

export async function resetSettings(): Promise<void> {
  if (hasElectron()) {
    const res = await window.electron.settings.reset();
    if ('ok' in res) {
      settingsStore.setState(() => res.ok);
    }
    return;
  }
  settingsStore.setState(() => DEFAULT_SETTINGS);
  persistLocal(DEFAULT_SETTINGS);
}

export async function openSettingsFile(): Promise<void> {
  if (hasElectron()) {
    await window.electron.settings.openFile();
  }
}
