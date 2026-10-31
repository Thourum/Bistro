import type { ThreadLifecycle } from '../types';
import type { StatusKind } from './StatusSquare';

/** Actionable threads sort first; unknown and outdated states stay neutral. */
export const THREAD_LIFECYCLE = {
  open: { label: 'Open', square: 'running', order: 0 },
  outdated: { label: 'Outdated', square: 'neutral', order: 1 },
  unknown: { label: 'Unknown', square: 'neutral', order: 2 },
  resolved: { label: 'Resolved', square: 'success', order: 3 },
} as const satisfies Record<
  ThreadLifecycle,
  {
    readonly label: string;
    readonly square: StatusKind;
    readonly order: number;
  }
>;
