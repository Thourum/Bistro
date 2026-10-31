// @vitest-environment jsdom
import { Shell } from '@bindings/services';
import type { Settings } from '@bindings/shell/models';
import { DEFAULT_SETTINGS } from '@renderer/types/settings';
import { afterEach, expect, it, vi } from 'vitest';
import { cancellable } from '../test/cancellable';
import {
  initSettings,
  resetSettings,
  settingsStore,
  updateSettings,
} from './settings';

vi.mock('@bindings/services', () => ({
  Shell: { GetSettings: vi.fn(), SetSettings: vi.fn(), ResetSettings: vi.fn() },
}));
afterEach(() => {
  settingsStore.setState(() => DEFAULT_SETTINGS);
  vi.clearAllMocks();
});

it('omits reset keyboard overrides while preserving other bindings and applying the Go response', async () => {
  const otherBindings = { 'list.down': ['j'] };
  settingsStore.setState(() => ({
    ...DEFAULT_SETTINGS,
    keybindings: otherBindings,
  }));
  const savedBindings = { ...otherBindings, 'list.top': ['g', 'g'] };
  const returnedBindings = { 'list.down': ['ArrowDown'] };
  vi.mocked(Shell.SetSettings)
    .mockReturnValueOnce(
      cancellable(
        Promise.resolve({ ...DEFAULT_SETTINGS, keybindings: savedBindings }),
      ),
    )
    .mockReturnValueOnce(
      cancellable(
        Promise.resolve({ ...DEFAULT_SETTINGS, keybindings: returnedBindings }),
      ),
    );
  expect(
    (await updateSettings({ keybindings: { 'list.top': ['g', 'g'] } })).isOk(),
  ).toBe(true);
  expect(settingsStore.state.keybindings).toEqual(savedBindings);
  expect(
    (await updateSettings({ keybindings: { 'list.top': null } })).isOk(),
  ).toBe(true);
  const resetPayload = vi.mocked(Shell.SetSettings).mock.calls[1]?.[0];
  expect(resetPayload?.keybindings).toEqual(otherBindings);
  expect(resetPayload?.keybindings).not.toHaveProperty('list.top');
  expect(settingsStore.state.keybindings).toEqual(returnedBindings);
});

it('queues reset between pending edits', async () => {
  const first: { resolve: (settings: Settings) => void } = {
    resolve: () => undefined,
  };
  vi.mocked(Shell.SetSettings)
    .mockReturnValueOnce(
      cancellable(
        new Promise<Settings>((resolve) => {
          first.resolve = resolve;
        }),
      ),
    )
    .mockImplementationOnce((settings) =>
      cancellable(Promise.resolve(settings)),
    );
  vi.mocked(Shell.ResetSettings).mockReturnValueOnce(
    cancellable(Promise.resolve(DEFAULT_SETTINGS)),
  );
  const theme = updateSettings({ appearance: { theme: 'dark' } });
  const reset = resetSettings();
  const density = updateSettings({ appearance: { highDensity: true } });
  await vi.waitFor(() => expect(Shell.SetSettings).toHaveBeenCalledTimes(1));
  expect(Shell.ResetSettings).not.toHaveBeenCalled();
  first.resolve({
    ...DEFAULT_SETTINGS,
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
  });
  await theme;
  await reset;
  expect((await density).isOk()).toBe(true);
  expect(settingsStore.state.appearance).toMatchObject({
    theme: 'light',
    highDensity: true,
  });
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

it('returns an error and retains defaults when loading engine settings fails', async () => {
  vi.mocked(Shell.GetSettings).mockReturnValueOnce(
    cancellable(Promise.reject(new Error('settings unavailable'))),
  );
  expect((await initSettings()).isErr()).toBe(true);
  expect(settingsStore.state).toEqual(DEFAULT_SETTINGS);
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

it('retains two pending edits in the same settings section', async () => {
  const first: { resolve: (settings: Settings) => void } = {
    resolve: () => undefined,
  };
  vi.mocked(Shell.SetSettings)
    .mockReturnValueOnce(
      cancellable(
        new Promise<Settings>((resolve) => {
          first.resolve = resolve;
        }),
      ),
    )
    .mockImplementationOnce((settings) =>
      cancellable(Promise.resolve(settings)),
    );
  const theme = updateSettings({ appearance: { theme: 'dark' } });
  const density = updateSettings({ appearance: { highDensity: true } });
  await vi.waitFor(() => expect(Shell.SetSettings).toHaveBeenCalledTimes(1));
  first.resolve({
    ...DEFAULT_SETTINGS,
    appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
  });
  expect((await theme).isOk()).toBe(true);
  expect((await density).isOk()).toBe(true);
  expect(settingsStore.state.appearance).toMatchObject({
    theme: 'dark',
    highDensity: true,
  });
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
