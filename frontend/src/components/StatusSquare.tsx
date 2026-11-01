import type React from 'react';

/**
 * Status is encoded as a hard colored square + a word — never a circle, dot, or
 * emoji. This is the single most identifying status signal in the brand:
 * `w-3 h-3` in lists, `w-2 h-2` in the system bar. Running/pending pulses.
 *
 * Mirrors the design system's `StatusDot` primitive (which, despite its name,
 * is always square) and the inline pattern already used by `PRListTable`.
 */
export type StatusKind =
  | 'success'
  | 'failure'
  | 'running'
  | 'pending'
  | 'neutral';

const TONE: Record<StatusKind, string> = {
  success: 'bg-green-500',
  failure: 'bg-error',
  running: 'bg-yellow-500',
  pending: 'bg-yellow-500',
  neutral: 'bg-outline-variant',
};

interface StatusSquareProps {
  status: StatusKind;
  /** Tailwind size utility pair. Lists use `w-3 h-3`; the system bar `w-2 h-2`. */
  size?: string;
  /** Accessible label + tooltip, e.g. `CI SUCCESS`. */
  label: string;
  className?: string;
}

export function StatusSquare({
  status,
  size = 'w-3 h-3',
  label,
  className = '',
}: StatusSquareProps): React.ReactElement {
  const pulse =
    status === 'running' || status === 'pending'
      ? 'motion-safe:animate-pulse'
      : '';
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={`${size} inline-block shrink-0 ${TONE[status]} ${pulse} ${className}`.trim()}
    />
  );
}
