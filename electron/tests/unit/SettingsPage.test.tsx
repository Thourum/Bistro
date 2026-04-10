// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, vi } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { SettingsPage } from '../../src/renderer/pages/SettingsPage';

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ send: vi.fn(), request: vi.fn() }),
}));

expect.extend(matchers);

describe('SettingsPage', () => {
  it('renders settings title', () => {
    render(<SettingsPage />);
    expect(screen.getByText(/Settings/i)).toBeInTheDocument();
  });

  it('renders all three tabs', () => {
    render(<SettingsPage />);
    expect(screen.getByRole('tab', { name: /Appearance/i })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /Sync/i })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /Keyboard/i })).toBeInTheDocument();
  });

  it('renders appearance tab content by default', () => {
    render(<SettingsPage />);
    expect(screen.getByLabelText(/Dark mode/i)).toBeInTheDocument();
  });

  it('switches to sync tab on click', async () => {
    render(<SettingsPage />);
    const syncTab = screen.getByRole('tab', { name: /Sync/i });
    await userEvent.click(syncTab);
    expect(screen.getByLabelText(/Auto-sync/i)).toBeInTheDocument();
  });

  it('switches to keyboard tab on click', async () => {
    render(<SettingsPage />);
    const keyboardTab = screen.getByRole('tab', { name: /Keyboard/i });
    await userEvent.click(keyboardTab);
    expect(screen.getByRole('button', { name: /View All Shortcuts/i })).toBeInTheDocument();
  });

  it('renders within SettingsLayout', () => {
    const { container } = render(<SettingsPage />);
    expect(container.querySelector('[data-testid="settings-header"]')).toBeInTheDocument();
  });

  it('renders description text', () => {
    render(<SettingsPage />);
    expect(screen.getByText(/Configure Bistro to your preferences/i)).toBeInTheDocument();
  });
});
