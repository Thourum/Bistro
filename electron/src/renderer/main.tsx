// Sentry must initialize before app modules can throw during import/evaluation.
import './sentry';
import { Fault, ResultAsync } from '@thourum/fault';
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import { ErrorBoundary } from './components/ErrorBoundary';
import { createLogger } from './utils/logger';

import '@fontsource-variable/geist/index.css';
import '@fontsource-variable/geist-mono/index.css';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';
import '@fontsource/jetbrains-mono/600.css';
import '@fontsource/jetbrains-mono/700.css';
import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/inter/600.css';
import '@fontsource/inter/700.css';
import '@fontsource/fira-code/400.css';
import '@fontsource/fira-code/500.css';
import '@fontsource/ibm-plex-mono/400.css';
import '@fontsource/ibm-plex-mono/500.css';
import '@fontsource/ibm-plex-sans/400.css';
import '@fontsource/ibm-plex-sans/500.css';

import './styles/globals.css';

function renderApp(): void {
  const root = document.getElementById('root');
  if (root === null) {
    document.body.textContent =
      'Unable to start Bistro: root element is missing.';
    return;
  }
  ReactDOM.createRoot(root).render(
    <React.StrictMode>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </React.StrictMode>,
  );
}

if (new URLSearchParams(location.search).get('bench_collector')) {
  // Install the listener before mounting so the first cached list cannot race
  // the benchmark chunk download.
  void ResultAsync.fromThrowable(
    async () => {
      const { startBench } = await import('./bench');
      startBench();
    },
    (cause) =>
      new Fault('Could not initialize benchmark')
        .withTag('BENCH_ERROR')
        .withCause(cause),
  )().match(renderApp, (fault) => {
    createLogger('bench').error(
      { err: fault.message },
      'benchmark initialization failed',
    );
    renderApp();
  });
} else {
  renderApp();
}
