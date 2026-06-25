import * as React from 'react';

/** Inline review comment / composer — avatar, mono handle, tag, body, actions. */
export interface CommentCardProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Author handle (rendered uppercase mono). */
  author: string;
  /** Right-aligned state tag, e.g. "ACTIVE_DRAFT". */
  tag?: string;
  /** Relative time. */
  time?: string;
  /** Trailing action row (buttons). */
  actions?: React.ReactNode;
  /** Body. Empty renders the "Type_comment_here…" placeholder. */
  children?: React.ReactNode;
}

export function CommentCard(props: CommentCardProps): React.JSX.Element;
