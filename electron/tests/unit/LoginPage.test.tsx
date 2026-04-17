// @vitest-environment jsdom

import * as matchers from '@testing-library/jest-dom/matchers';
import { render, screen } from '@testing-library/react';
import { expect } from 'vitest';
import { LoginPage } from '../../src/renderer/pages/LoginPage';

expect.extend(matchers);

describe('LoginPage', () => {
  it('renders BISTRO header', () => {
    render(<LoginPage />);
    expect(screen.getByText(/BISTRO/i)).toBeInTheDocument();
  });

  it('renders tagline', () => {
    render(<LoginPage />);
    expect(screen.getByText(/Connect your platform/i)).toBeInTheDocument();
  });

  it('renders all three provider buttons', () => {
    render(<LoginPage />);
    expect(screen.getByText('GitHub')).toBeInTheDocument();
    expect(screen.getByText('GitLab')).toBeInTheDocument();
    expect(screen.getByText('Gitea')).toBeInTheDocument();
  });

  it('renders keyboard hints', () => {
    render(<LoginPage />);
    expect(screen.getByText('G')).toBeInTheDocument(); // GitHub
    expect(screen.getByText('L')).toBeInTheDocument(); // GitLab
    expect(screen.getByText('T')).toBeInTheDocument(); // Gitea
  });

  it('renders version info in footer', () => {
    render(<LoginPage />);
    expect(screen.getByText(/v\d+\.\d+\.\d+/i)).toBeInTheDocument();
  });

  it('displays live sync status', () => {
    render(<LoginPage />);
    // Status text shows live syncStatus (not hardcoded "Ready")
    // Value depends on WebSocket state in test env (connecting, error, disconnected)
    expect(screen.getByText(/Status:/i)).toBeInTheDocument();
  });
});
