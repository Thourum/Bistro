import { describe, expect, it } from 'vitest';
import { readWsPortFromArgv, wsPortToArg } from '../../src/main/ws-port';

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

  it('returns null when the arg is absent', () => {
    expect(readWsPortFromArgv(['/path/electron'])).toBeNull();
  });

  it('returns null for non-numeric values', () => {
    expect(readWsPortFromArgv(['--bistro-ws-port=abc'])).toBeNull();
  });

  it('returns null for out-of-range ports', () => {
    expect(readWsPortFromArgv(['--bistro-ws-port=0'])).toBeNull();
    expect(readWsPortFromArgv(['--bistro-ws-port=70000'])).toBeNull();
  });

  it('returns null for empty argv', () => {
    expect(readWsPortFromArgv([])).toBeNull();
  });
});
