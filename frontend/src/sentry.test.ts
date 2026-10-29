import { afterEach, expect, it, vi } from 'vitest';

const init = vi.hoisted(() => vi.fn());
vi.mock('@sentry/electron/renderer', () => ({ init }));
afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  init.mockClear();
});

it('keeps native benchmark telemetry disabled after query removal and reload', async () => {
  vi.stubEnv('VITE_SENTRY_ENABLED', 'true');
  vi.stubGlobal('window', { electron: { isKeysBench: true } });
  vi.stubGlobal('location', { search: '' });
  vi.resetModules();
  await import('./sentry');
  expect(init).not.toHaveBeenCalled();
});

it.each(['?bench_keys=1', '?bench_keys=invalid'])(
  'never initializes renderer Sentry when keyboard benchmark requested: %s',
  async (search) => {
    vi.stubEnv('VITE_SENTRY_ENABLED', 'true');
    vi.stubGlobal('location', { search });
    vi.resetModules();
    await import('./sentry');
    expect(init).not.toHaveBeenCalled();
  },
);
