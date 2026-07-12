import { describe, expect, it } from 'vitest';
import {
  DEFAULT_SETTINGS,
  SETTINGS_SCHEMA_VERSION,
  type AppSettings,
} from '../../src/types/settings';
import { normalise } from '../../src/main/settings';

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
      keybindings: DEFAULT_SETTINGS.keybindings,
    } satisfies Omit<AppSettings, 'classification'>;

    expect(normalise(oldSettings).classification).toEqual(
      DEFAULT_SETTINGS.classification,
    );
  });
});
