import { describe, expect, it } from 'vitest';
import {
  ERROR_MESSAGES,
  formatErrorMessage,
} from '../../src/utils/error';

describe('formatErrorMessage', () => {
  it('maps AUTH_TOKEN_EXPIRED to friendly message', () => {
    expect(formatErrorMessage('AUTH_TOKEN_EXPIRED')).toBe(
      'Your authentication expired. Please re-login.',
    );
  });

  it('maps RATE_LIMITED to friendly message', () => {
    expect(formatErrorMessage('RATE_LIMITED')).toBe(
      'Too many requests. Try again in a few minutes.',
    );
  });

  it('maps WEBSOCKET_DISCONNECTED to friendly message', () => {
    expect(formatErrorMessage('WEBSOCKET_DISCONNECTED')).toBe(
      'Connection lost. Reconnecting...',
    );
  });

  it('maps GO_PROCESS_CRASHED to friendly message', () => {
    expect(formatErrorMessage('GO_PROCESS_CRASHED')).toBe(
      'Sync engine crashed. Restarting automatically...',
    );
  });

  it('maps NETWORK_OFFLINE to friendly message', () => {
    expect(formatErrorMessage('NETWORK_OFFLINE')).toBe(
      'No network connection. Using cached data.',
    );
  });

  it('returns raw message for unknown error codes', () => {
    expect(formatErrorMessage('Something unexpected happened')).toBe(
      'Something unexpected happened',
    );
  });

  it('strips sensitive data patterns from messages', () => {
    expect(formatErrorMessage('Error: token=ghp_abc123xyz')).not.toContain(
      'ghp_',
    );
    expect(formatErrorMessage('Bearer eyJhbGci...')).not.toContain('eyJ');
  });

  it('has entries for all known error codes', () => {
    const codes = [
      'AUTH_TOKEN_EXPIRED',
      'RATE_LIMITED',
      'WEBSOCKET_DISCONNECTED',
      'GO_PROCESS_CRASHED',
      'NETWORK_OFFLINE',
      'DB_QUERY_FAILED',
      'PERMISSION_DENIED',
      'PR_NOT_FOUND',
      'DIFF_FETCH_FAILED',
    ];
    for (const code of codes) {
      expect(ERROR_MESSAGES[code]).toBeDefined();
    }
  });
});
