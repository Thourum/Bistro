import { join } from 'node:path';
import { BrowserWindow } from 'electron';
import type { Logger } from 'electron-log';
import type { AppConfig } from './types';

declare const MAIN_WINDOW_VITE_DEV_SERVER_URL: string | undefined;
declare const MAIN_WINDOW_VITE_NAME: string;

let mainWindow: BrowserWindow | null = null;

export function createWindow(config: AppConfig, logger: Logger): BrowserWindow {
  mainWindow = new BrowserWindow({
    width: config.windowWidth,
    height: config.windowHeight,
    webPreferences: {
      preload: join(__dirname, 'preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
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
