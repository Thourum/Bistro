import { BrowserWindow } from 'electron';
const isDev = process.env.NODE_ENV === 'development';
import { join } from 'path';
import { Logger } from 'electron-log';
import type { AppConfig } from './types';

let mainWindow: BrowserWindow | null = null;

export function createWindow(config: AppConfig, logger: Logger): BrowserWindow {
  mainWindow = new BrowserWindow({
    width: config.windowWidth,
    height: config.windowHeight,
    webPreferences: {
      preload: join(__dirname, '../main/preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  });

  // __dirname at runtime = dist/main/main/
  // renderer is at dist/renderer/index.html (3 levels up then into renderer)
  const startUrl = isDev
    ? 'http://localhost:5173'
    : `file://${join(__dirname, '..', '..', 'renderer', 'index.html')}`;

  mainWindow.loadURL(startUrl);

  if (isDev) {
    mainWindow.webContents.openDevTools();
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
