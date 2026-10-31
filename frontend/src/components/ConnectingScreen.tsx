import { openLogFolder } from '../services/logs';
import { ActionKeyHint } from './ui/action-key-hint';
import { Button } from './ui/button';

/** Auth bootstrap gate; the engine runs in-process. */
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
        <Button
          variant="outline"
          onClick={() => {
            void openLogFolder();
          }}
        >
          Open log folder <ActionKeyHint actionId="connecting.openLogs" />
        </Button>
      </div>
    </div>
  );
}

export function InitialisingScreen(): React.ReactElement {
  return (
    <div className="w-full h-screen bg-surface flex items-center justify-center">
      <p className="text-sm text-on-surface-variant font-mono uppercase tracking-widest">
        Initialising...
      </p>
    </div>
  );
}
