// Shared loading screens — used by router pending states and App.tsx init gate.

import type React from 'react';

/** Shown while the WebSocket auth check is in-flight. */
export function ConnectingScreen(): React.ReactElement {
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
