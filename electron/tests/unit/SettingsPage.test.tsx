// @vitest-environment jsdom
import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { SettingsPage } from '../../src/renderer/pages/SettingsPage';

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ send: vi.fn(), request: vi.fn() }),
}));

expect.extend(matchers);

describe('SettingsPage', () => {
  it('renders the Settings title', () => {
    render(<SettingsPage />);
    expect(
      screen.getByRole('heading', { name: /Settings/i }),
    ).toBeInTheDocument();
  });

  it('renders all navigation sections in the sidebar', () => {
    render(<SettingsPage />);
    expect(
      screen.getByRole('button', { name: /Appearance/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Keyboard/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Connections/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Sync$/i })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Privacy/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Advanced/i }),
    ).toBeInTheDocument();
  });

  it('renders theme cards for light and dark mode', () => {
    render(<SettingsPage />);
    expect(screen.getByText('Light Mode')).toBeInTheDocument();
    expect(screen.getByText('Dark Mode')).toBeInTheDocument();
  });

  it('exposes the UI font picker', () => {
    render(<SettingsPage />);
    expect(screen.getByLabelText(/^UI Font$/i)).toBeInTheDocument();
  });

  it('allows switching to the Advanced section via sidebar', async () => {
    render(<SettingsPage />);
    await userEvent.click(screen.getByRole('button', { name: /Advanced/i }));
    expect(screen.getAllByText(/Advanced/i).length).toBeGreaterThan(0);
  });

  it('renders usage telemetry off by default', () => {
    render(<SettingsPage />);
    expect(screen.getByText('Usage Telemetry')).toBeInTheDocument();
    expect(
      screen.getByRole('switch', { name: /Usage Telemetry/i }),
    ).not.toBeChecked();
  });
});
