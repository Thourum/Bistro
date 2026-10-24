import type { Logger } from 'electron-log';
import { execa } from 'execa';
import { describe, expect, it, vi } from 'vitest';
import {
  getStderrTail,
  parseReadyLine,
  spawnGoProcess,
} from '../../src/main/go-spawn';

vi.mock('execa', async (importOriginal) => {
  const actual = await importOriginal<typeof import('execa')>();
  return { ...actual, execa: vi.fn(actual.execa) };
});

it('captures startup stderr without a trailing newline at EOF', async () => {
  const actual = await vi.importActual<typeof import('execa')>('execa');
  vi.mocked(execa).mockImplementationOnce(() =>
    actual.execa(
      process.execPath,
      [
        '-e',
        'process.stderr.write("fatal startup failure"); process.exitCode = 1;',
      ],
      { stdin: 'pipe', stdout: 'pipe', stderr: 'pipe', buffer: false },
    ),
  );
  const logger = {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  } as unknown as Logger;
  await expect(
    spawnGoProcess({
      logger,
      config: {
        binaryPath: 'unused',
        dbPath: 'unused',
        wsPort: 0,
        logLevel: 'info',
        logFile: '',
        replayQueue: true,
        syncInterval: 30,
        autoSync: false,
      },
    }),
  ).rejects.toBeDefined();
  expect(getStderrTail()).toContain('fatal startup failure');
});

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
    expect(
      parseReadyLine('{"event":"sync_complete","wsPort":8080}'),
    ).toBeNull();
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
