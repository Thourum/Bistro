import { describe, expect, it, vi } from 'vitest';

vi.mock('electron', () => ({
  ipcMain: { handle: vi.fn() },
  dialog: {},
  app: { getPath: vi.fn(() => '/tmp'), on: vi.fn() },
  BrowserWindow: { getAllWindows: vi.fn(() => []) },
  shell: { openPath: vi.fn() },
}));
vi.mock('electron-store', () => ({ default: class {} }));

import { normalise } from './settings';

describe('normalise navigation settings', () => {
  it('defaults navigation.style to vim for settings files written before the section existed', () => {
    expect(normalise({ appearance: { theme: 'dark' } }).navigation).toEqual({
      style: 'vim',
    });
  });

  it.each(['vim', 'emacs', 'standard'])('keeps the valid %s style', (style) => {
    expect(normalise({ navigation: { style } }).navigation.style).toBe(style);
  });

  it('drops an invalid style back to the default', () => {
    expect(normalise({ navigation: { style: 'nano' } }).navigation.style).toBe(
      'vim',
    );
  });
});
