import { beforeEach, describe, expect, it, vi } from 'vitest';

const electronMock = vi.hoisted(() => ({ app: { isPackaged: true } }));
const updaterMock = vi.hoisted(() => ({
  makeUserNotifier: vi.fn((options: unknown) => options),
  updateElectronApp: vi.fn(() => ({ stopUpdates: vi.fn() })),
  UpdateSourceType: { ElectronPublicUpdateService: 'public' },
}));

vi.mock('electron', () => electronMock);
vi.mock('update-electron-app', () => updaterMock);

const logger = {
  error: vi.fn(),
  info: vi.fn(),
  warn: vi.fn(),
} as never;

describe('auto updates', () => {
  beforeEach(() => {
    vi.resetModules();
    updaterMock.updateElectronApp.mockClear();
    updaterMock.makeUserNotifier.mockClear();
    electronMock.app.isPackaged = true;
    delete process.env.BISTRO_ENABLE_AUTO_UPDATES;
    delete process.env.BISTRO_DISABLE_AUTO_UPDATES;
  });

  it('does not start outbound update checks in packaged builds by default', async () => {
    const { startAutoUpdates } = await import('../../src/main/updater');

    startAutoUpdates(logger);

    expect(updaterMock.updateElectronApp).not.toHaveBeenCalled();
  });

  it('starts update checks only after explicit environment opt-in', async () => {
    process.env.BISTRO_ENABLE_AUTO_UPDATES = '1';
    const { startAutoUpdates } = await import('../../src/main/updater');

    startAutoUpdates(logger);

    expect(updaterMock.updateElectronApp).toHaveBeenCalledTimes(1);
  });

  it('honors the explicit disable override', async () => {
    process.env.BISTRO_ENABLE_AUTO_UPDATES = '1';
    process.env.BISTRO_DISABLE_AUTO_UPDATES = '1';
    const { startAutoUpdates } = await import('../../src/main/updater');

    startAutoUpdates(logger);

    expect(updaterMock.updateElectronApp).not.toHaveBeenCalled();
  });
});
