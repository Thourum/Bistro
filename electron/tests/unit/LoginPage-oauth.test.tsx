// @vitest-environment jsdom
import { render, screen, fireEvent } from '@testing-library/react';
import { expect, vi, beforeEach } from 'vitest';
import * as matchers from '@testing-library/jest-dom/matchers';
import { LoginPage } from '../../src/renderer/pages/LoginPage';
import { appStore } from '../../src/renderer/store';

expect.extend(matchers);

// Mock useWebSocket so send is a spy
const mockSend = vi.fn();
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ send: mockSend, request: vi.fn(), wsRef: { current: null } }),
}));

beforeEach(() => {
  mockSend.mockClear();
  // Reset store to initial state
  appStore.setState((s) => ({
    ...s,
    authLoading: false,
    authError: null,
    isAuthenticated: false,
  }));
});

describe('LoginPage — OAuth auth code flow', () => {
  it('sends AUTH_START with provider when GitHub button is clicked', () => {
    render(<LoginPage />);
    fireEvent.click(screen.getByText('GitHub'));
    expect(mockSend).toHaveBeenCalledWith('AUTH_START', { provider: 'github' });
  });

  it('does not set isAuthenticated optimistically on click', () => {
    render(<LoginPage />);
    fireEvent.click(screen.getByText('GitHub'));
    expect(appStore.state.isAuthenticated).toBe(false);
  });

  it('shows waiting UI when authLoading is true', () => {
    appStore.setState((s) => ({
      ...s,
      authLoading: true,
    }));
    render(<LoginPage />);
    expect(screen.getByText(/Waiting for authorization/i)).toBeInTheDocument();
    expect(screen.getByText(/Complete the sign-in in your browser/i)).toBeInTheDocument();
  });

  it('hides provider buttons when authLoading is true', () => {
    appStore.setState((s) => ({
      ...s,
      authLoading: true,
    }));
    render(<LoginPage />);
    expect(screen.queryByText('GitHub')).not.toBeInTheDocument();
  });

  it('shows auth error when authError is set', () => {
    appStore.setState((s) => ({
      ...s,
      authError: 'device code expired — please restart the login flow',
    }));
    render(<LoginPage />);
    expect(screen.getByText(/device code expired/i)).toBeInTheDocument();
  });

  it('shows provider buttons when no auth is in progress', () => {
    render(<LoginPage />);
    expect(screen.getByText('GitHub')).toBeInTheDocument();
    expect(screen.getByText('GitLab')).toBeInTheDocument();
  });
});
