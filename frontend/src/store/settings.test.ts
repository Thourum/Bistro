// @vitest-environment jsdom
import { Shell } from '@bindings/services';
import { DEFAULT_SETTINGS } from '@renderer/types/settings';
import { afterEach, expect, it, vi } from 'vitest';
import { cancellable } from '../test/cancellable';
import { initSettings, settingsStore, updateSettings } from './settings';

vi.mock('@bindings/services', () => ({
  Shell: { GetSettings: vi.fn(), SetSettings: vi.fn() },
}));
afterEach(() => {
  settingsStore.setState(() => DEFAULT_SETTINGS);
  vi.clearAllMocks();
});

it('loads persisted engine settings before navigation', async () => {
  const settings = {
    ...DEFAULT_SETTINGS,
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' as const },
    sync: { ...DEFAULT_SETTINGS.sync, autoSync: false },
  };
  vi.mocked(Shell.GetSettings).mockReturnValue(
    cancellable(
      Promise.resolve({
        ...settings,
        window: { x: 0, y: 0, width: 0, height: 0, maximised: false },
      }),
    ),
  );
  await initSettings();
  expect(settingsStore.state).toEqual(settings);
});

it('uses the validated Go response rather than the requested settings patch', async () => {
  const returned = {
    ...DEFAULT_SETTINGS,
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'light' },
  };
  vi.mocked(Shell.SetSettings).mockReturnValue(
    cancellable(Promise.resolve(returned)),
  );
  const saved = await updateSettings({
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
  });
  expect(saved.isOk()).toBe(true);
  expect(Shell.SetSettings).toHaveBeenCalledWith(
    expect.objectContaining({
      appearance: expect.objectContaining({ theme: 'dark' }),
    }),
  );
  expect(settingsStore.state.appearance.theme).toBe('light');
});
