// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import { appStore } from '../../src/renderer/store';
import ToastContainer from '../../src/renderer/components/ToastContainer';

describe('ToastContainer', () => {
  beforeEach(() => {
    appStore.setState((s) => ({ ...s, toasts: [] }));
  });

  it('renders nothing when toasts is empty', () => {
    const { container } = render(<ToastContainer />);
    expect(container.querySelector('[data-toast]')).toBeNull();
  });

  it('renders error toast with message', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't1', type: 'error', message: 'Something broke', dismissible: true }],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('Something broke')).toBeInTheDocument();
  });

  it('renders success toast with message', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't2', type: 'success', message: 'All good', dismissible: true }],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('All good')).toBeInTheDocument();
  });

  it('renders multiple toasts', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [
        { id: 't1', type: 'error', message: 'Error 1', dismissible: true },
        { id: 't2', type: 'info', message: 'Info 1', dismissible: true },
      ],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('Error 1')).toBeInTheDocument();
    expect(screen.getByText('Info 1')).toBeInTheDocument();
  });

  it('dismisses toast when close button clicked', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't1', type: 'error', message: 'Dismiss me', dismissible: true }],
    }));
    render(<ToastContainer />);
    const closeBtn = screen.getByRole('button', { name: /dismiss/i });
    fireEvent.click(closeBtn);
    expect(appStore.state.toasts).toHaveLength(0);
  });

  it('auto-dismisses success toasts after 5 seconds', () => {
    vi.useFakeTimers();
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't1', type: 'success', message: 'Done', dismissible: true }],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('Done')).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(5000); });
    expect(appStore.state.toasts).toHaveLength(0);
    vi.useRealTimers();
  });
});
