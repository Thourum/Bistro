import * as React from 'react';

/**
 * Bistro Button — square, technical, keyboard-forward.
 *
 * @startingPoint section="Core" subtitle="Square mono buttons with key hints" viewport="700x180"
 */
export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  /** Visual style. @default "primary" */
  variant?: 'primary' | 'outline' | 'ghost' | 'danger';
  /** Control height. @default "md" */
  size?: 'sm' | 'md' | 'lg';
  /** Bound keyboard shortcut, rendered as a boxed key on the trailing edge. */
  keyHint?: string;
  /** Material Symbols icon name, rendered leading. */
  icon?: string;
  children?: React.ReactNode;
}

export function Button(props: ButtonProps): React.JSX.Element;
