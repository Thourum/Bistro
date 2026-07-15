import pino from 'pino';

/**
 * Structured logger for the renderer process.
 *
 * Uses pino's built-in browser transport which maps levels to console methods.
 * All log entries include a component name for easy filtering in DevTools.
 *
 * Usage:
 *   import { createLogger } from '../utils/logger';
 *   const log = createLogger('WebSocket');
 *   log.info({ url, status }, 'connected');
 *   log.error({ type, readyState }, 'send failed — not connected');
 */

const ROOT_LEVEL = ((): pino.LevelWithSilent => {
  try {
    const stored = localStorage.getItem('bistro-log-level');
    if (stored !== null) return stored as pino.LevelWithSilent;
  } catch {
    // localStorage not available
  }
  // Default: debug in dev, info in prod
  return process.env.NODE_ENV === 'development' ? 'debug' : 'info';
})();

const rootLogger = pino({
  level: ROOT_LEVEL,
  browser: {
    asObject: true,
  },
});

/**
 * Create a child logger with a fixed `component` field.
 */
export function createLogger(component: string): pino.Logger {
  return rootLogger.child({ component });
}
