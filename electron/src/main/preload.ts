import { contextBridge, ipcRenderer } from 'electron';
import type { EngineReply, EngineStatus } from '../types/engine';
import type {
  DBQueryRequest,
  DBQueryResponse,
  FSReadPatchRequest,
  FSReadPatchResponse,
  GoCommandRequest,
  GoCommandResponse,
  IPCError,
  OpenInEditorRequest,
  OpenInEditorResponse,
  SyncActionRequest,
  SyncActionResponse,
} from '../types/ipc';
import type { AppSettings } from '../types/settings';
import type { ProcessExit } from './go-spawn';
import type { SyncEvent } from './websocket-client';
import { readWsPortFromArgv } from './ws-port';

const VALID_SEND_CHANNELS = ['sync:subscribe'] as const;
const VALID_EVENT_CHANNELS = [
  'sync:event',
  'settings:changed',
  'engine:status',
  'go:crashed',
] as const;

type ValidSendChannel = (typeof VALID_SEND_CHANNELS)[number];
type ValidEventChannel = (typeof VALID_EVENT_CHANNELS)[number];

// Expose safe IPC methods to renderer
contextBridge.exposeInMainWorld('electron', {
  /**
   * Actual WebSocket port of the Go sync engine, delivered via
   * additionalArguments at window creation (Go picks it dynamically).
   */
  wsPort: readWsPortFromArgv(process.argv),
  platform: process.platform,
  getEngineStatus: (): Promise<EngineReply<EngineStatus>> =>
    ipcRenderer.invoke('engine:get-status'),
  getAppVersion: (): Promise<EngineReply<string>> =>
    ipcRenderer.invoke('app:get-version'),
  restartEngine: (): Promise<EngineReply<boolean>> =>
    ipcRenderer.invoke('engine:restart'),
  getEngineLogTail: (lines: number): Promise<EngineReply<readonly string[]>> =>
    ipcRenderer.invoke('engine:log-tail', { lines }),
  openLogFolder: (): Promise<EngineReply<boolean>> =>
    ipcRenderer.invoke('engine:open-log-folder'),
  reportCrash: (): Promise<EngineReply<boolean>> =>
    ipcRenderer.invoke('engine:report-crash'),

  async getWsPort(): Promise<number | null> {
    const response = (await ipcRenderer.invoke('engine:get-ws-port')) as {
      ok: unknown;
    };
    return typeof response.ok === 'number' &&
      Number.isInteger(response.ok) &&
      response.ok >= 1 &&
      response.ok <= 65535
      ? response.ok
      : null;
  },

  /**
   * Query SQLite database
   * @returns { ok: Record<string, unknown>[] } | { err: string }
   */
  async db(request: DBQueryRequest): Promise<DBQueryResponse | IPCError> {
    return ipcRenderer.invoke('db:query', request);
  },

  /**
   * Read patch file from filesystem
   * @returns { ok: string } | { err: string }
   */
  async fs(
    request: FSReadPatchRequest,
  ): Promise<FSReadPatchResponse | IPCError> {
    return ipcRenderer.invoke('fs:read-patch', request);
  },

  /**
   * Send command to Go backend
   * @returns { ok: boolean } | { err: string }
   */
  async go(request: GoCommandRequest): Promise<GoCommandResponse | IPCError> {
    return ipcRenderer.invoke('go:command', request);
  },

  /**
   * Listen for WebSocket events from Go
   */
  onGoEvent(callback: (event: Record<string, unknown>) => void): void {
    ipcRenderer.on('go:event', (_, event) => callback(event));
  },

  /**
   * Send a one-way message to main process (e.g. sync:subscribe)
   */
  send(channel: ValidSendChannel, data?: unknown): void {
    if ((VALID_SEND_CHANNELS as readonly string[]).includes(channel)) {
      ipcRenderer.send(channel, data);
    }
  },

  /**
   * Send a sync action to Go backend via WebSocket
   * @returns { ok: { success: boolean } } | { err: string }
   */
  async syncAction(
    request: SyncActionRequest,
  ): Promise<SyncActionResponse | IPCError> {
    return ipcRenderer.invoke('sync:action', request);
  },

  /**
   * Subscribe to events from the main process (sync events, settings changes).
   * Returns an unsubscribe function.
   */
  on<C extends ValidEventChannel>(
    channel: C,
    listener: (
      data: C extends 'sync:event'
        ? SyncEvent
        : C extends 'settings:changed'
          ? AppSettings
          : C extends 'engine:status'
            ? EngineStatus
            : C extends 'go:crashed'
              ? ProcessExit
              : unknown,
    ) => void,
  ): () => void {
    if (!(VALID_EVENT_CHANNELS as readonly string[]).includes(channel)) {
      return () => undefined;
    }
    const wrappedListener = (
      _event: Electron.IpcRendererEvent,
      data: unknown,
    ) => listener(data as never);
    ipcRenderer.on(channel, wrappedListener);
    return () => ipcRenderer.removeListener(channel, wrappedListener);
  },

  /**
   * Open a URL in the system default browser.
   * Only https:// URLs are allowed.
   * @returns { ok: true } | { err: string }
   */
  async openExternal(url: string): Promise<{ ok: true } | { err: string }> {
    return ipcRenderer.invoke('open-external', url);
  },

  /**
   * Open a file in $EDITOR at an optional line. Main validates relativePath
   * stays inside repoRoot before spawning.
   * @returns { ok: true } | { err: string }
   */
  async openInEditor(
    request: OpenInEditorRequest,
  ): Promise<OpenInEditorResponse | IPCError> {
    return ipcRenderer.invoke('open-in-editor', request);
  },

  quitApp(): Promise<{ ok: true }> {
    return ipcRenderer.invoke('app:quit');
  },

  pickFolder(): Promise<string | null> {
    return ipcRenderer.invoke('dialog:pick-folder');
  },

  /**
   * User settings persisted to `userData/settings.json`.
   */
  settings: {
    async get(): Promise<{ ok: AppSettings } | IPCError> {
      return ipcRenderer.invoke('settings:get');
    },
    async set(
      patch: Partial<AppSettings>,
    ): Promise<{ ok: AppSettings } | IPCError> {
      return ipcRenderer.invoke('settings:set', patch);
    },
    async reset(): Promise<{ ok: AppSettings } | IPCError> {
      return ipcRenderer.invoke('settings:reset');
    },
    async openFile(): Promise<{ ok: true } | { err: string }> {
      return ipcRenderer.invoke('settings:open-file');
    },
    async path(): Promise<{ ok: string } | IPCError> {
      return ipcRenderer.invoke('settings:path');
    },
  },
});

