// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AppFooter } from '../../src/renderer/components/AppFooter';
import {
  FooterCenterSlot,
  FooterLeftSlot,
  FooterSlotProvider,
} from '../../src/renderer/components/FooterSlot';
import {
  OverlayProvider,
  useOverlays,
} from '../../src/renderer/overlays/OverlayContext';
import { appStore } from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import { initialUIState } from '../../src/renderer/store/ui';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

expect.extend(matchers);

// Mock router hooks — AppFooter now always renders inside RouterProvider
// (under authedRoute) but tests run without one, so we mock the hooks.
const mockPathname = { current: '/prs' };
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useRouterState: vi.fn(() => ({
      location: { pathname: mockPathname.current },
    })),
    useNavigate: vi.fn(() => vi.fn()),
  };
});

beforeEach(() => {
  mockPathname.current = '/prs';
  appStore.setState(() => ({ ...initialUIState }));
  settingsStore.setState(() => DEFAULT_SETTINGS);
});

function HelpSourceProbe(): React.ReactElement {
  const { helpSourceRoute } = useOverlays();
  return <div data-testid="help-source">{helpSourceRoute}</div>;
}

function renderFooter(children: React.ReactNode = null) {
  return render(
    <OverlayProvider>
      <FooterSlotProvider>
        {children}
        <HelpSourceProbe />
        <AppFooter />
      </FooterSlotProvider>
    </OverlayProvider>,
  );
}

describe('AppFooter', () => {
  it('renders a route-aware legend derived from effective bindings', () => {
    mockPathname.current = '/prs';
    renderFooter();

    // list route preset includes list.down (j), list.up (k), list.open (Enter).
    expect(screen.getByText('Next')).toBeInTheDocument();
    expect(screen.getByText('Previous')).toBeInTheDocument();
    expect(screen.getByText('Open')).toBeInTheDocument();
  });

  it('lets a page override the left region via FooterLeftSlot', () => {
    mockPathname.current = '/prs';
    renderFooter(<FooterLeftSlot>CUSTOM-LEFT</FooterLeftSlot>);

    expect(screen.getByText('CUSTOM-LEFT')).toBeInTheDocument();
    // Default legend should no longer be rendered when a page overrides left.
    expect(screen.queryByText('Next')).not.toBeInTheDocument();
  });

  it('projects page content into the center region via FooterCenterSlot', () => {
    mockPathname.current = '/prs';
    renderFooter(
      <FooterCenterSlot>{'FILTER:OPEN // SORT:UPDATED'}</FooterCenterSlot>,
    );

    expect(screen.getByText('FILTER:OPEN // SORT:UPDATED')).toBeInTheDocument();
  });

  it('omits the brand strip and exposes the status label on hover only', () => {
    renderFooter();
    expect(
      screen.queryByText(/BISTRO \/\/ TECHNICAL ATELIER/i),
    ).not.toBeInTheDocument();

    // Status indicator is rendered as an accessible hover-card trigger.
    const statusTrigger = screen.getByLabelText(/Sync status:/);
    expect(statusTrigger).toBeInTheDocument();
    expect(statusTrigger).not.toHaveAttribute('title');
  });

  it('includes the next scheduled sync countdown in the status label', () => {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'ready',
      lastSyncAt: new Date().toISOString(),
    }));
    settingsStore.setState((s) => ({
      ...s,
      sync: { ...s.sync, autoSync: true, syncInterval: 90 },
    }));

    renderFooter();

    expect(
      screen.getByLabelText(/Sync status: OPERATIONAL \/\/ Next refresh in/),
    ).toBeInTheDocument();
  });

  it('shows a spinning loader while sync is running', () => {
    appStore.setState((s) => ({ ...s, syncStatus: 'syncing' }));

    renderFooter();

    const statusTrigger = screen.getByLabelText(
      'Sync status: SYNCING // Syncing now',
    );
    expect(statusTrigger.querySelector('svg')).toHaveClass('animate-spin');
  });

  it('shows diff-view legend on diff route', () => {
    mockPathname.current = '/prs/pr-1/diff';
    renderFooter();
    // diff preset includes detail.scrollDown (j) and global.escape
    expect(screen.getByText('Back / Cancel')).toBeInTheDocument();
  });

  it('opens shortcut help with the diff route as its source', async () => {
    const user = userEvent.setup();
    mockPathname.current = '/prs/pr-1/diff';
    renderFooter();

    await user.click(screen.getByRole('button', { name: 'Shortcuts' }));

    expect(screen.getByTestId('help-source')).toHaveTextContent('diff');
  });

  // AppFooter has NO RouterSafe — it's only rendered under authedRoute
  // (inside RouterProvider). The old "renders without RouterProvider" test is
  // superseded by the auth-guards test that confirms AppFooter is absent on
  // /login. Verify the component has no RouterSafe class.
  it('AppFooter renders with router context mocked (no RouterSafe needed)', () => {
    // With the mock in place this should render cleanly.
    renderFooter();
    const footer = document.querySelector('footer');
    expect(footer).toBeInTheDocument();
    // Shortcuts button appears (router mock allows useNavigate).
    expect(screen.getByText('Shortcuts')).toBeInTheDocument();
    expect(screen.getByText('Settings')).toBeInTheDocument();
  });
});
