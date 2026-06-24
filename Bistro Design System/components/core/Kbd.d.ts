import * as React from 'react';

/** A single bound key, rendered as a boxed cap. The atom of Bistro's chrome. */
export interface KbdProps extends React.HTMLAttributes<HTMLElement> {
  /** @default "default" */
  tone?: 'default' | 'muted' | 'primary';
  children?: React.ReactNode;
}

export function Kbd(props: KbdProps): React.JSX.Element;
