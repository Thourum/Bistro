import * as React from 'react';

/** Mark + wordmark locked together — the primary logo lockup. */
export interface BistroLockupProps
  extends React.HTMLAttributes<HTMLSpanElement> {
  /** Wordmark cap size in px; the mark scales off it. @default 32 */
  size?: number;
  /** @default "horizontal" */
  orientation?: 'horizontal' | 'stacked';
  /** @default "default" */
  variant?: 'default' | 'inverse' | 'mono';
  /** Override wordmark color. */
  color?: string;
}

export function BistroLockup(props: BistroLockupProps): React.JSX.Element;
