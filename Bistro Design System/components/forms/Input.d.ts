import * as React from 'react';

/** Square flat text field with optional leading icon and focus ring. */
export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {
  /** Material Symbols icon name, rendered leading. */
  icon?: string;
  /** Render the error ring. */
  invalid?: boolean;
}

export function Input(props: InputProps): React.JSX.Element;
