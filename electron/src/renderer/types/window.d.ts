import type { ProcessExit } from '../../main/go-spawn';
import type { SyncEvent } from '../../main/websocket-client';
import type { EngineReply, EngineStatus } from '../../types/engine';
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
} from '../../types/ipc';
import type { AppSettings } from '../../types/settings';

type EventChannel =
  | 'sync:event'
  | 'settings:changed'
  | 'engine:status'
  | 'go:crashed';

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
