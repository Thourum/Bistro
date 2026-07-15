import { mkdtemp, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
  beforeEach(() => {
    vi.stubEnv('BISTRO_POSTHOG_API_KEY', '');
    vi.stubEnv('POSTHOG_API_KEY', '');
  });

  afterEach(() => {
    vi.unstubAllEnvs();
  });

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

  it('does not create install id or capture before explicit consent', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'bistro-telemetry-disabled-'));
    const client = {
      capture: vi.fn(),
      disable: vi.fn(async () => undefined),
      enable: vi.fn(async () => undefined),
      flush: vi.fn(async () => undefined),
    };
    const service = new TelemetryService({
      enabled: false,
      storageDir: dir,
      endpoint: 'https://telemetry.example.test/batch',
      appVersion: '0.1.0',
      platform: 'darwin',
      logger,
      apiKey: 'phc_test',
      clientFactory: () => client,
    });

    await service.trackAppStarted();

    await expect(stat(join(dir, 'anonymous-install-id'))).rejects.toThrow();
    expect(client.capture).not.toHaveBeenCalled();
    await rm(dir, { recursive: true, force: true });
  });

  it('captures only sanitized payloads through PostHog after consent', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'bistro-telemetry-enabled-'));
    const client = {
      capture: vi.fn(),
      disable: vi.fn(async () => undefined),
      enable: vi.fn(async () => undefined),
      flush: vi.fn(async () => undefined),
    };
    const service = new TelemetryService({
      enabled: true,
      storageDir: dir,
      endpoint: 'https://telemetry.example.test/batch',
      appVersion: '0.1.0',
      platform: 'darwin',
      logger,
      apiKey: 'phc_test',
      clientFactory: () => client,
    });

    await service.track('settings_updated', {
      section: 'privacy',
      telemetryEnabled: true,
      repo: 'secret/repo',
      path: '/private/file.ts',
      code: 'private()',
    });

    const installId = (
      await readFile(join(dir, 'anonymous-install-id'), 'utf-8')
    ).trim();
    expect(client.capture).toHaveBeenCalledWith({
      distinctId: installId,
      event: 'settings_updated',
      timestamp: expect.any(Date),
      properties: {
        section: 'privacy',
        telemetryEnabled: true,
        telemetrySchemaVersion: 1,
      },
    });
    const captured = JSON.stringify(client.capture.mock.calls);
    expect(captured).not.toContain('secret/repo');
    expect(captured).not.toContain('/private/file.ts');
    expect(captured).not.toContain('private()');
    await rm(dir, { recursive: true, force: true });
  });

  it('does not create a PostHog client without endpoint and API key', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'bistro-telemetry-unconfigured-'));
    const clientFactory = vi.fn();
    const service = new TelemetryService({
      enabled: true,
      storageDir: dir,
      endpoint: null,
      appVersion: '0.1.0',
      platform: 'darwin',
      logger,
      apiKey: null,
      clientFactory,
    });

    await service.trackAppStarted();

    await expect(stat(join(dir, 'anonymous-install-id'))).rejects.toThrow();
    expect(clientFactory).not.toHaveBeenCalled();
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
