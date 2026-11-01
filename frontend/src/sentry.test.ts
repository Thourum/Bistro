import { afterEach, expect, it, vi } from 'vitest';

const init = vi.hoisted(() => vi.fn());
vi.mock('@sentry/browser', () => ({ init }));
afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  init.mockClear();
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

it('gates error events on current telemetry consent and drops click breadcrumbs', async () => {
  vi.stubEnv('VITE_SENTRY_ENABLED', 'true');
  vi.stubGlobal('location', { search: '' });
  vi.resetModules();
  await import('./sentry');
  const { settingsStore } = await import('./store/settings');
  const options = init.mock.calls[0]?.[0];
  expect(options.beforeSend({ message: 'failure' })).toBeNull();
  settingsStore.setState((settings) => ({
    ...settings,
    privacy: { telemetryEnabled: true },
  }));
  expect(options.beforeSend({ message: 'failure' })).toEqual({
    message: 'failure',
  });
  expect(options.beforeBreadcrumb({ category: 'ui.click' })).toBeNull();
});
