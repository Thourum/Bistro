import {
  type EngineStatus,
  SLOW_ENGINE_MS,
} from '@renderer/types/electron/engine';
import { useEffect, useState } from 'react';
import { addToast } from '../components/ToastContainer';
import { getEngineLogTail } from '../services/engine';
import { appStore } from '../store';
import { useStore } from './useStore';

const TIMER_TICK_MS = 1_000;
const SECONDS_PER_MINUTE = 60;

interface EngineScreenData {
  readonly status: EngineStatus | null;
  readonly elapsed: string;
  readonly isSlow: boolean;
  readonly logs: readonly string[];
}

interface LogTailOptions {
  readonly lines: number;
  readonly shouldFetch: boolean;
  readonly shouldPoll: boolean;
}

function formatElapsed(elapsedSeconds: number): string {
  const minutes = String(Math.floor(elapsedSeconds / SECONDS_PER_MINUTE));
  const seconds = String(elapsedSeconds % SECONDS_PER_MINUTE);
  return `${minutes.padStart(2, '0')}:${seconds.padStart(2, '0')}`;
}

/** One read fault is toasted and stops polling, rather than toasting every tick. */
function useEngineLogTail({
  lines,
  shouldFetch,
  shouldPoll,
}: LogTailOptions): readonly string[] {
  const [logs, setLogs] = useState<readonly string[]>([]);
  useEffect(() => {
    if (!shouldFetch) return;
    let isActive = true;
    let hasFailed = false;
    const refresh = (): void => {
      if (hasFailed) return;
      void getEngineLogTail(lines).match(
        (tail) => {
          if (isActive) setLogs(tail);
        },
        (fault) => {
          if (!isActive || hasFailed) return;
          hasFailed = true;
          addToast({
            type: 'error',
            message: fault.message,
            dismissible: true,
          });
        },
      );
    };
    refresh();
    const interval = shouldPoll
      ? window.setInterval(refresh, TIMER_TICK_MS)
      : null;
    return () => {
      isActive = false;
      if (interval !== null) window.clearInterval(interval);
    };
  }, [lines, shouldFetch, shouldPoll]);
  return logs;
}

/** Engine status, startup timer and log tail for the slow and crashed screens. */
export function useEngineScreenData(lines: number): EngineScreenData {
  const status = useStore(appStore, (state) => state.engineStatus);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (status?.state !== 'starting') return;
    const interval = window.setInterval(
      () => setNow(Date.now()),
      TIMER_TICK_MS,
    );
    return () => window.clearInterval(interval);
  }, [status?.state]);
  const elapsedSeconds = Math.max(
    0,
    Math.floor((now - (status?.startedAt ?? now)) / TIMER_TICK_MS),
  );
  const isSlow =
    status?.state === 'starting' && now - status.startedAt >= SLOW_ENGINE_MS;
  const logs = useEngineLogTail({
    lines,
    shouldFetch: isSlow || status?.state === 'crashed',
    shouldPoll: isSlow,
  });
  return { status, elapsed: formatElapsed(elapsedSeconds), isSlow, logs };
}
