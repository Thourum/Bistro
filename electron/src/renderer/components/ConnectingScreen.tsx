// Shared loading screens — used by router pending states and App.tsx init gate.

import type React from 'react';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useEngineScreenData } from '../hooks/useEngineScreenData';
import { openEngineLogFolder } from '../services/engine';
import { findActionKeys } from '../utils/legendPresets';

const SLOW_LOG_LINES = 3;

interface KeyHintProps {
  readonly actionId: string;
}

/** Effective key for `actionId`, or nothing when the user unbound it. */
export function KeyHint({ actionId }: KeyHintProps): React.ReactElement | null {
  const keys = findActionKeys(useEffectiveBindings(), [actionId]);
  if (keys === undefined) return null;
  return (
    <kbd className="ml-3 font-mono text-[10px] px-1.5 py-0.5 bg-surface border border-outline-variant text-on-surface-variant">
      {keys}
    </kbd>
  );
}

/** Shown while the WebSocket auth check is in-flight. */
export function ConnectingScreen(): React.ReactElement {
  const { elapsed, isSlow, logs } = useEngineScreenData(SLOW_LOG_LINES);
  if (!isSlow) {
    return (
      <div className="w-full h-screen bg-surface flex items-center justify-center">
        <div className="text-center space-y-4">
          <h1 className="text-2xl font-black tracking-tighter text-on-surface uppercase">
            BISTRO
          </h1>
          <p className="text-sm text-on-surface-variant font-mono animate-pulse uppercase tracking-widest">
            Connecting...
          </p>
        </div>
      </div>
    );
  }
  return (
    <div className="w-full h-screen bg-surface flex items-center justify-center px-6">
      <div className="w-full max-w-md flex flex-col items-center gap-6 text-center">
        <h1 className="text-2xl font-black tracking-tighter text-on-surface uppercase">
          BISTRO
        </h1>
        <p className="font-mono text-sm text-primary">CONNECTING… {elapsed}</p>
        <div className="w-full text-left border border-outline-variant bg-surface-container-lowest">
          <p className="px-4 py-3 text-sm text-on-surface-variant">
            Taking longer than usual. This can happen after an update or on
            first launch.
          </p>
          <pre className="overflow-x-auto rounded-lg bg-surface-container p-4 font-mono text-xs text-on-surface-variant">
            {logs.join('\n') || 'Waiting for engine log output…'}
          </pre>
        </div>
        <button
          type="button"
          onClick={openEngineLogFolder}
          className="rounded-lg border border-outline-variant px-4 py-2 text-sm text-on-surface"
        >
          Open log folder
          <KeyHint actionId="connecting.openLogs" />
        </button>
      </div>
    </div>
  );
}

/** Shown while settings are loading from disk on first launch. */
export function InitialisingScreen(): React.ReactElement {
  return (
    <div className="w-full h-screen bg-surface flex items-center justify-center">
      <p className="text-sm text-on-surface-variant font-mono uppercase tracking-widest">
        Initialising...
      </p>
    </div>
  );
}
