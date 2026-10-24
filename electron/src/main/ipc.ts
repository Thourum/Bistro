import { readFile, realpath } from 'node:fs/promises';
import { homedir } from 'node:os';
import { join, resolve, sep } from 'node:path';
import {
  errAsync,
  Fault,
  fromPromise,
  fromThrowable,
  ServiceError,
} from '@thourum/fault';
import { dialog, ipcMain } from 'electron';
import type { Logger } from 'electron-log';
import { buildEditorArgs, parseEditorCommand } from '../shared/editorArgs';
import {
  fsReadPatchRequestSchema,
  goCommandRequestSchema,
  type OpenInEditorRequest,
  syncActionRequestSchema,
} from '../types/ipc';
import { type AppSettings, appSettingsSchema } from '../types/settings';
import { launchEditor } from './editorLaunch';
import { validateOpenInEditorRequest } from './editorRequest';
import { resolveEditorTarget } from './editorTarget';
import type { SettingsService } from './settings';
import type { TelemetryService } from './telemetry';
import type { WebSocketClient } from './websocket-client';

const ACTION_TYPE_MAP = {
  approve_pr: 'approve',
  request_changes: 'request_changes',
  post_comment: 'post_comment',
} satisfies Record<
  keyof (typeof goCommandRequestSchema.shape.type)['enum'],
  string
>;

function ipcFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause : String(cause))
    .withTag('IPC_ERROR')
    .withCause(cause);
}

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

  ipcMain.handle('settings:set', (_event, input: unknown) => {
    const parsed = appSettingsSchema.partial().safeParse(input);
    if (!parsed.success) return { err: 'Invalid settings patch' };
    const patch = Object.fromEntries(
      Object.entries(parsed.data).filter(([, value]) => value !== undefined),
    ) as Partial<AppSettings>;
    return fromThrowable(() => settings.setResult(patch), ipcFault)().match(
      (result) => {
        if ('err' in result) return result;
        telemetry?.setEnabled(result.ok.privacy.telemetryEnabled);
        trackSettingsPatch(telemetry, patch, result.ok);
        return result;
      },
      (error) => {
        logger.error(`settings:set error: ${error.message}`);
        return { err: error.message };
      },
    );
  });

  ipcMain.handle('settings:reset', () =>
    fromThrowable(() => settings.resetResult(), ipcFault)().match(
      (result) => {
        if ('ok' in result)
          telemetry?.setEnabled(result.ok.privacy.telemetryEnabled);
        return result;
      },
      (error) => {
        logger.error(`settings:reset error: ${error.message}`);
        return { err: error.message };
      },
    ),
  );

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
  ipcMain.handle('fs:read-patch', async (_event, input: unknown) => {
    const parsed = fsReadPatchRequestSchema.safeParse(input);
    if (!parsed.success) return { err: 'Invalid patch path' };

    // Mirror Go's os.UserCacheDir() per platform.
    const cacheBase =
      process.env.XDG_CACHE_HOME ??
      (process.platform === 'darwin'
        ? join(homedir(), 'Library', 'Caches')
        : process.platform === 'win32'
          ? (process.env.LOCALAPPDATA ?? join(homedir(), 'AppData', 'Local'))
          : join(homedir(), '.cache'));
    const baseDir = resolve(join(cacheBase, 'bistro'));
    const resolved = resolve(parsed.data.path);
    if (!resolved.startsWith(baseDir + sep)) {
      void telemetry?.track('fs_patch_read', { allowed: false });
      return { err: 'Access denied: path outside allowed directory' };
    }
    return fromPromise(realpath(baseDir), ipcFault)
      .andThen((canonicalBase) =>
        fromPromise(realpath(resolved), ipcFault).andThen((canonicalFile) =>
          canonicalFile.startsWith(canonicalBase + sep)
            ? fromPromise(readFile(canonicalFile, 'utf-8'), ipcFault)
            : errAsync(
                ServiceError(
                  'AUTHORIZATION_ERROR',
                  'Access denied: path outside allowed directory',
                ),
              ),
        ),
      )
      .match(
        (content) => {
          void telemetry?.track('fs_patch_read', { allowed: true });
          return { ok: content };
        },
        (error) => {
          void telemetry?.track('fs_patch_read', { allowed: false });
          logger.warn(`fs:read-patch rejected: ${error.message}`);
          return { err: error.message };
        },
      );
  });

  // ---- open-in-editor handler ----
  // Canonicalises the checkout root and the target (following symlinks) and
  // rejects anything that escapes the repository, then spawns the editor with
  // an argument array — never a shell string.
  ipcMain.handle(
    'open-in-editor',
    async (_event, request: OpenInEditorRequest) => {
      const editorCommand = process.env.EDITOR ?? '';
      // `$EDITOR` may carry arguments (`code --wait`); split it into
      // binary + leading args so spawn stays argv-based (no shell).
      const editor = parseEditorCommand(editorCommand);
      if (editor.isErr()) {
        return { err: 'Set $EDITOR' };
      }

      return validateOpenInEditorRequest(request)
        .asyncAndThen((valid) =>
          resolveEditorTarget({
            repoRoot: valid.repoRoot,
            relativePath: valid.relativePath,
          }).map((target) => ({ target, line: valid.line })),
        )
        .andThen(({ target, line }) =>
          buildEditorArgs(editorCommand, target, line).asyncAndThen((args) =>
            launchEditor(editor.value.bin, args, logger),
          ),
        )
        .match(
          () => ({ ok: true as const }),
          (error) => {
            logger.warn(`open-in-editor rejected: ${error.message}`);
            return { err: error.message };
          },
        );
    },
  );

  // ---- go:command handler ----
  ipcMain.handle('go:command', (_event, input: unknown) => {
    const parsed = goCommandRequestSchema.safeParse(input);
    if (!parsed.success) return { err: 'Invalid go command' };
    const request = parsed.data;
    const wsClient = getWsClient();
    const action = ACTION_TYPE_MAP[request.type];
    if (!wsClient?.isConnected()) {
      void telemetry?.track('review_action_requested', {
        action,
        online: false,
      });
      return { err: 'WebSocket not connected — action queued for retry' };
    }
    logger.info(`Forwarding go:command: ${action} for PR ${request.pr_id}`);
    void telemetry?.track('review_action_requested', { action, online: true });
    return wsClient
      .send({
        type: 'ACTION_REQUEST',
        action,
        pr_id: request.pr_id,
        body: request.comment ?? '',
        line: request.line ?? 0,
        file: request.file ?? '',
      })
      .match(
        () => ({ ok: true }),
        (error) => {
          logger.error(`go:command error: ${error.message}`);
          return { err: error.message };
        },
      );
  });

  // ---- sync:action handler ----
  ipcMain.handle('sync:action', (_event, input: unknown) => {
    const parsed = syncActionRequestSchema.safeParse(input);
    if (!parsed.success) return { err: 'Invalid sync action' };
    const request = parsed.data;
    const wsClient = getWsClient();
    const action = ACTION_TYPE_MAP[request.type];
    if (!wsClient?.isConnected()) {
      void telemetry?.track('review_action_requested', {
        action,
        online: false,
      });
      return { err: 'WebSocket not connected' };
    }
    void telemetry?.track('review_action_requested', { action, online: true });
    return wsClient
      .send({
        type: 'ACTION_REQUEST',
        action,
        pr_id: request.pr_id,
        body: request.comment ?? '',
        line: request.line ?? 0,
        file: '',
      })
      .match(
        () => ({ ok: { success: true } }),
        (error) => {
          logger.error(`sync:action error: ${error.message}`);
          return { err: error.message };
        },
      );
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
  if (patch.navigation !== undefined) {
    void telemetry?.track('settings_updated', {
      section: 'navigation',
      style: next.navigation.style,
    });
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
