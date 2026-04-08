import type { DBQueryRequest, FSReadPatchRequest, GoCommandRequest } from '../types/ipc';

export interface IPCHandler {
  'db:query': (req: DBQueryRequest) => Promise<Record<string, unknown>[]>;
  'fs:read-patch': (req: FSReadPatchRequest) => Promise<string>;
  'go:command': (req: GoCommandRequest) => Promise<boolean>;
}

export interface GoProcessConfig {
  binaryPath: string;
  dbPath: string;
  wsPort: number;
  logLevel: 'debug' | 'info' | 'warn' | 'error';
  logFile: string;
  replayQueue: boolean;
}

export interface AppConfig {
  windowWidth: number;
  windowHeight: number;
  goProcess: GoProcessConfig;
}
