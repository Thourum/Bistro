// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { AppFooter } from '../../src/renderer/components/AppFooter';
import {
  FooterCenterSlot,
  FooterLeftSlot,
  FooterSlotProvider,
} from '../../src/renderer/components/FooterSlot';
import { appStore } from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import { initialUIState } from '../../src/renderer/store/ui';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

expect.extend(matchers);

beforeEach(() => {
  appStore.setState(() => ({ ...initialUIState, mode: 'list' }));
  settingsStore.setState(() => DEFAULT_SETTINGS);
});

function renderFooter(children: React.ReactNode = null) {
  return render(
    <FooterSlotProvider>
      {children}
      <AppFooter />
    </FooterSlotProvider>,
  );
}

describe('AppFooter', () => {
  it('renders a mode-aware legend derived from effective bindings', () => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
    renderFooter();

    // list mode preset includes list.down (j), list.up (k), list.open (Enter).
    expect(screen.getByText('Next')).toBeInTheDocument();
    expect(screen.getByText('Previous')).toBeInTheDocument();
    expect(screen.getByText('Open')).toBeInTheDocument();
  });

  it('lets a page override the left region via FooterLeftSlot', () => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
    renderFooter(<FooterLeftSlot>CUSTOM-LEFT</FooterLeftSlot>);

    expect(screen.getByText('CUSTOM-LEFT')).toBeInTheDocument();
    // Default legend should no longer be rendered when a page overrides left.
    expect(screen.queryByText('Next')).not.toBeInTheDocument();
  });

  it('projects page content into the center region via FooterCenterSlot', () => {
    appStore.setState((s) => ({ ...s, mode: 'list' }));
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
});
