import { describe, expect, it } from 'vitest';
import {
  DEFAULT_WS_PORT,
  readWsPortFromArgv,
  wsPortToArg,
} from '../../src/main/ws-port';

describe('wsPortToArg', () => {
  it('formats the port as a CLI argument', () => {
    expect(wsPortToArg(63796)).toBe('--bistro-ws-port=63796');
  });
});

describe('readWsPortFromArgv', () => {
  it('reads the port from argv', () => {
    expect(
      readWsPortFromArgv(['/path/electron', '--bistro-ws-port=63796']),
    ).toBe(63796);
  });

  it('round-trips with wsPortToArg', () => {
    expect(readWsPortFromArgv([wsPortToArg(12345)])).toBe(12345);
  });

  it('falls back to default when the arg is absent', () => {
    expect(readWsPortFromArgv(['/path/electron'])).toBe(DEFAULT_WS_PORT);
  });

  it('falls back to default for non-numeric values', () => {
    expect(readWsPortFromArgv(['--bistro-ws-port=abc'])).toBe(DEFAULT_WS_PORT);
  });

  it('falls back to default for out-of-range ports', () => {
    expect(readWsPortFromArgv(['--bistro-ws-port=0'])).toBe(DEFAULT_WS_PORT);
    expect(readWsPortFromArgv(['--bistro-ws-port=70000'])).toBe(
      DEFAULT_WS_PORT,
    );
  });

  it('falls back to default for empty argv', () => {
    expect(readWsPortFromArgv([])).toBe(DEFAULT_WS_PORT);
  });
});
