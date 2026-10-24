import type React from 'react';
import { useState } from 'react';
import type { EngineStatus } from '../../types/engine';
import { useEngineScreenData } from '../hooks/useEngineScreenData';
import { AuthLayout } from '../layouts/AuthLayout';
import {
  openEngineLogFolder,
  reportEngineCrash,
  restartEngine,
} from '../services/engine';
import { ActionKeyHint } from './ui/action-key-hint';
import { Button } from './ui/button';

const CRASH_LOG_LINES = 60;

interface CrashAction {
  readonly actionId: string;
  readonly label: string;
  readonly onClick: () => void;
}

function formatExit(status: EngineStatus | null): string {
  return status?.signal !== null && status?.signal !== undefined
    ? `Signal: ${status.signal}`
    : `Exit code: ${status?.code ?? 'unknown'}`;
}

/** Terminal engine failure: the engine exited and will not restart on its own. */
export function EngineCrashedScreen(): React.ReactElement {
  const { status, logs } = useEngineScreenData(CRASH_LOG_LINES);
  // Disabled until the main process pushes a new status for this restart.
  const [restartedFrom, setRestartedFrom] = useState<EngineStatus | null>(null);
  const isRestarting = restartedFrom !== null && restartedFrom === status;
  const actions: readonly CrashAction[] = [
    {
      actionId: 'connecting.restart',
      label: 'Restart engine',
      onClick: () => {
        setRestartedFrom(status);
        void restartEngine().mapErr(() => setRestartedFrom(null));
      },
    },
    {
      actionId: 'connecting.openLogs',
      label: 'Open log folder',
      onClick: openEngineLogFolder,
    },
    {
      actionId: 'connecting.report',
      label: 'Report on GitHub',
      onClick: reportEngineCrash,
    },
  ].filter(
    (action) =>
      action.actionId !== 'connecting.restart' || status?.isSpawned === true,
  );

  // AuthLayout grows with flex-1; RootShell's route slot is not a flex box.
  return (
    <div className="flex h-full">
      <AuthLayout>
        <div className="p-8 space-y-6">
          <div role="alert" className="space-y-2">
            <p className="text-sm font-black tracking-tighter text-on-surface uppercase">
              BISTRO
            </p>
            <h1 className="text-2xl font-bold text-on-surface">
              The sync engine stopped
            </h1>
            <p className="font-mono text-sm text-error">{formatExit(status)}</p>
          </div>
          <pre className="max-h-64 overflow-auto bg-surface-container p-4 font-mono text-xs text-on-surface-variant">
            {logs.join('\n') || 'No engine log output was captured.'}
          </pre>
          <div className="flex flex-wrap gap-3">
            {actions.map((action) => (
              <Button
                key={action.actionId}
                variant="outline"
                onClick={action.onClick}
                disabled={
                  action.actionId === 'connecting.restart' && isRestarting
                }
              >
                {action.label}
                <ActionKeyHint actionId={action.actionId} />
              </Button>
            ))}
          </div>
          <p className="text-xs text-on-surface-variant">
            Opens a public GitHub issue with the last 60 log lines. Check them
            before you submit.
          </p>
        </div>
      </AuthLayout>
    </div>
  );
}
