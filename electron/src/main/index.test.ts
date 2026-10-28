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
  config: {} as Record<string, unknown>,
  connected: (): void => undefined,
  settingsChanged: (_settings: typeof DEFAULT_SETTINGS): void => undefined,
  sent: [] as Record<string, unknown>[],
  telemetryCreated: vi.fn(),
  sentryConsent: vi.fn(),
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
    snapshot = {
      ...DEFAULT_SETTINGS,
      privacy: { ...DEFAULT_SETTINGS.privacy, telemetryEnabled: true },
    };
    path = '/unused';
    loadResult = () => ok(undefined);
    addChangeListener = (
      listener: (settings: typeof DEFAULT_SETTINGS) => void,
    ) => {
      startup.settingsChanged = listener;
    };
  },
}));
vi.mock('./telemetry', () => ({
  resolveTelemetryEndpoint: () => null,
  TelemetryService: class {
    constructor() {
      startup.telemetryCreated();
    }
    trackAppStarted = vi.fn();
    setEnabled = vi.fn();
  },
}));
vi.mock('./sentry', () => ({
  configureSentryConsent: startup.sentryConsent,
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
  spawnGoProcess: (options: {
    config: Record<string, unknown>;
    onStillStarting: () => void;
  }) => {
    startup.config = options.config;
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
    on = (event: string, listener: () => void) => {
      if (event === 'connected') startup.connected = listener;
    };
    isConnected = () => true;
    send = (message: Record<string, unknown>) => {
      startup.sent.push(message);
      return ok(undefined);
    };
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

it('forces isolated engine and rules-only settings on first connect, reconnect and settings changes in keys benchmark', async () => {
  vi.resetModules();
  const query =
    'bench_keys=1&bench_collector=http://localhost:9000&bench_keys_pr=github:bench/large-diff:1';
  const originalArgv = process.argv;
  process.argv = [`--bench-query=${encodeURIComponent(query)}`];
  Object.defineProperty(process, 'resourcesPath', {
    value: '/unused',
    configurable: true,
  });
  startup.port = 0;
  startup.windowExists = false;
  startup.telemetryCreated.mockClear();
  startup.sentryConsent.mockClear();
  vi.stubEnv('WS_PORT', '8080');
  await import('./index');
  startup.ready();
  expect(startup.config).toMatchObject({
    wsPort: 0,
    autoSync: false,
    replayQueue: false,
  });
  startup.showSlowWindow();
  expect(startup.windowExists).toBe(false);
  expect(startup.telemetryCreated).not.toHaveBeenCalled();
  expect(startup.sentryConsent).toHaveBeenCalledExactlyOnceWith(false);
  startup.sent.length = 0;
  startup.settlePort(64568);
  await vi.waitFor(() => expect(startup.port).toBe(64568));
  startup.connected();
  startup.connected();
  startup.settingsChanged({
    ...DEFAULT_SETTINGS,
    classification: { ...DEFAULT_SETTINGS.classification, mode: 'rules+llm' },
    privacy: { ...DEFAULT_SETTINGS.privacy, telemetryEnabled: true },
  });
  expect(startup.telemetryCreated).not.toHaveBeenCalled();
  expect(startup.sentryConsent.mock.calls).toEqual([[false], [false]]);
  expect(startup.sent).toHaveLength(3);
  for (const message of startup.sent) {
    expect(message).toEqual({
      type: 'SET_CLASSIFICATION_SETTINGS',
      data: {
        mode: 'rules',
        dismissal_scope: 'local',
        provider: '',
        model: '',
        endpoint: '',
      },
    });
  }
  process.argv = originalArgv;
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

it('reconnects the existing slow-start renderer when initial readiness arrives after port discovery stops', async () => {
  vi.resetModules();
  startup.port = 0;
  startup.rendererPort = 0;
  startup.windowExists = false;
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
