import { join } from 'node:path';
import { app, BrowserWindow } from 'electron';
import type { Logger } from 'electron-log';
import type { AppConfig } from './types';
import { wsPortToArg } from './ws-port';

declare const MAIN_WINDOW_VITE_DEV_SERVER_URL: string | undefined;
declare const MAIN_WINDOW_VITE_NAME: string;

let mainWindow: BrowserWindow | null = null;

export function createWindow(config: AppConfig, logger: Logger): BrowserWindow {
  mainWindow = new BrowserWindow({
    width: config.windowWidth,
    height: config.windowHeight,
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'hidden',
    trafficLightPosition: { x: 14, y: 10 },
    titleBarOverlay: {
      color: '#f8f9fa',
      symbolColor: '#191c1d',
      height: 32,
    },
    webPreferences: {
      preload: join(__dirname, 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      additionalArguments: [wsPortToArg(config.goProcess.wsPort)],
    },
  });

  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));

  mainWindow.webContents.on('before-input-event', (event, input) => {
    const key = input.key.toLowerCase();
    const isMacQuit =
      process.platform === 'darwin' && input.meta && key === 'q';
    const isWindowsQuit =
      process.platform === 'win32' && input.alt && key === 'f4';

    if (isMacQuit || isWindowsQuit) {
      event.preventDefault();
      app.quit();
    }
  });

  mainWindow.webContents.on('will-navigate', (event, url) => {
    const devServerUrl = MAIN_WINDOW_VITE_DEV_SERVER_URL;
    if (
      devServerUrl !== undefined &&
      (devServerUrl.startsWith('http://') ||
        devServerUrl.startsWith('https://')) &&
      (url.startsWith('http://') || url.startsWith('https://'))
    ) {
      const devOrigin = new URL(devServerUrl).origin;
      const navOrigin = new URL(url).origin;
      if (navOrigin === devOrigin) {
        return;
      }
    }
    event.preventDefault();
  });

  if (MAIN_WINDOW_VITE_DEV_SERVER_URL) {
    mainWindow.loadURL(MAIN_WINDOW_VITE_DEV_SERVER_URL);
    mainWindow.webContents.openDevTools();
  } else {
    mainWindow.loadFile(
      join(__dirname, `../renderer/${MAIN_WINDOW_VITE_NAME}/index.html`),
    );
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  logger.info('Main window created');
  return mainWindow;
}

export function getMainWindow(): BrowserWindow | null {
  return mainWindow;
}
