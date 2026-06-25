import * as React from 'react';

/** Square icon-only control (Material Symbols). Toolbars, close buttons, row affordances. */
export interface IconButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  /** Material Symbols icon name. */
  icon: string;
  /** Accessible label (also the tooltip). */
  label: string;
  /** @default "md" */
  size?: 'sm' | 'md' | 'lg';
  /** @default "ghost" */
  variant?: 'ghost' | 'outline' | 'primary';
}

export function IconButton(props: IconButtonProps): React.JSX.Element;
