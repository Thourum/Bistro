import { app } from 'electron';
import type { Logger } from 'electron-log';
import {
  type IUpdateElectronApp,
  makeUserNotifier,
  UpdateSourceType,
  updateElectronApp,
} from 'update-electron-app';

const BISTRO_UPDATE_REPO = 'Itterno/Bistro';

let updater: IUpdateElectronApp | null = null;

export function startAutoUpdates(logger: Logger): void {
  if (!app.isPackaged) {
    logger.info('Auto-update disabled outside packaged app');
    return;
  }

  if (process.env.BISTRO_ENABLE_AUTO_UPDATES !== '1') {
    logger.info('Auto-update disabled by default');
    return;
  }

  if (process.env.BISTRO_DISABLE_AUTO_UPDATES === '1') {
    logger.info('Auto-update disabled by BISTRO_DISABLE_AUTO_UPDATES');
    return;
  }

  updater = updateElectronApp({
    updateSource: {
      type: UpdateSourceType.ElectronPublicUpdateService,
      repo: BISTRO_UPDATE_REPO,
    },
    updateInterval: process.env.BISTRO_UPDATE_INTERVAL ?? '1 hour',
    logger,
    notifyUser: true,
    onNotifyUser: makeUserNotifier({
      title: 'Bistro update ready',
      detail:
        'A new Bistro version has been downloaded. Restart Bistro to finish updating.',
      restartButtonText: 'Restart Bistro',
      laterButtonText: 'Later',
    }),
  });

  logger.info(`Auto-update enabled via GitHub releases: ${BISTRO_UPDATE_REPO}`);
}

export function stopAutoUpdates(): void {
  updater?.stopUpdates();
  updater = null;
}
