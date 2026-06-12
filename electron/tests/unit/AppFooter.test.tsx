// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AppFooter } from '../../src/renderer/components/AppFooter';
import {
  FooterCenterSlot,
  FooterLeftSlot,
  FooterSlotProvider,
} from '../../src/renderer/components/FooterSlot';
import { appStore } from '../../src/renderer/store';

expect.extend(matchers);

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

    // Status indicator is rendered as an accessible image with a sync label.
    const statusDot = screen.getByRole('img', { name: /Sync status:/ });
    expect(statusDot).toBeInTheDocument();
  });
});
