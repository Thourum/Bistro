import { expect, it } from 'vitest';
import { getAuthScreenState, type AuthScreenInput } from '../../src/utils/authScreenState';

const idle: AuthScreenInput = {
  isConnected: true,
  authLoading: false,
  authUrl: null,
  authError: null,
  authTimedOut: false,
  authInterrupted: false,
};
it.each([
  [idle, 'idle'],
  [{ ...idle, authLoading: true, authUrl: 'https://github.com/login/oauth/authorize' }, 'waiting'],
  [{ ...idle, authTimedOut: true }, 'timedOut'],
  [{ ...idle, authError: 'denied' }, 'failed'],
  [{ ...idle, authInterrupted: true }, 'interrupted'],
  [{ ...idle, isConnected: false }, 'engineDown'],
  [{ ...idle, isConnected: false, authLoading: true, authUrl: 'https://github.com/login/oauth/authorize' }, 'interrupted'],
])('maps %j to %s', (input, expected) => {
  expect(getAuthScreenState(input)).toBe(expected);
});
