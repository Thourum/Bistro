// Slice 8 — RouterProvider outside AuthGate (C3)
//
// RouterContext is now { queryClient } only — auth state is read directly
// from appStore / settingsStore inside guards so the router never receives
// stale context values from React props.
//
// Route tree:
//   rootRoute        → RootShell  (OverlayProvider + hotkeys + toasts, all routes)
//     /              → index      (sync redirect to /login|/onboarding|/prs)
//     /login         → LoginRoute (ConnectingScreen while !authChecked, then LoginPage)
//     /onboarding    → OnboardingPage (guard: requires authed, redirects if already done)
//     authedRoute    → AuthedRouteShell (pathless, requireAuthedAndOnboarded guard)
//       /prs         → prsRoute (Outlet)
//         /          → PrsIndexRouteComponent
//         /$prId     → PRDetailPage
//         /$prId/diff→ DiffPage
//       /settings    → SettingsPage
//
// Auth flow (no redirect loops):
//   /          → immediate sync redirect (if !authChecked → /login which shows
//                ConnectingScreen; when Bootstrap invalidates → right place)
//   /login     → beforeLoad: if already authed, redirect out.
//                component: ConnectingScreen while !authChecked, LoginPage after.
//   /onboarding→ requires authed (else /login); if onboarded → redirect to target.
//   authedRoute→ requireAuthedAndOnboarded: awaits authChecked, then checks creds.
//                Deep-link with authChecked=false → shows ConnectingScreen (pendingMs=500)
//                then after auth resolves, either stays or redirects to /login.
//
// Reconnect safety: authChecked momentarily false → requireAuthedAndOnboarded
//   awaits. Bootstrap fires invalidate when authChecked flips back to true.
//   isAuthenticated is still true → no redirect. ✓
//
// Auth-expired safety: AUTH_TOKEN_EXPIRED → isAuthenticated:false, authChecked
//   stays true. Bootstrap invalidates → requireAuthedAndOnboarded: checked
//   immediately (no wait), isAuthenticated:false → redirect /login. ✓

import type { QueryClient } from '@tanstack/react-query';
import {
  createHashHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from '@tanstack/react-router';
import type React from 'react';
import { z } from 'zod/v4';
import { AuthedRouteShell } from './components/AuthedRouteShell';
import { ConnectingScreen } from './components/ConnectingScreen';
import { PrsIndexRouteComponent } from './components/PrsIndexRouteComponent';
import { RootShell } from './components/RootShell';
import { useStore } from './hooks/useStore';
import { DiffPage } from './pages/DiffPage';
import { LoginPage } from './pages/LoginPage';
import { OnboardingPage } from './pages/OnboardingPage';
import { PRDetailPage } from './pages/PRDetailPage';
import { SettingsPage } from './pages/SettingsPage';
import { appStore } from './store';
import { settingsStore } from './store/settings';

// ---------------------------------------------------------------------------
// Router context type (Slice 8: auth fields removed, guards read stores)
// ---------------------------------------------------------------------------

export interface RouterContext {
  queryClient: QueryClient;
}

// ---------------------------------------------------------------------------
// Auth guard helpers
// ---------------------------------------------------------------------------

function getAuthSnapshot() {
  return {
    authChecked: appStore.state.authChecked,
    isAuthenticated: appStore.state.isAuthenticated,
    onboardingComplete: settingsStore.state.appearance.onboardingComplete,
  };
}

/**
 * Resolves when appStore.authChecked becomes true.
 * If it's already true, resolves immediately (synchronous-ish).
 * CRITICAL: never redirects while authChecked===false — callers must await this
 * before checking isAuthenticated so transient reconnect states don't produce
 * spurious /login redirects.
 */
function waitForAuthChecked(): Promise<void> {
  if (appStore.state.authChecked) return Promise.resolve();
  return new Promise<void>((resolve) => {
    const sub = appStore.subscribe(() => {
      if (appStore.state.authChecked) {
        sub.unsubscribe();
        resolve();
      }
    });
  });
}

/**
 * Validates a redirect param: must be a string, must start with '/',
 * must not redirect back to /login or /onboarding (loop prevention).
 */
function safeRedirect(v: unknown): string | undefined {
  if (typeof v !== 'string') return undefined;
  if (!v.startsWith('/')) return undefined;
  if (v.startsWith('/login') || v.startsWith('/onboarding')) return undefined;
  return v;
}

/**
 * Guards all authed routes. Awaits authChecked (reconnect-safe), then:
 * - !isAuthenticated  → /login?redirect=<current>
 * - !onboardingComplete → /onboarding?redirect=<current>
 */
async function requireAuthedAndOnboarded({
  location,
}: {
  location: { pathname: string; searchStr?: string };
}) {
  // Wait for auth to be resolved — handles reconnect (authChecked:false) without
  // bouncing to /login. The promise resolves once authChecked becomes true.
  if (!appStore.state.authChecked) {
    await waitForAuthChecked();
  }
  const { isAuthenticated, onboardingComplete } = getAuthSnapshot();
  const redirectPath = location.pathname + (location.searchStr ?? '');
  if (!isAuthenticated) {
    throw redirect({
      to: '/login',
      search: { redirect: safeRedirect(redirectPath) ?? undefined },
    });
  }
  if (!onboardingComplete) {
    throw redirect({
      to: '/onboarding',
      search: { redirect: safeRedirect(redirectPath) ?? undefined },
    });
  }
}

// ---------------------------------------------------------------------------
// Search param schemas
// ---------------------------------------------------------------------------

const redirectSearchSchema = z.object({
  redirect: z.string().optional(),
});

const prsIndexSearchSchema = z.object({
  filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
  sort: z.enum(['updated', 'created']).default('updated'),
  q: z.string().default(''),
});

const diffSearchSchema = z.object({
  file: z.string().default(''),
  side: z.enum(['left', 'right']).default('right'),
});

// ---------------------------------------------------------------------------
// Route components
// ---------------------------------------------------------------------------

/** /login component: ConnectingScreen while auth unknown, LoginPage once checked. */
function LoginRouteComponent(): React.ReactElement {
  const authChecked = useStore(appStore, (s) => s.authChecked);
  if (!authChecked) return <ConnectingScreen />;
  return <LoginPage />;
}

// ---------------------------------------------------------------------------
// Route definitions
// ---------------------------------------------------------------------------

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootShell,
});

