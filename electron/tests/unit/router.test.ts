// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest';
import { router } from '../../src/renderer/router';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';
import { settingsStore } from '../../src/renderer/store/settings';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

beforeEach(() => {
  appStore.setState(() => ({ ...initialUIState, authChecked: false }));
});

describe('router smoke tests', () => {
  it('instantiates without throwing', () => {
    expect(router).toBeDefined();
  });

  it('route tree includes /prs path', () => {
    // Walk the tree to find prs route regardless of nesting
    function findPath(routes: Array<{ path?: string; children?: unknown[] }>, p: string): boolean {
      for (const r of routes) {
        if (r.path === p) return true;
        if (r.children && findPath(r.children as Array<{ path?: string; children?: unknown[] }>, p)) return true;
      }
      return false;
    }
    const routes = router.routeTree.children ?? [];
    expect(findPath(routes as Array<{ path?: string; children?: unknown[] }>, 'prs')).toBe(true);
  });

  it('router can navigate to /prs when authed', async () => {
    // Seed auth so the requireAuthedAndOnboarded guard passes without waiting.
    appStore.setState((s) => ({ ...s, authChecked: true, isAuthenticated: true }));
    settingsStore.setState((s) => ({
      ...s,
      appearance: { ...s.appearance, onboardingComplete: true },
    }));
    await router.navigate({ to: '/prs' });
    expect(router.state.location.pathname).toBe('/prs');
  });
});
