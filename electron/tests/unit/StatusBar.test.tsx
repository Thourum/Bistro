// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { appStore } from '../../src/renderer/store';
import StatusBar from '../../src/renderer/components/StatusBar';

describe('StatusBar', () => {
  beforeEach(() => {
    appStore.setState((s) => ({
      ...s,
      mode: 'list',
      syncStatus: 'connected',
      syncError: null,
      lastSyncAt: null,
      isOffline: false,
      confirmingAction: null,
    }));
  });

  it('shows offline indicator when isOffline is true', () => {
    appStore.setState((s) => ({ ...s, isOffline: true }));
    render(<StatusBar />);
    expect(screen.getByText(/offline/i)).toBeInTheDocument();
  });

  it('shows error state with message', () => {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'error',
      syncError: 'Connection failed',
    }));
    render(<StatusBar />);
    expect(screen.getByText(/Connection failed/i)).toBeInTheDocument();
  });

  it('shows rate_limited state', () => {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'rate_limited',
      syncError: 'Rate limited until 12:00',
    }));
    render(<StatusBar />);
    expect(screen.getByText(/rate limited/i)).toBeInTheDocument();
  });

  it('shows connected state normally', () => {
    appStore.setState((s) => ({ ...s, syncStatus: 'connected' }));
    render(<StatusBar />);
    expect(screen.getByText(/connected/i)).toBeInTheDocument();
  });

  it('shows synced time when ready', () => {
    const now = new Date().toISOString();
    appStore.setState((s) => ({ ...s, syncStatus: 'ready', lastSyncAt: now }));
    render(<StatusBar />);
    expect(screen.getByText(/synced at/i)).toBeInTheDocument();
  });
});
