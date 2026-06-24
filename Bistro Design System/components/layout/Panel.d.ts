import * as React from 'react';

/** Flat white container plane with a hairline border. The card of Bistro. */
export interface PanelProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Mono eyebrow header. */
  title?: string;
  /** Element pinned to the header's trailing edge. */
  action?: React.ReactNode;
  /** Pad the body. @default true */
  padded?: boolean;
  /** Add the single ambient shadow. @default false */
  elevated?: boolean;
  children?: React.ReactNode;
}

export function Panel(props: PanelProps): React.JSX.Element;
