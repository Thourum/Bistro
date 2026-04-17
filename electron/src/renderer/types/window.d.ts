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
} from '../../types/ipc';
import type { AppSettings } from '../../types/settings';
import type { SyncEvent } from '../../main/websocket-client';

type EventChannel = 'sync:event' | 'settings:changed';

declare global {
  interface Window {
    electron: {
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (
        request: FSReadPatchRequest,
      ) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
      send: (channel: 'sync:subscribe', data?: unknown) => void;
      syncAction: (
        request: SyncActionRequest,
      ) => Promise<SyncActionResponse | IPCError>;
      on: <C extends EventChannel>(
        channel: C,
        listener: (
          data: C extends 'sync:event'
            ? SyncEvent
            : C extends 'settings:changed'
              ? AppSettings
              : unknown,
        ) => void,
      ) => () => void;
      openExternal: (url: string) => Promise<{ ok: true } | { err: string }>;
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
    electronAPI?: {
      quitApp?: () => void;
    };
  }
}
