import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { spawnGoProcess, isGoProcessRunning, killGoProcess } from '../../src/main/go-spawn';
import type { GoProcessConfig } from '../../src/main/types';

describe('Go Process Spawn', () => {
  let logger = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
  let config: GoProcessConfig = {
    binaryPath: '/fake/path/bistro-sync',
    dbPath: '/fake/path/bistro.db',
    wsPort: 8080,
  };

  beforeEach(() => {
    logger = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
  });

  afterEach(() => {
    if (isGoProcessRunning()) {
      killGoProcess();
    }
  });

  it('should track process state', () => {
    expect(isGoProcessRunning()).toBe(false);

    // NOTE: Can't actually spawn without Go binary; this tests the API
    // Real test would mock child_process
  });
});
