import { Result } from 'neverthrow';
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

const DEFAULT_LEVEL: pino.LevelWithSilent = import.meta.env.DEV
  ? 'debug'
  : 'info';
const readStoredLevel = Result.fromThrowable(() =>
  localStorage.getItem('bistro-log-level'),
);

function isLogLevel(level: string): level is pino.LevelWithSilent {
  return level === 'silent' || Object.hasOwn(pino.levels.values, level);
}

const ROOT_LEVEL = readStoredLevel().match(
  (stored) => (stored !== null && isLogLevel(stored) ? stored : DEFAULT_LEVEL),
  () => DEFAULT_LEVEL,
);

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
