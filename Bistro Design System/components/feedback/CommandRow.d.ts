import * as React from 'react';

/** One command-palette result — icon, title, mono subtitle, shortcut keys. */
export interface CommandRowProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  /** Material Symbols icon name. */
  icon?: string;
  title: string;
  /** Mono subtitle / description. */
  subtitle?: string;
  /** Shortcut keys, rendered as boxed caps. */
  shortcut?: string[];
  /** Keyboard-selected row. */
  selected?: boolean;
}

export function CommandRow(props: CommandRowProps): React.JSX.Element;
