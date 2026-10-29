import type {
  EngineReply,
  EngineStatus,
} from '@renderer/types/electron/engine';
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
} from '@renderer/types/electron/ipc';
import type { AppSettings } from '@renderer/types/electron/settings';

// ponytail: retain only the legacy event shapes until Task 18 replaces this bridge.
type ProcessExit = { code: number | null; signal: string | null };
type SyncEvent = {
  type: string;
  data?: Record<string, unknown> | undefined;
  id?: string | undefined;
  timestamp?: string | undefined;
};

type EventChannel =
  | 'sync:event'
  | 'settings:changed'
  | 'engine:status'
  | 'go:crashed';

declare global {
  interface Window {
    electron: {
      readonly isKeysBench: boolean;
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
