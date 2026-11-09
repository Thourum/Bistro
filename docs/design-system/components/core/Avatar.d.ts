import * as React from 'react';

/** Square initials chip. Sharp corners, mono initials. */
export interface AvatarProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** Full name / handle — initials are derived from it. */
  name?: string;
  /** Edge length in px. @default 32 */
  size?: number;
  /** @default "primary" */
  tone?: 'primary' | 'neutral' | 'inverse';
}

export function Avatar(props: AvatarProps): React.JSX.Element;
