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
    deviceCode: null,
    verificationUri: null,
    authLoading: false,
    authError: null,
    isAuthenticated: false,
  }));
});

describe('LoginPage — OAuth device flow', () => {
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

  it('shows device code UI when deviceCode and verificationUri are set', () => {
    appStore.setState((s) => ({
      ...s,
      deviceCode: 'ABCD-1234',
      verificationUri: 'https://github.com/login/device',
      authLoading: true,
    }));
    render(<LoginPage />);
    expect(screen.getByText('ABCD-1234')).toBeInTheDocument();
    expect(screen.getByText(/Enter this code at GitHub/i)).toBeInTheDocument();
    expect(screen.getByText(/Waiting for authorization/i)).toBeInTheDocument();
  });

  it('shows a link to the verification URI', () => {
    appStore.setState((s) => ({
      ...s,
      deviceCode: 'ABCD-1234',
      verificationUri: 'https://github.com/login/device',
      authLoading: true,
    }));
    render(<LoginPage />);
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', 'https://github.com/login/device');
  });

  it('hides provider buttons when device code is showing', () => {
    appStore.setState((s) => ({
      ...s,
      deviceCode: 'ABCD-1234',
      verificationUri: 'https://github.com/login/device',
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

  it('shows provider buttons when no device code is active', () => {
    render(<LoginPage />);
    expect(screen.getByText('GitHub')).toBeInTheDocument();
    expect(screen.getByText('GitLab')).toBeInTheDocument();
  });
});
