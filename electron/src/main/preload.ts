import { contextBridge, ipcRenderer } from 'electron';
import type {
  DBQueryRequest,
  DBQueryResponse,
  FSReadPatchRequest,
  FSReadPatchResponse,
  GoCommandRequest,
  GoCommandResponse,
  SyncActionRequest,
  SyncActionResponse,
  IPCError,
} from '../types/ipc';
import type { SyncEvent } from './websocket-client';

const VALID_SEND_CHANNELS = ['sync:subscribe'] as const;
const VALID_EVENT_CHANNELS = ['sync:event'] as const;

type ValidSendChannel = typeof VALID_SEND_CHANNELS[number];
type ValidEventChannel = typeof VALID_EVENT_CHANNELS[number];

// Expose safe IPC methods to renderer
contextBridge.exposeInMainWorld('electron', {
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
  async fs(request: FSReadPatchRequest): Promise<FSReadPatchResponse | IPCError> {
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
  async syncAction(request: SyncActionRequest): Promise<SyncActionResponse | IPCError> {
    return ipcRenderer.invoke('sync:action', request);
  },

  /**
   * Subscribe to sync events from main process.
   * Returns an unsubscribe function.
   */
  on(channel: ValidEventChannel, listener: (data: SyncEvent) => void): () => void {
    if (!(VALID_EVENT_CHANNELS as readonly string[]).includes(channel)) {
      return () => undefined;
    }
    const wrappedListener = (_event: Electron.IpcRendererEvent, data: SyncEvent) => listener(data);
    ipcRenderer.on(channel, wrappedListener);
    return () => ipcRenderer.removeListener(channel, wrappedListener);
  },
});

declare global {
  interface Window {
    electron: {
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (request: FSReadPatchRequest) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
      send: (channel: ValidSendChannel, data?: unknown) => void;
      syncAction: (request: SyncActionRequest) => Promise<SyncActionResponse | IPCError>;
      on: (channel: ValidEventChannel, listener: (data: SyncEvent) => void) => () => void;
    };
  }
}
