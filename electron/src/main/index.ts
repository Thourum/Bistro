import { app } from 'electron';
import log from 'electron-log';
import { createWindow } from './window';
import { setupIPC, closeDatabase } from './ipc';
import { spawnGoProcess, killGoProcess } from './go-spawn';
import { WebSocketClient } from './websocket-client';
import type { AppConfig } from './types';

const isDev = process.env.NODE_ENV === 'development';

const appConfig: AppConfig = {
  windowWidth: parseInt(process.env.APP_WINDOW_WIDTH || '1600'),
  windowHeight: parseInt(process.env.APP_WINDOW_HEIGHT || '900'),
  goProcess: {
    binaryPath: process.env.GO_BINARY_PATH || './go-bin/bistro-sync',
    dbPath: process.env.GO_DB_PATH || './data/bistro.db',
    wsPort: parseInt(process.env.WS_PORT || '8080'),
  },
};

log.info('Starting Bistro Electron app');

let wsClient: WebSocketClient | null = null;

// Handle app ready
app.on('ready', async () => {
  try {
    // 1. Create WebSocket client (connects after Go process starts)
    const wsUrl = `ws://localhost:${appConfig.goProcess.wsPort}/ws`;
    wsClient = new WebSocketClient(wsUrl, log);

    // 2. Setup IPC handlers, passing WebSocket client
    setupIPC(appConfig.goProcess.dbPath, log, wsClient);

    // 3. Spawn Go process
    spawnGoProcess(appConfig.goProcess, log);

    // 4. Connect WebSocket after a short delay to allow Go to start
    setTimeout(() => {
      wsClient?.connect();
    }, 1000);

    // 5. Broadcast sync events to renderer
    wsClient.on('event', (event) => {
      const windows = require('electron').BrowserWindow.getAllWindows();
      windows.forEach((win: Electron.BrowserWindow) => {
        if (!win.isDestroyed()) {
          win.webContents.send('sync:event', event);
        }
      });
    });

    // 6. Create window
    createWindow(appConfig, log);
  } catch (error) {
    log.error('Failed to start app', error);
    app.quit();
  }
});

// Handle app quit
app.on('before-quit', () => {
  log.info('App shutting down');
  wsClient?.stop();
  killGoProcess();
  closeDatabase();
});

// Handle window all closed
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

// Activate on macOS
app.on('activate', () => {
  // TODO: Recreate window if needed
});

log.info('Main process initialized');
