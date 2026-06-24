import * as React from 'react';

/** Hard-square status signal (CI / sync / review). Never a circle. */
export interface StatusDotProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** @default "unknown" */
  status?: 'success' | 'failure' | 'running' | 'pending' | 'unknown';
  /** Edge length in px. @default 12 */
  size?: number;
  /** Force the pulsing heartbeat. */
  pulse?: boolean;
}

export function StatusDot(props: StatusDotProps): React.JSX.Element;
