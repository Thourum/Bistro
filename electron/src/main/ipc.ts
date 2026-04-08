import { ipcMain } from 'electron';
import Database from 'better-sqlite3';
import { readFileSync } from 'fs';
import { join } from 'path';
import { Logger } from 'electron-log';
import type {
  DBQueryRequest,
  FSReadPatchRequest,
  GoCommandRequest,
} from '../types/ipc';
import type { IPCHandler } from './types';

let db: Database.Database | null = null;

export function setupIPC(dbPath: string, logger: Logger): void {
  // Initialize database
  try {
    db = new Database(dbPath);
    logger.info(`Connected to SQLite: ${dbPath}`);
  } catch (e) {
    logger.error(`Failed to open database: ${e}`);
    throw e;
  }

  // ---- db:query handler ----
  ipcMain.handle('db:query', async (event, request: DBQueryRequest) => {
    try {
      if (!db) throw new Error('Database not initialized');

      const stmt = db.prepare(request.sql);
      const result = stmt.all(...(request.params || []));

      return { ok: result };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`db:query error: ${message}`);
      return { err: message };
    }
  });

  // ---- fs:read-patch handler ----
  ipcMain.handle('fs:read-patch', async (event, request: FSReadPatchRequest) => {
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
  ipcMain.handle('go:command', async (event, request: GoCommandRequest) => {
    try {
      // TODO: Send command via WebSocket to Go backend
      // For now, just acknowledge
      logger.info(`Go command received: ${request.type}`);
      return { ok: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`go:command error: ${message}`);
      return { err: message };
    }
  });

  logger.info('IPC handlers registered');
}

export function closeDatabase(): void {
  if (db) {
    db.close();
    db = null;
  }
}
