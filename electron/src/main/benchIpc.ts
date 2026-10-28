import { ipcMain } from 'electron';
import log from 'electron-log';
import { isKeysBenchSearch, parseBenchSearch } from './benchArgs';

declare const MAIN_WINDOW_VITE_DEV_SERVER_URL: string | undefined;
const devSearch =
  typeof MAIN_WINDOW_VITE_DEV_SERVER_URL !== 'undefined' &&
  MAIN_WINDOW_VITE_DEV_SERVER_URL
    ? new URL(MAIN_WINDOW_VITE_DEV_SERVER_URL).search
    : '';
const benchSearch = parseBenchSearch(process.argv, devSearch);
const IS_KEYS_BENCH =
  benchSearch.isErr() || isKeysBenchSearch(benchSearch.value);
const EXTERNAL_CHANNELS = new Set([
  'app:quit',
  'open-external',
  'open-in-editor',
  'engine:open-log-folder',
  'engine:report-crash',
  'settings:open-file',
  'dialog:pick-folder',
]);

/** Registers IPC with a latched benchmark refusal before any external launch. */
export function registerMainIpc(
  channel: string,
  handler: Parameters<typeof ipcMain.handle>[1],
): void {
  ipcMain.handle(channel, (event, ...args) => {
    if (IS_KEYS_BENCH && EXTERNAL_CHANNELS.has(channel)) {
      const message = `Keys benchmark blocked ${channel}`;
      log.warn(message);
      return { err: message };
    }
    return handler(event, ...args);
  });
}
