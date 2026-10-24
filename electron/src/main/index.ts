import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import {
  err,
  Fault,
  fromThrowable,
  ok,
  type Result,
  ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { app, BrowserWindow, dialog, ipcMain, shell } from 'electron';
import log from 'electron-log';
import { z } from 'zod';
import type { EngineStatus } from '../types/engine';
import type { ClassificationSettings, SyncSettings } from '../types/settings';
import { DEFAULT_SETTINGS } from '../types/settings';
import { registerEngineIPC } from './engineIpc';
import {
  type GoSpawnOptions,
  killGoProcess,
  restartGoProcess,
  spawnGoProcess,
} from './go-spawn';
import { setupIPC } from './ipc';
import {
  captureSentryException,
  configureSentryConsent,
  isSentryEnabled,
} from './sentry';
import { SettingsService } from './settings';
import { resolveTelemetryEndpoint, TelemetryService } from './telemetry';
import type { AppConfig } from './types';
import { startAutoUpdates, stopAutoUpdates } from './updater';
import { WebSocketClient } from './websocket-client';
import { createWindow } from './window';

const isDev = process.env.NODE_ENV === 'development' || !app.isPackaged;

/** Minimum and maximum sync interval in seconds (mirrors UI slider bounds). */
const SYNC_INTERVAL_MIN_SECS = 30;
const SYNC_INTERVAL_MAX_SECS = 3600;

type GoSyncSettings = Pick<SyncSettings, 'autoSync' | 'syncInterval'>;

/**
 * Normalise sync settings before using them for Go startup flags or runtime
 * WebSocket updates. Falls back to DEFAULT_SETTINGS for invalid values so a
 * hand-edited settings.json cannot produce invalid process flags.
 */
function normaliseSyncForGo(sync: SyncSettings): GoSyncSettings {
  const autoSync =
    typeof sync.autoSync === 'boolean'
      ? sync.autoSync
      : DEFAULT_SETTINGS.sync.autoSync;

  const rawInterval = sync.syncInterval;
  const syncInterval =
    typeof rawInterval === 'number' &&
    Number.isFinite(rawInterval) &&
    rawInterval > 0
      ? Math.min(
          SYNC_INTERVAL_MAX_SECS,
          Math.max(SYNC_INTERVAL_MIN_SECS, rawInterval),
        )
      : DEFAULT_SETTINGS.sync.syncInterval;

  return { autoSync, syncInterval };
}

function syncSettingsPayload(sync: SyncSettings): {
  auto_sync: boolean;
  sync_interval_secs: number;
} {
  const normalised = normaliseSyncForGo(sync);
  return {
    auto_sync: normalised.autoSync,
    sync_interval_secs: normalised.syncInterval,
  };
}

/**
 * Wire payload for SET_CLASSIFICATION_SETTINGS (Phase 3 spec contract).
 * Never includes any API key — BYOK keys travel only via SET_BYOK_KEY into
 * the OS keychain, never through settings.
 */
function classificationSettingsPayload(c: ClassificationSettings): {
  mode: string;
  dismissal_scope: string;
  provider: string;
  model: string;
  endpoint: string;
} {
  return {
    mode: c.mode,
    dismissal_scope: c.dismissalScope,
    provider: c.provider,
    model: c.model,
    endpoint: c.endpoint,
  };
}

/**
 * Resolve paths for data storage.
 * Dev: uses <project>/go/ directory (where the binary lives)
 * Prod: uses app.getPath('userData') for stable, writable location
 */
// app.getAppPath() → electron/ in dev, app.asar in prod
// process.resourcesPath → only defined in packaged app
const ELECTRON_ROOT = app.getAppPath();
const PROJECT_ROOT = app.isPackaged
  ? process.resourcesPath
  : join(ELECTRON_ROOT, '..');

function resolveDataDir(): string {
  if (isDev) {
    // In dev, store data alongside the Go binary
    return join(PROJECT_ROOT, 'go');
  }
  return app.getPath('userData');
}

function resolveGoBinaryPath(): string {
  if (isDev) {
    // In dev, the Go binary is at <project>/go/bistro-sync
    return join(PROJECT_ROOT, 'go', 'bistro-sync');
  }
  // In prod, binary is bundled alongside the app
  const binaryName =
    process.platform === 'win32' ? 'bistro-sync.exe' : 'bistro-sync';
  return join(process.resourcesPath, 'go-bin', binaryName);
}

const DEFAULT_WINDOW_WIDTH = 1600;
const DEFAULT_WINDOW_HEIGHT = 900;
const MAX_WS_PORT = 65535;
const goLogLevelSchema = z.enum(['debug', 'info', 'warn', 'error']);

function normaliseLogLevel(
  raw: string | undefined,
): z.infer<typeof goLogLevelSchema> {
  if (raw === undefined) return isDev ? 'debug' : 'info';
  const parsed = goLogLevelSchema.safeParse(raw);
  return parsed.success ? parsed.data : 'info';
}

function normaliseWindowDimension(
  raw: string | undefined,
  fallback: number,
): number {
  if (raw === undefined) return fallback;
  const parsed = z.coerce.number().int().positive().safeParse(raw);
  return parsed.success ? parsed.data : fallback;
}

function mainFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause : String(cause))
    .withTag('INTERNAL_ERROR')
    .withCause(cause);
}
function isValidStartupPort(port: number): boolean {
  return Number.isInteger(port) && port > 0 && port <= MAX_WS_PORT;
}
const startupEnvironmentSchema = z.object({
  WS_PORT: z.coerce.number().int().min(0).max(MAX_WS_PORT).default(0),
  APP_WINDOW_WIDTH: z.number().int().positive(),
  APP_WINDOW_HEIGHT: z.number().int().positive(),
  GO_LOG_LEVEL: goLogLevelSchema,
});

