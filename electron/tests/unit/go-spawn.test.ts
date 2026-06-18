import { describe, expect, it } from 'vitest';
import { parseReadyLine } from '../../src/main/go-spawn';

describe('parseReadyLine', () => {
  it('parses a valid ready signal', () => {
    expect(parseReadyLine('{"event":"ready","wsPort":63796}')).toBe(63796);
  });

  it('parses a ready signal with surrounding whitespace', () => {
    expect(parseReadyLine('  {"event":"ready","wsPort":8080}\n')).toBe(8080);
  });

  it('returns null for non-JSON lines', () => {
    expect(parseReadyLine('WebSocket server listening on :8080')).toBeNull();
  });

  it('returns null for JSON with a different event', () => {
    expect(parseReadyLine('{"event":"sync_complete","wsPort":8080}')).toBeNull();
  });

  it('returns null when wsPort is missing', () => {
    expect(parseReadyLine('{"event":"ready"}')).toBeNull();
  });

  it('returns null for out-of-range ports', () => {
    expect(parseReadyLine('{"event":"ready","wsPort":0}')).toBeNull();
    expect(parseReadyLine('{"event":"ready","wsPort":70000}')).toBeNull();
    expect(parseReadyLine('{"event":"ready","wsPort":-1}')).toBeNull();
  });

  it('returns null when wsPort is not a number', () => {
    expect(parseReadyLine('{"event":"ready","wsPort":"8080"}')).toBeNull();
  });

  it('returns null for empty input', () => {
    expect(parseReadyLine('')).toBeNull();
  });
});
