import { join } from 'node:path';
import { app, BrowserWindow } from 'electron';
import type { Logger } from 'electron-log';
import { NATIVE_KEYS_BENCH_ARG } from '../shared/benchSafety';
import { isKeysBenchSearch, parseBenchSearch } from './benchArgs';
import type { AppConfig } from './types';
import { wsPortToArg } from './ws-port';

declare const MAIN_WINDOW_VITE_DEV_SERVER_URL: string | undefined;
declare const MAIN_WINDOW_VITE_NAME: string;

let mainWindow: BrowserWindow | null = null;

export function createWindow(config: AppConfig, logger: Logger): BrowserWindow {
  if (mainWindow !== null && !mainWindow.isDestroyed()) {
    return mainWindow;
  }

  const devSearch = MAIN_WINDOW_VITE_DEV_SERVER_URL
    ? new URL(MAIN_WINDOW_VITE_DEV_SERVER_URL).search
    : '';
  const benchSearch = parseBenchSearch(process.argv, devSearch);
  const isKeysBench =
    benchSearch.isOk() && isKeysBenchSearch(benchSearch.value);

  // Packaged: shipped via Forge extraResource (the asar holds only .vite/).
  const iconPath = app.isPackaged
    ? join(process.resourcesPath, 'icon.png')
    : join(app.getAppPath(), 'assets/icon.png');

  if (process.platform === 'darwin') {
    app.dock?.setIcon(iconPath);
  }

  mainWindow = new BrowserWindow({
    width: config.windowWidth,
    height: config.windowHeight,
    icon: iconPath,
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'hidden',
    trafficLightPosition: { x: 14, y: 10 },
    titleBarOverlay: {
      color: '#f8f9fa',
      symbolColor: '#191c1d',
      height: 32,
    },
    webPreferences: {
      preload: join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      additionalArguments: [
        wsPortToArg(config.goProcess.wsPort),
        ...(isKeysBench ? [NATIVE_KEYS_BENCH_ARG] : []),
      ],
    },
  });

  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));

  // A missing renderer bundle otherwise shows only a white window.
  mainWindow.webContents.on('did-finish-load', () => {
    logger.info('Renderer loaded');
  });
  mainWindow.webContents.on(
    'did-fail-load',
    (_event, errorCode, errorDescription, validatedURL) => {
      logger.error(
        `Renderer failed to load ${validatedURL}: ${errorDescription} (${errorCode})`,
      );
    },
  );

  const search = benchSearch.isOk() ? benchSearch.value : '';
  const validSearch = benchSearch;
  if (validSearch.isErr()) {
    logger.error(validSearch.error.message);
    app.quit();
    return mainWindow;
  }

  mainWindow.webContents.on('before-input-event', (event, input) => {
    const key = input.key.toLowerCase();
    const isMacQuit =
      process.platform === 'darwin' && input.meta && key === 'q';
    const isWindowsQuit =
      process.platform === 'win32' && input.alt && key === 'f4';

    if (isMacQuit || isWindowsQuit) {
      event.preventDefault();
      if (!isKeysBench) app.quit();
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
    const rendererUrl = new URL(MAIN_WINDOW_VITE_DEV_SERVER_URL);
    if (search) {
      rendererUrl.search = search;
    }
    mainWindow.loadURL(rendererUrl.toString());
    mainWindow.webContents.openDevTools();
  } else {
    mainWindow.loadFile(
      join(__dirname, `../renderer/${MAIN_WINDOW_VITE_NAME}/index.html`),
      { search },
    );
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  logger.info('Main window created');
  return mainWindow;
}