function buildConfig(
  sync: SyncSettings = DEFAULT_SETTINGS.sync,
): Result<AppConfig, Fault> {
  const parsed = startupEnvironmentSchema.safeParse({
    WS_PORT: process.env.WS_PORT ?? undefined,
    APP_WINDOW_WIDTH: normaliseWindowDimension(
      process.env.APP_WINDOW_WIDTH,
      DEFAULT_WINDOW_WIDTH,
    ),
    APP_WINDOW_HEIGHT: normaliseWindowDimension(
      process.env.APP_WINDOW_HEIGHT,
      DEFAULT_WINDOW_HEIGHT,
    ),
    GO_LOG_LEVEL: normaliseLogLevel(process.env.GO_LOG_LEVEL),
  });
  if (!parsed.success) {
    return err(
      ServiceError(
        'CONFIGURATION_ERROR',
        `Invalid startup configuration: ${parsed.error.issues.map((issue) => issue.path.join('.')).join(', ')}`,
      ).withCause(parsed.error),
    );
  }
  const dataDir = resolveDataDir();
  const logsDir = join(dataDir, 'logs');
  return fromThrowable(
    () => {
      mkdirSync(logsDir, { recursive: true });
      return {
        windowWidth: parsed.data.APP_WINDOW_WIDTH,
        windowHeight: parsed.data.APP_WINDOW_HEIGHT,
        goProcess: {
          binaryPath: resolveGoBinaryPath(),
          dbPath: join(dataDir, 'sync.db'),
          wsPort: parsed.data.WS_PORT,
          logLevel: parsed.data.GO_LOG_LEVEL,
          logFile: join(logsDir, 'bistro-sync.log'),
          replayQueue: true,
          ...normaliseSyncForGo(sync),
        },
      } satisfies AppConfig;
    },
    (cause) => mainFault(cause).withTag('FILE_SYSTEM_ERROR'),
  )();
}

log.info('Starting Bistro Electron app');
log.info(`ELECTRON_ROOT: ${ELECTRON_ROOT}`);
log.info(`PROJECT_ROOT: ${PROJECT_ROOT}`);
log.info(`Mode: ${isDev ? 'development' : 'production'}`);

let wsClient: WebSocketClient | null = null;
let settingsService: SettingsService | null = null;
let telemetryService: TelemetryService | null = null;
let runtimeConfig: AppConfig | null = null;

ipcMain.handle('app:quit', () => {
  app.quit();
  return { ok: true as const };
});

ipcMain.handle('open-external', (_event, url: unknown) => {
  if (typeof url !== 'string' || !url.startsWith('https://')) {
    return { err: 'Only https:// URLs are allowed' };
  }
  return ResultAsync.fromPromise(shell.openExternal(url), (cause) =>
    mainFault(cause).withTag('EXTERNAL_ERROR'),
  ).match(
    () => ({ ok: true as const }),
    (error) => {
      log.error(`open-external failed: ${error.message}`);
      return { err: error.message };
    },
  );
});

