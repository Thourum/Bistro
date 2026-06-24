import * as React from 'react';

/** Small bordered mono label — "GROUP", "RUST", status pills. */
export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** @default "outline" */
  variant?: 'outline' | 'solid';
  /** Semantic tint. @default "neutral" */
  tone?: 'neutral' | 'primary' | 'success' | 'danger' | 'warning';
  children?: React.ReactNode;
}

export function Badge(props: BadgeProps): React.JSX.Element;
