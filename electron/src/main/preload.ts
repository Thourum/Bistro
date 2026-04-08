import { contextBridge, ipcRenderer } from 'electron';
import type {
  DBQueryRequest,
  DBQueryResponse,
  FSReadPatchRequest,
  FSReadPatchResponse,
  GoCommandRequest,
  GoCommandResponse,
  IPCError,
} from '../types/ipc';

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
});

declare global {
  interface Window {
    electron: {
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (request: FSReadPatchRequest) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
    };
  }
}