function initialiseMain(): Result<AppConfig, Fault> {
  return fromThrowable(() => new SettingsService(log), mainFault)()
    .andThen((settings) => settings.loadResult().map(() => settings))
    .andThen((settings) =>
      buildConfig(settings.snapshot.sync).andThen((config) =>
        fromThrowable(() => {
          log.info(`Settings loaded from ${settings.path}`);
          startAutoUpdates(log);
          configureSentryConsent(settings.snapshot.privacy.telemetryEnabled);
          log.info(
            `Sentry crash reporting: ${isSentryEnabled() ? 'enabled' : 'disabled'}`,
          );
          telemetryService = new TelemetryService({
            enabled: settings.snapshot.privacy.telemetryEnabled,
            storageDir: join(app.getPath('userData'), 'telemetry'),
            endpoint: resolveTelemetryEndpoint(
              process.env.BISTRO_TELEMETRY_ENDPOINT,
            ),
            appVersion: app.getVersion(),
            platform: process.platform,
            logger: log,
          });
          void telemetryService.trackAppStarted();
          settingsService = settings;
          runtimeConfig = config;
          setupIPC(
            config.goProcess.dbPath,
            log,
            () => wsClient,
            settings,
            () => runtimeConfig?.goProcess.wsPort ?? null,
            telemetryService,
          );
          return config;
        }, mainFault)(),
      ),
    );
}

