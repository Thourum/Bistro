import { ipcMain } from 'electron';
import Database from 'better-sqlite3';
import { readFileSync } from 'fs';
import { Logger } from 'electron-log';
import type {
  DBQueryRequest,
  FSReadPatchRequest,
  GoCommandRequest,
  SyncActionRequest,
} from '../types/ipc';
import type { IPCHandler } from './types';
import type { WebSocketClient } from './websocket-client';

let db: Database.Database | null = null;
let dbPath: string | null = null;

/**
 * Lazily opens the SQLite database on first query.
 * This allows the Go process to create the DB file before Electron reads it.
 */
function getDatabase(logger: Logger): Database.Database {
  if (db !== null) {
    return db;
  }

  if (dbPath === null) {
    throw new Error('Database path not configured');
  }

  try {
    db = new Database(dbPath);
    logger.info(`Connected to SQLite: ${dbPath}`);
    return db;
  } catch (e) {
    logger.error(`Failed to open database at ${dbPath}: ${e}`);
    throw e;
  }
}

export function setupIPC(path: string, logger: Logger, wsClient: WebSocketClient): void {
  dbPath = path;
  logger.info(`IPC database path set to: ${dbPath}`);

  // ---- db:query handler ----
  ipcMain.handle('db:query', async (_event, request: DBQueryRequest) => {
    try {
      const database = getDatabase(logger);
      const stmt = database.prepare(request.sql);
      const result = stmt.all(...(request.params || []));
      return { ok: result };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`db:query error: ${message}`);
      return { err: message };
    }
  });

  // ---- fs:read-patch handler ----
  ipcMain.handle('fs:read-patch', async (_event, request: FSReadPatchRequest) => {
    try {
      const content = readFileSync(request.path, 'utf-8');
      return { ok: content };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`fs:read-patch error: ${message}`);
      return { err: message };
    }
  });

  // ---- go:command handler ----
  ipcMain.handle('go:command', async (_event, request: GoCommandRequest) => {
    try {
      logger.info(`Go command received: ${request.type}`);
      return { ok: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`go:command error: ${message}`);
      return { err: message };
    }
  });

  // ---- sync:subscribe handler ----
  ipcMain.on('sync:subscribe', (_event) => {
    logger.info('Renderer subscribed to sync events');
  });

  // ---- sync:action handler ----
  ipcMain.handle('sync:action', async (_event, request: SyncActionRequest) => {
    try {
      if (!wsClient.isConnected()) {
        return { err: 'WebSocket not connected' };
      }

      wsClient.send({
        type: request.type,
        pr_id: request.pr_id,
        ...(request.comment !== undefined ? { comment: request.comment } : {}),
        ...(request.line !== undefined ? { line: request.line } : {}),
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

export function closeDatabase(): void {
  if (db !== null) {
    db.close();
    db = null;
  }
}
