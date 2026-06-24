import * as React from 'react';

/** The "BISTRO" logotype — Geist 800, uppercase, tight tracking. The established wordmark. */
export interface BistroWordmarkProps
  extends React.HTMLAttributes<HTMLElement> {
  /** Font size in px. @default 32 */
  size?: number;
  /** Text color. @default near-black on-surface */
  color?: string;
  /** Element to render. @default "span" */
  as?: keyof React.JSX.IntrinsicElements;
}

export function BistroWordmark(props: BistroWordmarkProps): React.JSX.Element;
