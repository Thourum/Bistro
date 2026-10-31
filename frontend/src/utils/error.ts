import type { Fault } from '@thourum/fault';

/**
 * Known error codes from engine services.
 */
export const ERROR_MESSAGES: Readonly<Record<string, string>> = {
  AUTH_TOKEN_EXPIRED: 'Your authentication expired. Please re-login.',
  RATE_LIMITED: 'Too many requests. Try again in a few minutes.',
  GO_PROCESS_CRASHED: 'Sync engine crashed. Restarting automatically...',
  NETWORK_OFFLINE: 'No network connection. Using cached data.',
  DB_QUERY_FAILED: 'Failed to load data. Please try again.',
  PERMISSION_DENIED: 'You do not have permission to perform this action.',
  PR_NOT_FOUND: 'Pull request not found. It may have been closed or deleted.',
  DIFF_FETCH_FAILED: 'Failed to load diff. Please try again.',
};

/**
 * Patterns that indicate sensitive data — must be stripped from user-facing messages.
 */
const SENSITIVE_PATTERNS: readonly RegExp[] = [
  /ghp_[a-zA-Z0-9]+/g,
  /gho_[a-zA-Z0-9]+/g,
  /Bearer\s+eyJ[a-zA-Z0-9._-]+/g,
  /token=[a-zA-Z0-9._-]+/g,
  /eyJ[a-zA-Z0-9._-]{20,}/g,
];

/**
 * Remove sensitive data from an error message string.
 */
function sanitize(message: string): string {
  let result = message;
  for (const pattern of SENSITIVE_PATTERNS) {
    result = result.replace(pattern, '[REDACTED]');
  }
  return result;
}

/**
 * Map an error code, raw message, or Fault to a user-friendly string.
 * Known codes/tags get a curated message; unknown messages are sanitized.
 */
export function formatErrorMessage(error: string | Fault): string {
  const message = typeof error === 'string' ? error : error.message;
  const code = typeof error === 'string' ? error : (error.tag ?? message);
  const friendly = Object.hasOwn(ERROR_MESSAGES, code)
    ? ERROR_MESSAGES[code]
    : undefined;
  const friendlyMessage = Object.hasOwn(ERROR_MESSAGES, message)
    ? ERROR_MESSAGES[message]
    : undefined;
  return friendly ?? friendlyMessage ?? sanitize(message);
}