declare global {
  interface Window {
    electron: {
      wsPort: number | null;
      readonly platform: string;
      getWsPort: () => Promise<number | null>;
      getEngineStatus: () => Promise<EngineReply<EngineStatus>>;
      getAppVersion: () => Promise<EngineReply<string>>;
      restartEngine: () => Promise<EngineReply<boolean>>;
      getEngineLogTail: (
        lines: number,
      ) => Promise<EngineReply<readonly string[]>>;
      openLogFolder: () => Promise<EngineReply<boolean>>;
      reportCrash: () => Promise<EngineReply<boolean>>;
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (
        request: FSReadPatchRequest,
      ) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
      send: (channel: ValidSendChannel, data?: unknown) => void;
      syncAction: (
        request: SyncActionRequest,
      ) => Promise<SyncActionResponse | IPCError>;
      on: <C extends ValidEventChannel>(
        channel: C,
        listener: (
          data: C extends 'sync:event'
            ? SyncEvent
            : C extends 'settings:changed'
              ? AppSettings
              : C extends 'engine:status'
                ? EngineStatus
                : C extends 'go:crashed'
                  ? ProcessExit
                  : unknown,
        ) => void,
      ) => () => void;
      openExternal: (url: string) => Promise<{ ok: true } | { err: string }>;
      openInEditor: (
        request: OpenInEditorRequest,
      ) => Promise<OpenInEditorResponse | IPCError>;
      quitApp: () => Promise<{ ok: true }>;
      pickFolder: () => Promise<string | null>;
      settings: {
        get: () => Promise<{ ok: AppSettings } | IPCError>;
        set: (
          patch: Partial<AppSettings>,
        ) => Promise<{ ok: AppSettings } | IPCError>;
        reset: () => Promise<{ ok: AppSettings } | IPCError>;
        openFile: () => Promise<{ ok: true } | { err: string }>;
        path: () => Promise<{ ok: string } | IPCError>;
      };
    };
  }
}
