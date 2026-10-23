// @vitest-environment jsdom

import { afterEach, expect, it } from 'vitest';
import { DEFAULT_SETTINGS } from '../../types/settings';
import { initSettings, settingsStore } from './settings';

const LOCAL_SETTINGS_KEY = 'bistro-settings-v1';

afterEach(() => {
  localStorage.clear();
  settingsStore.setState(() => DEFAULT_SETTINGS);
});

it('preserves pre-navigation browser preferences and backfills the vim style', async () => {
  const savedSettings = Object.fromEntries(
    Object.entries({
      ...DEFAULT_SETTINGS,
      appearance: { ...DEFAULT_SETTINGS.appearance, theme: 'dark' },
      sync: { ...DEFAULT_SETTINGS.sync, autoSync: false },
      prList: { view: 'grouped' },
      keybindings: { 'list.down': ['ArrowDown'] },
    }).filter(([section]) => section !== 'navigation'),
  );
  localStorage.setItem(LOCAL_SETTINGS_KEY, JSON.stringify(savedSettings));

  await initSettings();

  const expected = { ...savedSettings, navigation: { style: 'vim' } };
  expect(settingsStore.state).toEqual(expected);
  expect(
    JSON.parse(localStorage.getItem(LOCAL_SETTINGS_KEY) ?? 'null'),
  ).toEqual(expected);
});
