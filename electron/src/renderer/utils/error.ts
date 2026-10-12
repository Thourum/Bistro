/**
 * Known error codes from Go sync engine and IPC layer.
 */
export const ERROR_MESSAGES: Readonly<Record<string, string>> = {
  AUTH_TOKEN_EXPIRED: 'Your authentication expired. Please re-login.',
  RATE_LIMITED: 'Too many requests. Try again in a few minutes.',
  WEBSOCKET_DISCONNECTED: 'Connection lost. Reconnecting...',
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
 * Map an error code or raw message to a user-friendly string.
 * Known codes get a curated message; unknown messages are sanitized and returned as-is.
 */
export function formatErrorMessage(codeOrMessage: string): string {
  const friendly = Object.hasOwn(ERROR_MESSAGES, codeOrMessage)
    ? ERROR_MESSAGES[codeOrMessage]
    : undefined;
  return friendly ?? sanitize(codeOrMessage);
}
