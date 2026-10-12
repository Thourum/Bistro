import { Fault } from '@thourum/fault';
import { describe, expect, it } from 'vitest';
import { parseWsMessage } from './useWebSocket';

describe('parseWsMessage', () => {
  it.each([
    'AUTH_CHECK_RESULT',
    'AUTH_DISCONNECT_RESULT',
  ])('normalizes a null provider list in %s from an unauthenticated Go engine', (type) => {
    const parsed = parseWsMessage(
      JSON.stringify({ type, data: { authenticated: false, providers: null } }),
    );

    expect(parsed.isOk()).toBe(true);
    if (parsed.isOk())
      expect(parsed.value.data).toMatchObject({ providers: [] });

    const invalid = parseWsMessage(
      JSON.stringify({
        type,
        data: { authenticated: false, providers: false },
      }),
    );
    expect(invalid.isErr()).toBe(true);
    const fault = invalid._unsafeUnwrapErr();
    expect(fault).toBeInstanceOf(Fault);
    expect(fault.message).toBe(`Invalid ${type} payload`);
    expect(fault.tag).toBe('VALIDATION_ERROR');
  });

  it.each([
    'PR_CHECKOUT_STATUS',
    'PR_CHECKOUT_RESULT',
  ])('accepts %s for request correlation', (type) => {
    expect(
      parseWsMessage(
        JSON.stringify({ id: 'request-1', type, data: {} }),
      ).isOk(),
    ).toBe(true);
  });
});
