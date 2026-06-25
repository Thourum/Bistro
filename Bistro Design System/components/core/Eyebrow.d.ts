import * as React from 'react';

/** Uppercase monospace caption label. The connective tissue of Bistro's UI. */
export interface EyebrowProps extends React.HTMLAttributes<HTMLElement> {
  /** @default "muted" */
  tone?: 'muted' | 'primary' | 'strong';
  /** Element to render. @default "span" */
  as?: keyof React.JSX.IntrinsicElements;
  children?: React.ReactNode;
}

export function Eyebrow(props: EyebrowProps): React.JSX.Element;
