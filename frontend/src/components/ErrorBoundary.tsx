import React from 'react';
import { Sentry } from '../sentry';

type ErrorBoundaryState = {
  error: Error | null;
};

export class ErrorBoundary extends React.Component<
  React.PropsWithChildren,
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo): void {
    Sentry.captureException(error, {
      tags: { component: 'react-renderer' },
      extra: { componentStack: errorInfo.componentStack },
    });
  }

  render(): React.ReactNode {
    if (this.state.error) {
      return (
        <div className="flex min-h-screen items-center justify-center bg-zinc-950 p-8 text-zinc-100">
          <div className="max-w-md rounded-lg border border-red-500/30 bg-zinc-900 p-6 shadow-2xl">
            <p className="text-sm font-medium uppercase tracking-[0.2em] text-red-300">
              Bistro crashed
            </p>
            <h1 className="mt-3 text-2xl font-semibold">Restart the app</h1>
            <p className="mt-3 text-sm leading-6 text-zinc-300">
              The local review client hit an unexpected UI error. Your synced PR
              cache and queued review actions remain on this machine.
            </p>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
