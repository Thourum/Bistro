import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const sentryMock = vi.hoisted(() => ({
  captureException: vi.fn(),
  init: vi.fn(),
}));

vi.mock('@sentry/electron/main', () => sentryMock);
vi.mock('electron', () => ({ app: { isPackaged: false } }));

describe('Sentry consent gate', () => {
  beforeEach(() => {
    vi.resetModules();
    sentryMock.captureException.mockReset();
    sentryMock.init.mockReset();
    process.env.SENTRY_DSN = 'https://public@example.test/1';
    delete process.env.VITE_SENTRY_DSN;
  });

  afterEach(() => {
    delete process.env.SENTRY_DSN;
    delete process.env.VITE_SENTRY_DSN;
  });

  it('does not initialize or capture before explicit telemetry consent', async () => {
    const { captureSentryException, configureSentryConsent, isSentryEnabled } =
      await import('../../src/main/sentry');

    expect(configureSentryConsent(false)).toBe(false);
    expect(isSentryEnabled()).toBe(false);

    captureSentryException(new Error('private startup failure'));

    expect(sentryMock.init).not.toHaveBeenCalled();
    expect(sentryMock.captureException).not.toHaveBeenCalled();
  });

  it('captures only while telemetry consent remains enabled', async () => {
    const { captureSentryException, configureSentryConsent, isSentryEnabled } =
      await import('../../src/main/sentry');

    expect(configureSentryConsent(true)).toBe(true);
    expect(isSentryEnabled()).toBe(true);
    captureSentryException(new Error('consented failure'));

    expect(sentryMock.init).toHaveBeenCalledTimes(1);
    expect(sentryMock.captureException).toHaveBeenCalledTimes(1);

    configureSentryConsent(false);
    captureSentryException(new Error('disabled again'));

    expect(isSentryEnabled()).toBe(false);
    expect(sentryMock.captureException).toHaveBeenCalledTimes(1);
  });

  it('scrubs crash events and drops SDK-originated events after opt-out', async () => {
    const { configureSentryConsent } = await import('../../src/main/sentry');

    configureSentryConsent(true);

    type SentryInitOptions = {
      beforeSend: (event: {
        breadcrumbs?: unknown;
        contexts?: unknown;
        exception?: {
          values?: Array<{
            stacktrace?: { frames?: Array<{ filename?: string }> };
            type?: string;
            value?: string;
          }>;
        };
        extra?: unknown;
        message?: string;
        request?: unknown;
        user?: unknown;
      }) => unknown;
    };
    const options = sentryMock.init.mock.calls[0]?.[0] as SentryInitOptions;
    const event = {
      breadcrumbs: [{ message: '/Users/private/repo/file.ts' }],
      contexts: { os: { private: '/Users/private' } },
      exception: {
        values: [
          {
            stacktrace: { frames: [{ filename: '/Users/private/repo/file.ts' }] },
            type: 'Error',
            value: 'failed at /Users/private/repo with ghp_secret',
          },
        ],
      },
      extra: { token: 'ghp_secret' },
      message: 'failed at https://example.test/private',
      request: { url: 'https://example.test/private' },
      user: { username: 'octocat' },
    };

    expect(options.beforeSend(event)).toBe(event);
    expect(event.message).toBe('[redacted]');
    expect(event.exception.values?.[0]?.value).toBe('[redacted]');
    expect(event.exception.values?.[0]?.stacktrace?.frames?.[0]?.filename).toBe(
      'file.ts',
    );
    expect(event).not.toHaveProperty('breadcrumbs');
    expect(event).not.toHaveProperty('contexts');
    expect(event).not.toHaveProperty('extra');
    expect(event).not.toHaveProperty('request');
    expect(event).not.toHaveProperty('user');

    configureSentryConsent(false);

    expect(options.beforeSend({ message: 'post opt-out' })).toBeNull();
  });
});
