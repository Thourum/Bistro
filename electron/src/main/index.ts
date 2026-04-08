import { app } from 'electron';
import { join } from 'path';
import { mkdirSync, existsSync } from 'fs';
import log from 'electron-log';
import { createWindow } from './window';
import { setupIPC, closeDatabase } from './ipc';
import { spawnGoProcess, killGoProcess } from './go-spawn';
import { WebSocketClient } from './websocket-client';
import type { AppConfig } from './types';

const isDev = process.env.NODE_ENV === 'development' || !app.isPackaged;

/**
 * Resolve paths for data storage.
 * Dev: uses <project>/go/ directory (where the binary lives)
 * Prod: uses app.getPath('userData') for stable, writable location
 */
/**
 * __dirname at runtime = dist/main/main/
 * Project root (bistro/) = 4 levels up: main/ → main/ → dist/ → electron/ → bistro/
 * Electron root (electron/) = 3 levels up: main/ → main/ → dist/ → electron/
 */
const ELECTRON_ROOT = join(__dirname, '..', '..', '..');
const PROJECT_ROOT = join(ELECTRON_ROOT, '..');

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

function buildConfig(): AppConfig {
  const dataDir = resolveDataDir();
  const logsDir = join(dataDir, 'logs');

  // Ensure directories exist
  if (!existsSync(dataDir)) {
    mkdirSync(dataDir, { recursive: true });
  }
  if (!existsSync(logsDir)) {
    mkdirSync(logsDir, { recursive: true });
  }

  const wsPort = parseInt(process.env.WS_PORT || '8080');

  return {
    windowWidth: parseInt(process.env.APP_WINDOW_WIDTH || '1600'),
    windowHeight: parseInt(process.env.APP_WINDOW_HEIGHT || '900'),
    goProcess: {
      binaryPath: resolveGoBinaryPath(),
      dbPath: join(dataDir, 'sync.db'),
      wsPort,
      logLevel: (process.env.GO_LOG_LEVEL || 'debug') as 'debug' | 'info' | 'warn' | 'error',
      logFile: join(logsDir, 'bistro-sync.log'),
      replayQueue: true,
    },
  };
}

log.info('Starting Bistro Electron app');
log.info(`Mode: ${isDev ? 'development' : 'production'}`);

let wsClient: WebSocketClient | null = null;

app.on('ready', async () => {
  const appConfig = buildConfig();

  log.info(`Go binary: ${appConfig.goProcess.binaryPath}`);
  log.info(`DB path: ${appConfig.goProcess.dbPath}`);
  log.info(`Log file: ${appConfig.goProcess.logFile}`);
  log.info(`WS port: ${appConfig.goProcess.wsPort}`);

  try {
    // 1. Spawn Go process and wait for readiness
    log.info('Spawning Go sync engine...');
    await spawnGoProcess(appConfig.goProcess, log);
    log.info('Go sync engine is running');

    // 2. Create WebSocket client and connect (Go is now ready)
    const wsUrl = `ws://localhost:${appConfig.goProcess.wsPort}/ws`;
    wsClient = new WebSocketClient(wsUrl, log);
    wsClient.connect();

    // 3. Setup IPC handlers (DB should exist now that Go has started)
    setupIPC(appConfig.goProcess.dbPath, log, wsClient);

    // 4. Broadcast sync events from WebSocket to renderer
    wsClient.on('event', (event) => {
      const { BrowserWindow } = require('electron');
      const windows = BrowserWindow.getAllWindows();
      windows.forEach((win: Electron.BrowserWindow) => {
        if (!win.isDestroyed()) {
          win.webContents.send('sync:event', event);
        }
      });
    });

    // 5. Create window (last — everything is ready)
    createWindow(appConfig, log);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    log.error(`Failed to start app: ${message}`);

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
  wsClient?.stop();
  killGoProcess();
  closeDatabase();
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  const { BrowserWindow } = require('electron');
  if (BrowserWindow.getAllWindows().length === 0) {
    const appConfig = buildConfig();
    createWindow(appConfig, log);
  }
});

log.info('Main process initialized');
