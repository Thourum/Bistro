import { Fault } from '@thourum/fault';
import { describe, expect, it } from 'vitest';
import { parseWsMessage } from './wsSchemas';

describe('parseWsMessage', () => {
  it.each([
    'AUTH_CHECK_RESULT',
    'AUTH_DISCONNECT_RESULT',
  ])('normalizes null provider list in %s from unauthenticated Go engine', (type) => {
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
  ])('accepts %s request correlation', (type) => {
    expect(
      parseWsMessage(
        JSON.stringify({ id: 'request-1', type, data: {} }),
      ).isOk(),
    ).toBe(true);
  });

  it('rejects inherited object keys instead of treating them as event schemas', () => {
    expect(
      parseWsMessage(
        JSON.stringify({ type: 'constructor' }),
      )._unsafeUnwrapErr(),
    ).toMatchObject({
      tag: 'BAD_REQUEST',
      message: 'Unknown WebSocket event: constructor',
    });
  });

  it('preserves omitted response data accepted by unknown payload schemas', () => {
    expect(
      parseWsMessage(JSON.stringify({ type: 'DIFF_READY' }))._unsafeUnwrap(),
    ).toEqual({ type: 'DIFF_READY', data: undefined });
  });
});
