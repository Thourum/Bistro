// @vitest-environment jsdom
import { Auth, DevFolderScan } from '@bindings/services';
import { act, StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { subscribeEngineEvents } from '../services/events';
import { cancellable } from '../test/cancellable';
import { useEngine } from './useEngine';

vi.mock('@bindings/services', () => ({
  Auth: { Check: vi.fn() },
  DevFolderScan: { Scan: vi.fn() },
}));
vi.mock('../services/events', () => ({ subscribeEngineEvents: vi.fn() }));
vi.mock('../store/settings', () => ({
  settingsStore: { state: { repositories: { devFolderPath: '/projects' } } },
}));

function EngineHost(): null {
  useEngine();
  return null;
}

it('dispatches startup once while StrictMode cleans up each event subscription', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const firstCleanup = vi.fn();
  const secondCleanup = vi.fn();
  vi.mocked(subscribeEngineEvents)
    .mockReturnValueOnce(firstCleanup)
    .mockReturnValueOnce(secondCleanup);
  vi.mocked(Auth.Check).mockReturnValue(
    cancellable(
      Promise.resolve({
        authenticated: false,
        providers: null,
        auth_status: '',
        auth_reason: '',
        requires_auth: true,
        exchange: 'proxy',
      }),
    ),
  );
  vi.mocked(DevFolderScan.Scan).mockReturnValue(
    cancellable(
      Promise.resolve({
        discovered: 0,
        healthy: 0,
        degraded: 0,
        skipped: 0,
        partial: false,
      }),
    ),
  );
  const root = createRoot(document.createElement('div'));
  await act(async () =>
    root.render(
      <StrictMode>
        <EngineHost />
      </StrictMode>,
    ),
  );
  expect(Auth.Check).toHaveBeenCalledTimes(1);
  expect(DevFolderScan.Scan).toHaveBeenCalledExactlyOnceWith({
    path: '/projects',
    reason: 'startup',
  });
  expect(subscribeEngineEvents).toHaveBeenCalledTimes(2);
  expect(firstCleanup).toHaveBeenCalledTimes(1);
  expect(secondCleanup).not.toHaveBeenCalled();
  await act(async () => root.unmount());
  expect(secondCleanup).toHaveBeenCalledTimes(1);
  vi.unstubAllGlobals();
});