app.on('ready', () => {
  const initialised = initialiseMain();
  if (initialised.isErr()) {
    log.error(`Failed to initialise app: ${initialised.error.message}`);
    dialog.showErrorBox('Bistro could not start', initialised.error.message);
    app.quit();
    return;
  }
  const appConfig = initialised.value;
  log.info(`Go binary: ${appConfig.goProcess.binaryPath}`);
  log.info(`DB path: ${appConfig.goProcess.dbPath}`);
  log.info(`Log file: ${appConfig.goProcess.logFile}`);
  log.info(`WS port: ${appConfig.goProcess.wsPort}`);

  let engineStatus: EngineStatus = {
    state: 'starting',
    startedAt: Date.now(),
    isSpawned: process.env.BISTRO_SKIP_GO_SPAWN !== '1',
    code: null,
    signal: null,
    error: null,
  };
  const publishEngineStatus = (patch: Partial<EngineStatus>): void => {
    engineStatus = { ...engineStatus, ...patch };
    for (const win of BrowserWindow.getAllWindows())
      win.webContents.send('engine:status', engineStatus);
  };
  const spawnOptions: GoSpawnOptions = {
    config: appConfig.goProcess,
    logger: log,
    onStarting: () =>
      publishEngineStatus({
        state: 'starting',
        startedAt: Date.now(),
        error: null,
      }),
    onReady: () =>
      publishEngineStatus({
        state: 'ready',
        code: null,
        signal: null,
        error: null,
      }),
    onGiveUp: ({ code, signal }) =>
      publishEngineStatus({
        state: 'crashed',
        code,
        signal,
        error: `Engine exited: ${signal ?? code ?? 'unknown'}`,
      }),
    onStillStarting: () => {
      fromThrowable(
        () => createWindow(runtimeConfig ?? appConfig, log),
        mainFault,
      )().orInspect((error) => {
        log.error('Failed to show slow-start window', error.message);
        app.quit();
      });
    },
    onCrash: (code, signal) => {
      publishEngineStatus({
        state: 'starting',
        code,
        signal,
        startedAt: Date.now(),
      });
      log.error(`Go process crashed: code=${code}, signal=${signal}`);
      captureSentryException(
        ServiceError('GO_PROCESS_ERROR', 'Go sync engine crashed'),
        {
          tags: { component: 'go-sync-engine' },
          extra: { code, signal },
        },
      );
      BrowserWindow.getAllWindows().forEach((win) => {
        if (!win.isDestroyed()) {
          win.webContents.send('go:crashed', { code, signal });
        }
      });
    },
  };
  const connectMainToEngine = (actualWsPort: number): void => {
    if (!isValidStartupPort(actualWsPort)) {
      log.error(
        `Go sync engine reported invalid WebSocket port: ${actualWsPort}`,
      );
      app.quit();
      return;
    }
    log.info(`Go sync engine is running on port ${actualWsPort}`);

    runtimeConfig = {
      ...appConfig,
      goProcess: { ...appConfig.goProcess, wsPort: actualWsPort },
    };

    // 2. Create WebSocket client and connect (Go is now ready)
    // 127.0.0.1, not localhost: the engine binds IPv4 loopback only and
    // localhost may resolve to ::1 first.
    const wsUrl = `ws://127.0.0.1:${actualWsPort}/ws`;
    wsClient = new WebSocketClient(wsUrl, log);
    const activeSettingsService = settingsService;
    if (activeSettingsService === null) {
      log.error('Settings unavailable after engine startup');
      app.quit();
      return;
    }

    const sendSyncSettingsToGo = (sync: SyncSettings) => {
      if (!wsClient?.isConnected()) return;
      wsClient
        .send({
          type: 'SET_SYNC_SETTINGS',
          data: syncSettingsPayload(sync),
        })
        .orInspect((error) =>
          log.warn(`Sync settings update failed: ${error.message}`),
        );
    };
    const sendClassificationSettingsToGo = (c: ClassificationSettings) => {
      if (!wsClient?.isConnected()) return;
      wsClient
        .send({
          type: 'SET_CLASSIFICATION_SETTINGS',
          data: classificationSettingsPayload(c),
        })
        .orInspect((error) =>
          log.warn(`Classification settings update failed: ${error.message}`),
        );
    };
    wsClient.on('connected', () => {
      // Bootstrap-on-connect: Go starts with defaults; push live settings
      // on every (re)connect per the Phase 3 wire contract.
      sendSyncSettingsToGo(activeSettingsService.snapshot.sync);
      sendClassificationSettingsToGo(
        activeSettingsService.snapshot.classification,
      );
    });
    wsClient.connect();

    // 3. Forward runtime sync-settings changes to Go engine over WebSocket.
    activeSettingsService.addChangeListener((s) => {
      sendSyncSettingsToGo(s.sync);
      sendClassificationSettingsToGo(s.classification);
      telemetryService?.setEnabled(s.privacy.telemetryEnabled);
      configureSentryConsent(s.privacy.telemetryEnabled);
    });
  };
  registerEngineIPC({
    logFile: appConfig.goProcess.logFile,
    getStatus: () => engineStatus,
    restart: () =>
      ResultAsync.fromPromise(restartGoProcess(spawnOptions), mainFault)
        .orInspect((fault) =>
          publishEngineStatus({ state: 'crashed', error: fault.message }),
        )
        .map((port) => {
          if (wsClient === null) {
            connectMainToEngine(port);
            for (const win of BrowserWindow.getAllWindows())
              win.webContents.reload();
          }
          return undefined;
        }),
  });
  void ResultAsync.fromPromise(spawnGoProcess(spawnOptions), (cause) =>
    mainFault(cause).withTag('GO_PROCESS_ERROR'),
  ).match(
    (port) => {
      connectMainToEngine(port);
      fromThrowable(
        () => createWindow(runtimeConfig ?? appConfig, log),
        mainFault,
      )().orInspect((error) => {
        log.error('Failed to create window', error.message);
        app.quit();
      });
    },
    (error) => {
      publishEngineStatus({ state: 'crashed', error: error.message });
      log.error(`Failed to start app: ${error.message}`);
      captureSentryException(
        ServiceError('INTERNAL_ERROR', 'Electron main startup failed'),
        {
          tags: { component: 'electron-main' },
        },
      );
      fromThrowable(
        () => createWindow(runtimeConfig ?? appConfig, log),
        mainFault,
      )().orInspect((error) => {
        log.error('Failed to create fallback window', error.message);
        app.quit();
      });
    },
  );
});

app.on('before-quit', () => {
  log.info('App shutting down');
  stopAutoUpdates();
  wsClient?.stop();
  settingsService?.stop();
  killGoProcess();
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    const config = runtimeConfig === null ? buildConfig() : ok(runtimeConfig);
    config.match(
      (value) =>
        fromThrowable(() => createWindow(value, log), mainFault)().orInspect(
          (error) => log.error('Failed to activate window', error.message),
        ),
      (error) => log.error(`Failed to activate app: ${error.message}`),
    );
  }
});

log.info('Main process initialized');
