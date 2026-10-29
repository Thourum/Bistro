// Sentry must initialize before app modules can throw during import/evaluation.
import './sentry';
// Initialize the native runtime before the renderer migrates its service calls.
import '@wailsio/runtime';
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

const benchSearch = new URLSearchParams(location.search);
const isKeysRequested = benchSearch.has('bench_keys');

function stopBenchStartup(): void {
  document.body.textContent =
    'Keyboard benchmark configuration is invalid; Bistro was not started.';
}

if (benchSearch.get('bench_collector') || isKeysRequested) {
  // Install the listener before mounting so the first cached list cannot race
  // the benchmark chunk download.
  void ResultAsync.fromThrowable(
    async () => {
      const { startBench } = await import('./bench');
      return startBench();
    },
    (cause) =>
      new Fault('Could not initialize benchmark')
        .withTag('BENCH_ERROR')
        .withCause(cause),
  )().match(
    (started) => {
      if (isKeysRequested && !started) stopBenchStartup();
      else renderApp();
    },
    (fault) => {
      createLogger('bench').error(
        { err: fault.message },
        'benchmark initialization failed',
      );
      if (isKeysRequested) stopBenchStartup();
      else renderApp();
    },
  );
} else {
  renderApp();
}
