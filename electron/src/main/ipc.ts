import { readFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join, resolve, sep } from 'node:path';
import { dialog, ipcMain } from 'electron';
import type { Logger } from 'electron-log';
import type {
  FSReadPatchRequest,
  GoCommandRequest,
  SyncActionRequest,
} from '../types/ipc';
import type { AppSettings } from '../types/settings';
import type { SettingsService } from './settings';
import type { TelemetryService } from './telemetry';
import type { WebSocketClient } from './websocket-client';

const ACTION_TYPE_MAP: Record<string, string> = {
  approve_pr: 'approve',
  request_changes: 'request_changes',
  post_comment: 'post_comment',
  approve: 'approve',
};

export function setupIPC(
  path: string,
  logger: Logger,
  getWsClient: () => WebSocketClient | null,
  settings: SettingsService,
  getWsPort: () => number | null,
  telemetry?: TelemetryService,
): void {
  logger.info(`IPC setup (db path: ${path})`);

  // ---- engine:* handlers ----
  ipcMain.handle('engine:get-ws-port', () => {
    return { ok: getWsPort() };
  });

  // ---- settings:* handlers ----
  ipcMain.handle('settings:get', async () => {
    return { ok: settings.snapshot };
  });

  ipcMain.handle(
    'settings:set',
    async (_event, patch: Partial<AppSettings>) => {
      try {
        const next = await settings.set(patch);
        telemetry?.setEnabled(next.privacy.telemetryEnabled);
        trackSettingsPatch(telemetry, patch, next);
        return { ok: next };
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        logger.error(`settings:set error: ${message}`);
        return { err: message };
      }
    },
  );

  ipcMain.handle('settings:reset', async () => {
    try {
      const next = await settings.reset();
      telemetry?.setEnabled(next.privacy.telemetryEnabled);
      return { ok: next };
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      logger.error(`settings:reset error: ${message}`);
      return { err: message };
    }
  });

  ipcMain.handle('settings:open-file', async () => {
    return settings.openFile();
  });

  ipcMain.handle('settings:path', async () => {
    return { ok: settings.path };
  });

  // ---- dialog:* handlers ----
  ipcMain.handle('dialog:pick-folder', async () => {
    const result = await dialog.showOpenDialog({
      properties: ['openDirectory'],
    });
    return result.canceled ? null : (result.filePaths[0] ?? null);
  });

  // ---- fs:read-patch handler ----
  ipcMain.handle(
    'fs:read-patch',
    async (_event, request: FSReadPatchRequest) => {
      try {
        // Mirror Go's os.UserCacheDir() per platform
        const cacheBase =
          process.env.XDG_CACHE_HOME ??
          (process.platform === 'darwin'
            ? join(homedir(), 'Library', 'Caches')
            : process.platform === 'win32'
              ? (process.env.LOCALAPPDATA ??
                join(homedir(), 'AppData', 'Local'))
              : join(homedir(), '.cache'));
        const baseDir = resolve(join(cacheBase, 'bistro'));
        const resolved = resolve(request.path);
        if (!resolved.startsWith(baseDir + sep)) {
          logger.warn(
            `fs:read-patch: rejected path outside cache: ${resolved}`,
          );
          void telemetry?.track('fs_patch_read', { allowed: false });
          return { err: 'Access denied: path outside allowed directory' };
        }
        const content = await readFile(resolved, 'utf-8');
        void telemetry?.track('fs_patch_read', { allowed: true });
        return { ok: content };
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'Unknown error';
        logger.error(`fs:read-patch error: ${message}`);
        return { err: message };
      }
    },
  );

  // ---- go:command handler ----
  ipcMain.handle('go:command', async (_event, request: GoCommandRequest) => {
    try {
      const wsClient = getWsClient();
      if (!wsClient?.isConnected()) {
        void telemetry?.track('review_action_requested', {
          action: ACTION_TYPE_MAP[request.type] ?? 'unknown',
          online: false,
        });
        return { err: 'WebSocket not connected — action queued for retry' };
      }

      const action = ACTION_TYPE_MAP[request.type];
      if (action === undefined) {
        return { err: `Unknown action type: ${request.type}` };
      }

      logger.info(`Forwarding go:command: ${action} for PR ${request.pr_id}`);
      void telemetry?.track('review_action_requested', {
        action,
        online: true,
      });

      wsClient.send({
        type: 'ACTION_REQUEST',
        action,
        pr_id: request.pr_id,
        body: request.comment ?? '',
        line: request.line ?? 0,
        file: request.file ?? '',
      });

      return { ok: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`go:command error: ${message}`);
      return { err: message };
    }
  });

  // ---- sync:action handler ----
  ipcMain.handle('sync:action', async (_event, request: SyncActionRequest) => {
    try {
      const wsClient = getWsClient();
      if (!wsClient?.isConnected()) {
        void telemetry?.track('review_action_requested', {
          action: ACTION_TYPE_MAP[request.type] ?? request.type,
          online: false,
        });
        return { err: 'WebSocket not connected' };
      }

      const syncAction = ACTION_TYPE_MAP[request.type] ?? request.type;
      void telemetry?.track('review_action_requested', {
        action: syncAction,
        online: true,
      });

      wsClient.send({
        type: 'ACTION_REQUEST',
        action: syncAction,
        pr_id: request.pr_id,
        body: request.comment ?? '',
        line: request.line ?? 0,
        file: '',
      });

      return { ok: { success: true } };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`sync:action error: ${message}`);
      return { err: message };
    }
  });

  logger.info('IPC handlers registered');
}

function trackSettingsPatch(
  telemetry: TelemetryService | undefined,
  patch: Partial<AppSettings>,
  next: AppSettings,
): void {
  if (patch.appearance !== undefined) {
    void telemetry?.track('settings_updated', { section: 'appearance' });
  }
  if (patch.keybindings !== undefined) {
    void telemetry?.track('settings_updated', { section: 'keyboard' });
  }
  if (patch.prList !== undefined) {
    void telemetry?.track('settings_updated', { section: 'prList' });
  }
  if (patch.sync !== undefined) {
    void telemetry?.track('settings_updated', { section: 'sync' });
  }
  if (patch.privacy !== undefined) {
    void telemetry?.track('settings_updated', {
      section: 'privacy',
      telemetryEnabled: next.privacy.telemetryEnabled,
    });
  }
}
