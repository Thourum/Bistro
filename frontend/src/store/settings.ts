import { Shell } from '@bindings/services';
import { Store } from '@tanstack/store';
import { type Fault, ResultAsync } from '@thourum/fault';
import { call, callWrite } from '../services/engine/call';
import {
  type AppSettings,
  appSettingsSchema,
  DEFAULT_SETTINGS,
  type DisplaySettings,
} from '../types/settings';

/** Renderer display state; the shell owns settings validation and persistence. */
export const settingsStore: Store<DisplaySettings> = new Store(
  DEFAULT_SETTINGS,
);

export function applySettings(settings: AppSettings): void {
  const parsed = appSettingsSchema.safeParse(settings);
  if (parsed.success) settingsStore.setState(() => parsed.data);
  else console.warn('Settings: Invalid settings from engine');
}

function reportSettingsError(fault: Fault): void {
  console.warn(`Settings: ${fault.message}`);
}

export async function initSettings(): Promise<void> {
  await call(() => Shell.GetSettings()).match(
    (settings) => applySettings(settings),
    reportSettingsError,
  );
}

// Full snapshots must be merged only after the preceding shell write settles.
const settingsWrites = { settled: Promise.resolve() };

export function updateSettings(
  patch: Partial<AppSettings>,
): ResultAsync<void, Fault> {
  const write = settingsWrites.settled.then(() => writeSettingsPatch(patch));
  settingsWrites.settled = write.then(() => undefined);
  return new ResultAsync(write);
}

function writeSettingsPatch(
  patch: Partial<AppSettings>,
): ResultAsync<void, Fault> {
  const current = settingsStore.state;
  const merged: AppSettings = {
    ...current,
    ...patch,
    appearance: { ...current.appearance, ...patch.appearance },
    sync: { ...current.sync, ...patch.sync },
    prList: { ...current.prList, ...patch.prList },
    diff: { ...current.diff, ...patch.diff },
    privacy: { ...current.privacy, ...patch.privacy },
    repositories: { ...current.repositories, ...patch.repositories },
    classification: { ...current.classification, ...patch.classification },
    navigation: { ...current.navigation, ...patch.navigation },
    window: { ...current.window, ...patch.window },
  };
  return callWrite(() => Shell.SetSettings(merged)).map((settings) => {
    applySettings(settings);
    return undefined;
  });
}

export async function resetSettings(): Promise<void> {
  await callWrite(() => Shell.ResetSettings()).match(
    (settings) => applySettings(settings),
    reportSettingsError,
  );
}

export async function openSettingsFile(): Promise<void> {
  await call(() => Shell.OpenSettingsFile()).match(
    () => undefined,
    reportSettingsError,
  );
}
