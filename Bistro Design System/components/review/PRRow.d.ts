import * as React from 'react';

/**
 * One pull request row in the inbox — the heart of Bistro.
 *
 * @startingPoint section="Review" subtitle="Keyboard-navigable PR inbox row" viewport="760x56"
 */
export interface PRRowProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'title'> {
  number: number | string;
  title: string;
  author: string;
  /** Pre-formatted relative time, e.g. "2m ago". */
  time: string;
  /** CI state → status square color. @default "unknown" */
  ci?: 'success' | 'failure' | 'running' | 'pending' | 'unknown';
  /** Selected (keyboard cursor) row. */
  selected?: boolean;
}

export function PRRow(props: PRRowProps): React.JSX.Element;
