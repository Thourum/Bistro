import { errAsync, Fault, okAsync } from '@thourum/fault';
import { beforeEach, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import { initialUIState } from '../store/ui';
import { dismissFirstRunRail, recordFirstRunAction } from './firstRunRail';

vi.mock('../store/settings', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../store/settings')>();
  return { ...actual, updateSettings: vi.fn() };
});

function setAppearance(
  patch: Partial<typeof settingsStore.state.appearance>,
): void {
  settingsStore.setState((s) => ({
    ...s,
    appearance: { ...s.appearance, ...patch },
  }));
}

beforeEach(() => {
  vi.clearAllMocks();
  appStore.setState(() => ({ ...initialUIState }));
  setAppearance({ onboardingComplete: true, tourComplete: false });
});

it('does not count actions before onboarding completes or after dismissal', () => {
  setAppearance({ onboardingComplete: false });
  recordFirstRunAction('list.open');
  expect(appStore.state.firstRunRail.completed).toEqual([]);
  setAppearance({ onboardingComplete: true });
  recordFirstRunAction('list.open');
  expect(appStore.state.firstRunRail.completed).toEqual(['open']);
  setAppearance({ tourComplete: true });
  recordFirstRunAction('detail.comment');
  expect(appStore.state.firstRunRail.completed).toEqual(['open']);
});

it('keeps dismissal retryable on IPC failure and writes only the existing appearance flag', async () => {
  vi.mocked(updateSettings).mockReturnValueOnce(
    errAsync(new Fault('Disk unavailable').withTag('SETTINGS_ERROR')),
  );
  const failed = await dismissFirstRunRail();
  expect(failed.isErr()).toBe(true);
  expect(appStore.state.firstRunDismissPending).toBe(false);
  expect(settingsStore.state.appearance.tourComplete).toBe(false);
  vi.mocked(updateSettings).mockReturnValueOnce(okAsync(undefined));
  expect((await dismissFirstRunRail()).isOk()).toBe(true);
  expect(updateSettings).toHaveBeenLastCalledWith({
    appearance: { ...settingsStore.state.appearance, tourComplete: true },
  });
});
