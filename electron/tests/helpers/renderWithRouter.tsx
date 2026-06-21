// @vitest-environment jsdom
/**
 * renderWithRouter — test helper for components that need a router context.
 * Creates an isolated in-memory router for each test, so tests don't share state.
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router';
import { render } from '@testing-library/react';
import type { ReactNode } from 'react';
import type { RouterContext } from '../../src/renderer/router';

interface RenderWithRouterOptions {
  initialUrl?: string;
  context?: Partial<RouterContext>;
}

/**
 * Wrap a component in a minimal in-memory TanStack Router + QueryClientProvider.
 * Suitable for unit-testing route-aware components without the real route tree.
 */
export function renderWithRouter(
  ui: ReactNode,
  { initialUrl = '/', context = {} }: RenderWithRouterOptions = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  const rootRoute = createRootRouteWithContext<RouterContext>()({
    component: () => <>{ui}</>,
  });

  const routeTree = rootRoute;

  const memHistory = createMemoryHistory({ initialEntries: [initialUrl] });

  const testRouter = createRouter({
    routeTree,
    history: memHistory,
    context: {
      queryClient,
      authChecked: true,
      isAuthenticated: true,
      onboardingComplete: true,
      ...context,
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={testRouter} />
    </QueryClientProvider>,
  );
}