/** Index — awaits authChecked then redirects to the right screen.
 * Shows ConnectingScreen (pendingComponent) while waiting.
 * Never redirects while authChecked===false — satisfies C3. */
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  pendingComponent: ConnectingScreen,
  pendingMs: 0,
  beforeLoad: async () => {
    if (!appStore.state.authChecked) await waitForAuthChecked();
    const { isAuthenticated, onboardingComplete } = getAuthSnapshot();
    if (!isAuthenticated) throw redirect({ to: '/login' });
    if (!onboardingComplete) throw redirect({ to: '/onboarding' });
    throw redirect({ to: '/prs' });
  },
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: redirectSearchSchema,
  beforeLoad: ({ search }) => {
    const { authChecked, isAuthenticated, onboardingComplete } =
      getAuthSnapshot();
    // Only redirect away if we definitely know the user is authenticated.
    if (authChecked && isAuthenticated) {
      if (!onboardingComplete) {
        // Carry the redirect param so post-onboarding deep-link survives.
        throw redirect({
          to: '/onboarding',
          search: { redirect: safeRedirect(search.redirect) },
        });
      }
      throw redirect({ to: safeRedirect(search.redirect) ?? '/prs' });
    }
    // Not yet checked or not authenticated — stay on /login.
  },
  component: LoginRouteComponent,
});

const onboardingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/onboarding',
  validateSearch: redirectSearchSchema,
  beforeLoad: async ({ search }) => {
    if (!appStore.state.authChecked) await waitForAuthChecked();
    const { isAuthenticated, onboardingComplete } = getAuthSnapshot();
    if (!isAuthenticated) {
      throw redirect({ to: '/login' });
    }
    if (onboardingComplete) {
      throw redirect({ to: safeRedirect(search.redirect) ?? '/prs' });
    }
    // Not onboarded yet — stay on /onboarding.
  },
  component: OnboardingPage,
});

/**
 * Pathless layout route that guards all authenticated screens.
 * pendingMs: 500 avoids ConnectingScreen flash on fast reconnects.
 */
const authedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'authed',
  beforeLoad: requireAuthedAndOnboarded,
  pendingComponent: ConnectingScreen,
  pendingMs: 500,
  component: AuthedRouteShell,
});

// /prs layout route
const prsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/prs',
  component: () => <Outlet />,
});

// /prs/ index — owned by router (Slice 1)
const prsIndexRoute = createRoute({
  getParentRoute: () => prsRoute,
  path: '/',
  validateSearch: prsIndexSearchSchema,
  component: PrsIndexRouteComponent,
});

// /prs/$prId — Slice 2: real detail route (C2 fix)
export const prDetailRoute = createRoute({
  getParentRoute: () => prsRoute,
  path: '$prId',
  component: PRDetailPage,
});

// /prs/$prId/diff — Slice 3: real diff route (C5)
export const prDiffRoute = createRoute({
  getParentRoute: () => prsRoute,
  path: '$prId/diff',
  validateSearch: diffSearchSchema,
  component: DiffPage,
});

// /settings — Slice 5: real settings route
const settingsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/settings',
  component: SettingsPage,
});

// ---------------------------------------------------------------------------
// Route tree + router
// ---------------------------------------------------------------------------

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  onboardingRoute,
  authedRoute.addChildren([
    prsRoute.addChildren([prsIndexRoute, prDetailRoute, prDiffRoute]),
    settingsRoute,
  ]),
]);

const history = createHashHistory();

export const router = createRouter({
  routeTree,
  history,
  context: {
    queryClient: undefined as unknown as QueryClient,
  },
});

// ---------------------------------------------------------------------------
// Module augmentation for type safety
// ---------------------------------------------------------------------------

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
