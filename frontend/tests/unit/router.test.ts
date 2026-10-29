// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';
import { router } from '../../src/router';
import { appStore } from '../../src/store';
import { settingsStore } from '../../src/store/settings';
import { initialUIState } from '../../src/store/ui';
import { DEFAULT_SETTINGS } from '../../src/types/electron/settings';

beforeEach(() => {
  appStore.setState(() => ({ ...initialUIState, authChecked: false }));
  settingsStore.setState(() => DEFAULT_SETTINGS);
});

describe('router smoke tests', () => {
  it('instantiates without throwing', () => {
    expect(router).toBeDefined();
  });

  it('route tree includes /prs path', () => {
    // Walk the tree to find prs route regardless of nesting
    function findPath(
      routes: Array<{ path?: string; children?: unknown[] }>,
      p: string,
    ): boolean {
      for (const r of routes) {
        if (r.path === p) return true;
        if (
          r.children &&
          findPath(
            r.children as Array<{ path?: string; children?: unknown[] }>,
            p,
          )
        )
          return true;
      }
      return false;
    }
    const routes = router.routeTree.children ?? [];
    expect(
      findPath(routes as Array<{ path?: string; children?: unknown[] }>, 'prs'),
    ).toBe(true);
  });

  it('canonicalizes mixed-case PR IDs in encoded detail links', () => {
    const matches = router.matchRoutes('/prs/github%3AThourum%2FTesting%2342', {});
    const detailMatch = matches.find(
      (match) => match.routeId === '/authed/prs/$prId',
    );

    expect(detailMatch?.params.prId).toBe('github:thourum/testing#42');
  });

  it('canonicalizes mixed-case PR IDs in encoded diff links without changing file casing', () => {
    const matches = router.matchRoutes(
      '/prs/github%3AThourum%2FTesting%2342/diff',
      { file: 'src/MixedCase.ts', side: 'right' },
    );
    const diffMatch = matches.find(
      (match) => match.routeId === '/authed/prs/$prId/diff',
    );

    expect(diffMatch?.params.prId).toBe('github:thourum/testing#42');
    expect(diffMatch?.search.file).toBe('src/MixedCase.ts');
  });

  it('router can navigate to /prs when authed', async () => {
    // Seed auth so the requireAuthedAndOnboarded guard passes without waiting.
    appStore.setState((s) => ({
      ...s,
      authChecked: true,
      isAuthenticated: true,
    }));
    settingsStore.setState((s) => ({
      ...s,
      appearance: { ...s.appearance, onboardingComplete: true },
    }));
    await router.navigate({ to: '/prs' });
    expect(router.state.location.pathname).toBe('/prs');
  });
});
