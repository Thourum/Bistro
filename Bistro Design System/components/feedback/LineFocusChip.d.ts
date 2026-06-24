import * as React from 'react';

/** Dark inverse position/tooltip chip — "LINE_FOCUS: 44" over a path. */
export interface LineFocusChipProps
  extends React.HTMLAttributes<HTMLSpanElement> {
  /** Primary label (dark cap). */
  label: string;
  /** Optional secondary line (lighter, below). */
  sub?: string;
}

export function LineFocusChip(props: LineFocusChipProps): React.JSX.Element;
