import * as React from 'react';

/** Terse action-feedback bar — dark, mono, square. Confirms, never celebrates. */
export interface ToastProps extends React.HTMLAttributes<HTMLDivElement> {
  message: string;
  /** @default "success" */
  tone?: 'success' | 'error' | 'info' | 'warning';
  /** Optional trailing key hint (e.g. "Z" to undo). */
  keyHint?: string;
}

export function Toast(props: ToastProps): React.JSX.Element;
