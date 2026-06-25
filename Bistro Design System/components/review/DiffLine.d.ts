import * as React from 'react';

/** One unified-diff row — number gutters, +/- marker, mono code. */
export interface DiffLineProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Old-side line number (omit on additions). */
  oldNo?: number | string;
  /** New-side line number (omit on deletions). */
  newNo?: number | string;
  /** @default "context" */
  type?: 'add' | 'remove' | 'context';
  /** Active line — primary outline. */
  focused?: boolean;
  children?: React.ReactNode;
}

export function DiffLine(props: DiffLineProps): React.JSX.Element;
