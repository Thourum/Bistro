import type {
  DBQueryRequest,
  DBQueryResponse,
  FSReadPatchRequest,
  FSReadPatchResponse,
  GoCommandRequest,
  GoCommandResponse,
  IPCError,
} from '../../types/ipc';

declare global {
  interface Window {
    electron: {
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (request: FSReadPatchRequest) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
    };
    electronAPI?: {
      quitApp?: () => void;
    };
  }
}

export {};
