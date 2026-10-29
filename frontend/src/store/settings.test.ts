// @vitest-environment jsdom
import { Shell } from '@bindings/services';
import { DEFAULT_SETTINGS } from '@renderer/types/electron/settings';
import { afterEach, expect, it, vi } from 'vitest';
import { cancellable } from '../test/cancellable';
import { initSettings, settingsStore } from './settings';

vi.mock('@bindings/services', () => ({ Shell: { GetSettings: vi.fn() } }));
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
