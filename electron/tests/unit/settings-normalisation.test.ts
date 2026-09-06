import { describe, expect, it, vi } from 'vitest';

vi.mock('electron-store', () => ({
  default: class MockElectronStore {},
}));

vi.mock('electron', () => ({
  BrowserWindow: { getAllWindows: vi.fn(() => []) },
  app: { getPath: vi.fn(() => ''), getVersion: vi.fn(() => '0.0.0') },
  ipcMain: { on: vi.fn() },
  shell: { openPath: vi.fn() },
}));

import { normalise } from '../../src/main/settings';
import {
  type AppSettings,
  DEFAULT_SETTINGS,
  SETTINGS_SCHEMA_VERSION,
} from '../../src/types/settings';

describe('settings normalisation', () => {
  it('defines default classification settings without bumping the schema', () => {
    expect(SETTINGS_SCHEMA_VERSION).toBe(1);
    expect(DEFAULT_SETTINGS.classification).toEqual({
      mode: 'rules',
      dismissalScope: 'local',
      provider: 'anthropic',
      model: '',
      endpoint: '',
    });
  });

  it('fills classification settings for an old settings object', () => {
    const oldSettings = {
      $schema: SETTINGS_SCHEMA_VERSION,
      appearance: DEFAULT_SETTINGS.appearance,
      sync: DEFAULT_SETTINGS.sync,
      prList: DEFAULT_SETTINGS.prList,
      privacy: DEFAULT_SETTINGS.privacy,
      repositories: DEFAULT_SETTINGS.repositories,
      keybindings: DEFAULT_SETTINGS.keybindings,
    } satisfies Omit<AppSettings, 'classification'>;

    expect(normalise(oldSettings).classification).toEqual(
      DEFAULT_SETTINGS.classification,
    );
  });

  it('fills diff settings for a settings object written before the section existed', () => {
    const withoutDiff = {
      $schema: SETTINGS_SCHEMA_VERSION,
      appearance: DEFAULT_SETTINGS.appearance,
      sync: DEFAULT_SETTINGS.sync,
      prList: DEFAULT_SETTINGS.prList,
      privacy: DEFAULT_SETTINGS.privacy,
      repositories: DEFAULT_SETTINGS.repositories,
      classification: DEFAULT_SETTINGS.classification,
      keybindings: DEFAULT_SETTINGS.keybindings,
    } satisfies Omit<AppSettings, 'diff'>;

    expect(normalise(withoutDiff).diff).toEqual({ style: 'unified' });
  });
});
