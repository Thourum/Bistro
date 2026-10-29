// @vitest-environment jsdom
import { Shell } from '@bindings/services';
import type { Settings } from '@bindings/shell/models';
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

it('merges overlapping patches after the preceding write returns', async () => {
  const first: { resolve: (settings: Settings) => void } = {
    resolve: () => undefined,
  };
  const second: { resolve: (settings: Settings) => void } = {
    resolve: () => undefined,
  };
  const firstPromise = new Promise<Settings>((resolve) => {
    first.resolve = resolve;
  });
  const secondPromise = new Promise<Settings>((resolve) => {
    second.resolve = resolve;
  });
  vi.mocked(Shell.SetSettings)
    .mockReturnValueOnce(cancellable(firstPromise))
    .mockReturnValueOnce(cancellable(secondPromise));

  const themeWrite = updateSettings({
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
  });
  const syncWrite = updateSettings({
    sync: { ...DEFAULT_SETTINGS.sync, autoSync: false },
  });
  await vi.waitFor(() => expect(Shell.SetSettings).toHaveBeenCalledTimes(1));
  first.resolve({
    ...DEFAULT_SETTINGS,
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
  });
  expect((await themeWrite).isOk()).toBe(true);
  await vi.waitFor(() => expect(Shell.SetSettings).toHaveBeenCalledTimes(2));
  const secondPayload = vi.mocked(Shell.SetSettings).mock.calls[1]?.[0];
  expect(secondPayload?.appearance.theme).toBe('dark');
  expect(secondPayload?.sync.autoSync).toBe(false);
  second.resolve(secondPayload ?? DEFAULT_SETTINGS);
  expect((await syncWrite).isOk()).toBe(true);
  expect(settingsStore.state.appearance.theme).toBe('dark');
  expect(settingsStore.state.sync.autoSync).toBe(false);
});

it('continues queued settings writes after a failed write', async () => {
  vi.mocked(Shell.SetSettings)
    .mockReturnValueOnce(cancellable(Promise.reject(new Error('disk full'))))
    .mockImplementationOnce((settings) =>
      cancellable(Promise.resolve(settings)),
    );
  const failedWrite = updateSettings({
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
  });
  const nextWrite = updateSettings({
    sync: { ...DEFAULT_SETTINGS.sync, autoSync: false },
  });
  expect((await failedWrite).isErr()).toBe(true);
  expect((await nextWrite).isOk()).toBe(true);
  expect(settingsStore.state.appearance.theme).toBe('light');
  expect(settingsStore.state.sync.autoSync).toBe(false);
});
