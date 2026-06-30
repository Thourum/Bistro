import { existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { app, ipcMain, shell } from 'electron';
import log from 'electron-log';
import type { SyncSettings } from '../types/settings';
import { DEFAULT_SETTINGS } from '../types/settings';
import { killGoProcess, spawnGoProcess } from './go-spawn';
import { setupIPC } from './ipc';
import { SettingsService } from './settings';
import { resolveTelemetryEndpoint, TelemetryService } from './telemetry';
import {
  captureSentryException,
  configureSentryConsent,
  isSentryEnabled,
} from './sentry';
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
  return join(process.resourcesPath, 'go-bin', 'bistro-sync');
}

function buildConfig(sync: SyncSettings = DEFAULT_SETTINGS.sync): AppConfig {
  const normalisedSync = normaliseSyncForGo(sync);
  const dataDir = resolveDataDir();
  const logsDir = join(dataDir, 'logs');

  // Ensure directories exist
  if (!existsSync(dataDir)) {
    mkdirSync(dataDir, { recursive: true });
  }
  if (!existsSync(logsDir)) {
    mkdirSync(logsDir, { recursive: true });
  }

  // 0 = let Go pick a random available port (reported via its ready signal).
  const wsPort = parseInt(process.env.WS_PORT || '0', 10);
  if (Number.isNaN(wsPort) || wsPort < 0 || wsPort > 65535) {
    throw new Error('WS_PORT must be a number between 0 and 65535');
  }

  const windowWidth = parseInt(process.env.APP_WINDOW_WIDTH || '1600', 10);
  const windowHeight = parseInt(process.env.APP_WINDOW_HEIGHT || '900', 10);

  const GO_LOG_LEVELS = ['debug', 'info', 'warn', 'error'] as const;
  type GoLogLevel = (typeof GO_LOG_LEVELS)[number];
  // Dev defaults to debug so per-PR sync steps are visible when things hang.
  const rawLogLevel = process.env.GO_LOG_LEVEL ?? (isDev ? 'debug' : 'info');
  const logLevel: GoLogLevel = (GO_LOG_LEVELS as readonly string[]).includes(
    rawLogLevel,
  )
    ? (rawLogLevel as GoLogLevel)
    : 'info';

  return {
    windowWidth: Number.isNaN(windowWidth) ? 1600 : windowWidth,
    windowHeight: Number.isNaN(windowHeight) ? 900 : windowHeight,
    goProcess: {
      binaryPath: resolveGoBinaryPath(),
      dbPath: join(dataDir, 'sync.db'),
      wsPort,
      logLevel,
      logFile: join(logsDir, 'bistro-sync.log'),
      replayQueue: true,
      syncInterval: normalisedSync.syncInterval,
      autoSync: normalisedSync.autoSync,
    },
  };
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

app.on('ready', async () => {
  startAutoUpdates(log);

  // Load settings first so we can pass sync config to the Go process.
  settingsService = new SettingsService(log);
  await settingsService.load();
  log.info(`Settings loaded from ${settingsService.path}`);
  configureSentryConsent(settingsService.snapshot.privacy.telemetryEnabled);
  log.info(`Sentry crash reporting: ${isSentryEnabled() ? 'enabled' : 'disabled'}`);
  telemetryService = new TelemetryService({
    enabled: settingsService.snapshot.privacy.telemetryEnabled,
    storageDir: join(app.getPath('userData'), 'telemetry'),
    endpoint: resolveTelemetryEndpoint(process.env.BISTRO_TELEMETRY_ENDPOINT),
    appVersion: app.getVersion(),
    platform: process.platform,
    logger: log,
  });
  void telemetryService.trackAppStarted();

  const appConfig = buildConfig(settingsService.snapshot.sync);

  log.info(`Go binary: ${appConfig.goProcess.binaryPath}`);
  log.info(`DB path: ${appConfig.goProcess.dbPath}`);
  log.info(`Log file: ${appConfig.goProcess.logFile}`);
  log.info(`WS port: ${appConfig.goProcess.wsPort}`);

  try {
    // 1. Spawn Go process and wait for readiness (resolves with actual port)
    log.info('Spawning Go sync engine...');
    const actualWsPort = await spawnGoProcess({
      config: appConfig.goProcess,
      logger: log,
      onCrash: (code, signal) => {
        log.error(`Go process crashed: code=${code}, signal=${signal}`);
        captureSentryException(new Error('Go sync engine crashed'), {
          tags: { component: 'go-sync-engine' },
          extra: { code, signal },
        });
        const { BrowserWindow } = require('electron');
        (BrowserWindow.getAllWindows() as Electron.BrowserWindow[]).forEach(
          (win) => {
            if (!win.isDestroyed()) {
              win.webContents.send('go:crashed', { code, signal });
            }
          },
        );
      },
    });
    log.info(`Go sync engine is running on port ${actualWsPort}`);

    runtimeConfig = {
      ...appConfig,
      goProcess: { ...appConfig.goProcess, wsPort: actualWsPort },
    };

    // 2. Create WebSocket client and connect (Go is now ready)
    const wsUrl = `ws://localhost:${actualWsPort}/ws`;
    wsClient = new WebSocketClient(wsUrl, log);
    const activeSettingsService = settingsService;

    const sendSyncSettingsToGo = (sync: SyncSettings) => {
      if (!wsClient?.isConnected()) return;
      wsClient.send({
        type: 'SET_SYNC_SETTINGS',
        data: syncSettingsPayload(sync),
      });
    };
    wsClient.on('connected', () => {
      sendSyncSettingsToGo(activeSettingsService.snapshot.sync);
    });
    wsClient.connect();

    // 3. Setup IPC handlers (DB should exist now that Go has started)
    setupIPC(
      appConfig.goProcess.dbPath,
      log,
      wsClient,
      settingsService,
      telemetryService,
    );

    // 4a. Forward runtime sync-settings changes to Go engine over WebSocket.
    settingsService.addChangeListener((s) => {
      sendSyncSettingsToGo(s.sync);
      telemetryService?.setEnabled(s.privacy.telemetryEnabled);
      configureSentryConsent(s.privacy.telemetryEnabled);
    });

    // 3b. Register open-external IPC handler (HTTPS-only URL validation)
    ipcMain.handle('open-external', async (_event, url: unknown) => {
      if (typeof url !== 'string' || !url.startsWith('https://')) {
        return { err: 'Only https:// URLs are allowed' };
      }
      try {
        await shell.openExternal(url);
        return { ok: true as const };
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        log.error(`open-external failed: ${message}`);
        return { err: message };
      }
    });

    // 4. Create window (last — everything is ready)
    createWindow(runtimeConfig, log);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    log.error(`Failed to start app: ${message}`);
    captureSentryException(new Error('Electron main startup failed'), {
      tags: { component: 'electron-main' },
    });

    // Still create window so user sees error state
    try {
      createWindow(appConfig, log);
    } catch {
      // If even window creation fails, quit
      app.quit();
    }
  }
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
  const { BrowserWindow } = require('electron');
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow(runtimeConfig ?? buildConfig(), log);
  }
});

log.info('Main process initialized');
