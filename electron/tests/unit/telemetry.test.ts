import { mkdtemp, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it, vi } from 'vitest';
import {
  resolveTelemetryEndpoint,
  sanitizeTelemetryEvent,
  TelemetryService,
} from '../../src/main/telemetry';

const logger = {
  warn: vi.fn(),
  info: vi.fn(),
  error: vi.fn(),
} as never;

describe('privacy-safe telemetry', () => {
  it('drops confidential and non-allowlisted event properties', () => {
    const event = sanitizeTelemetryEvent('review_action_requested', {
      action: 'approve',
      online: true,
      repoName: 'secret/repo',
      username: 'octocat',
      filePath: '/private/repo/src/App.tsx',
      diff: '@@ private code',
      comment: 'LGTM with private context',
      token: 'ghp_secret',
      pr_id: 123,
    });

    expect(event).toEqual({
      name: 'review_action_requested',
      properties: { action: 'approve', online: true },
    });
  });

  it('does not persist or transmit before explicit consent', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'bistro-telemetry-disabled-'));
    const transport = vi.fn(async () => undefined);
    const service = new TelemetryService({
      enabled: false,
      storageDir: dir,
      endpoint: 'https://telemetry.example.test/batch',
      appVersion: '0.1.0',
      platform: 'darwin',
      logger,
      transport,
    });

    await service.trackAppStarted();

    await expect(stat(join(dir, 'events.jsonl'))).rejects.toThrow();
    expect(transport).not.toHaveBeenCalled();
    await rm(dir, { recursive: true, force: true });
  });

  it('persists and transmits only sanitized payloads after consent', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'bistro-telemetry-enabled-'));
    const batches: unknown[] = [];
    const service = new TelemetryService({
      enabled: true,
      storageDir: dir,
      endpoint: 'https://telemetry.example.test/batch',
      appVersion: '0.1.0',
      platform: 'darwin',
      logger,
      transport: async (_endpoint, payloads) => {
        batches.push(payloads);
      },
    });

    await service.track('settings_updated', {
      section: 'privacy',
      telemetryEnabled: true,
      repo: 'secret/repo',
      path: '/private/file.ts',
      code: 'private()',
    });

    const stored = await readFile(join(dir, 'events.jsonl'), 'utf-8');
    expect(stored).toContain('settings_updated');
    expect(stored).toContain('telemetryEnabled');
    expect(stored).not.toContain('secret/repo');
    expect(stored).not.toContain('/private/file.ts');
    expect(stored).not.toContain('private()');
    expect(batches).toHaveLength(1);
    await rm(dir, { recursive: true, force: true });
  });

  it('only accepts https or localhost development endpoints', () => {
    expect(resolveTelemetryEndpoint(undefined)).toBeNull();
    expect(resolveTelemetryEndpoint('')).toBeNull();
    expect(
      resolveTelemetryEndpoint('http://telemetry.example.test'),
    ).toBeNull();
    expect(resolveTelemetryEndpoint('https://telemetry.example.test')).toBe(
      'https://telemetry.example.test',
    );
    expect(resolveTelemetryEndpoint('http://127.0.0.1:8787/batch')).toBe(
      'http://127.0.0.1:8787/batch',
    );
    expect(resolveTelemetryEndpoint('http://localhost:8787/batch')).toBe(
      'http://localhost:8787/batch',
    );
    expect(resolveTelemetryEndpoint('http://[::1]:8787/batch')).toBe(
      'http://[::1]:8787/batch',
    );
    expect(
      resolveTelemetryEndpoint('http://127.0.0.1.evil.example/batch'),
    ).toBeNull();
    expect(resolveTelemetryEndpoint('http://user@127.0.0.1/batch')).toBeNull();
    expect(resolveTelemetryEndpoint('http://192.168.1.20/batch')).toBeNull();
  });
});
