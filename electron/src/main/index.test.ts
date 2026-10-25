import { ok } from '@thourum/fault';
import { expect, it, vi } from 'vitest';
import { DEFAULT_SETTINGS } from '../types/settings';

const startup = vi.hoisted(() => ({
  ready: (): void => undefined,
  port: 0,
  rendererPort: 0,
  windowExists: false,
  settlePort: (_port: number): void => undefined,
  showSlowWindow: (): void => undefined,
}));
const window = vi.hoisted(() => ({
  webContents: {
    send: vi.fn(),
    reload: () => {
      startup.rendererPort = startup.port;
    },
  },
}));
vi.mock('electron', () => ({
  app: {
    isPackaged: true,
    getAppPath: () => '/unused',
    getPath: () => '/unused',
    getVersion: () => 'test',
    on: (event: string, listener: () => void) => {
      if (event === 'ready') startup.ready = listener;
    },
    quit: vi.fn(),
  },
  BrowserWindow: {
    getAllWindows: () => (startup.windowExists ? [window] : []),
  },
  ipcMain: { handle: vi.fn() },
  dialog: { showErrorBox: vi.fn() },
  shell: {},
}));
vi.mock('node:fs', () => ({ mkdirSync: vi.fn() }));
vi.mock('electron-log', () => ({
  default: { info: vi.fn(), warn: vi.fn(), error: vi.fn() },
}));
vi.mock('./settings', () => ({
  SettingsService: class {
    snapshot = DEFAULT_SETTINGS;
    path = '/unused';
    loadResult = () => ok(undefined);
    addChangeListener = vi.fn();
  },
}));
vi.mock('./telemetry', () => ({
  resolveTelemetryEndpoint: () => null,
  TelemetryService: class {
    trackAppStarted = vi.fn();
  },
}));
vi.mock('./sentry', () => ({
  configureSentryConsent: vi.fn(),
  isSentryEnabled: () => false,
  captureSentryException: vi.fn(),
}));
vi.mock('./updater', () => ({
  startAutoUpdates: vi.fn(),
  stopAutoUpdates: vi.fn(),
}));
vi.mock('./ipc', () => ({ setupIPC: vi.fn() }));
vi.mock('./engineIpc', () => ({ registerEngineIPC: vi.fn() }));
vi.mock('./go-spawn', () => ({
  spawnGoProcess: (options: { onStillStarting: () => void }) => {
    startup.showSlowWindow = options.onStillStarting;
    return new Promise<number>((resolve) => {
      startup.settlePort = resolve;
    });
  },
  restartGoProcess: vi.fn(),
  killGoProcess: vi.fn(),
}));
vi.mock('./websocket-client', () => ({
  WebSocketClient: class {
    constructor(url: string) {
      startup.port = Number(new URL(url).port);
    }
    on = vi.fn();
    connect = vi.fn();
  },
}));
vi.mock('./window', () => ({
  createWindow: () => {
    if (!startup.windowExists) {
      startup.windowExists = true;
      startup.rendererPort = startup.port;
    }
    return window;
  },
}));

it('reconnects the existing slow-start renderer when initial readiness arrives after port discovery stops', async () => {
  vi.stubEnv('WS_PORT', '0');
  Object.defineProperty(process, 'resourcesPath', {
    value: '/unused',
    configurable: true,
  });
  await import('./index');
  startup.ready();
  startup.showSlowWindow();
  expect(startup.rendererPort).toBe(0);
  const enginePort = 64568;
  startup.settlePort(enginePort);
  await vi.waitFor(() => expect(startup.port).toBe(enginePort));
  expect(startup.rendererPort).toBe(enginePort);
  vi.unstubAllEnvs();
});
