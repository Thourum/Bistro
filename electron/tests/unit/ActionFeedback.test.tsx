// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import ActionFeedback from '../../src/renderer/components/ActionFeedback';

describe('ActionFeedback', () => {
  it('renders nothing when no pendingAction or actionResult', () => {
    const { container } = render(
      <ActionFeedback pendingAction={null} actionResult={null} onDismiss={vi.fn()} />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders loading state when pendingAction is set', () => {
    render(
      <ActionFeedback
        pendingAction={{ type: 'approve', prId: 'pr-1', startedAt: Date.now() }}
        actionResult={null}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText(/approving/i)).toBeInTheDocument();
  });

  it('renders success state when actionResult.success is true', () => {
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: true, message: 'PR approved' }}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText('PR approved')).toBeInTheDocument();
    expect(screen.getByText('✓')).toBeInTheDocument();
  });

  it('renders error state and Dismiss button when actionResult.success is false', () => {
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: false, message: 'Forbidden' }}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText('Forbidden')).toBeInTheDocument();
    expect(screen.getByText('✗')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /dismiss/i })).toBeInTheDocument();
  });

  it('auto-dismisses successful result after 3 seconds', () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: true, message: 'Done' }}
        onDismiss={onDismiss}
      />
    );
    expect(onDismiss).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(3000); });
    expect(onDismiss).toHaveBeenCalled();
    vi.useRealTimers();
  });

  it('does not auto-dismiss failed results', () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: false, message: 'Error' }}
        onDismiss={onDismiss}
      />
    );
    act(() => { vi.advanceTimersByTime(5000); });
    expect(onDismiss).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  it('renders request_changes label correctly', () => {
    render(
      <ActionFeedback
        pendingAction={{ type: 'request_changes', prId: 'pr-1', startedAt: Date.now() }}
        actionResult={null}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.getByText(/requesting changes/i)).toBeInTheDocument();
  });
});

describe('ActionFeedback retry', () => {
  it('renders retry button on failure when onRetry is provided', () => {
    const onRetry = vi.fn();
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: false, message: 'Failed' }}
        onDismiss={vi.fn()}
        onRetry={onRetry}
      />
    );
    const retryBtn = screen.getByRole('button', { name: /retry/i });
    fireEvent.click(retryBtn);
    expect(onRetry).toHaveBeenCalled();
  });

  it('does not render retry button when onRetry is not provided', () => {
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: false, message: 'Failed' }}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull();
  });
});
