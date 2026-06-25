import * as React from 'react';

/** Horizontal command strip with a hairline underline. Top bars. */
export interface ToolbarProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Pinned to the leading edge (before children). */
  leading?: React.ReactNode;
  /** Pinned to the trailing edge. */
  trailing?: React.ReactNode;
  children?: React.ReactNode;
}

export function Toolbar(props: ToolbarProps): React.JSX.Element;
