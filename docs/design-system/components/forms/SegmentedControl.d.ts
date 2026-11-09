import * as React from 'react';

export interface SegmentOption {
  value: string;
  label: string;
  count?: number;
}

/** Boxed filter row — the inbox "Open / Draft / All / Needs Action" control. */
export interface SegmentedControlProps {
  /** Options as plain strings or `{ value, label, count }`. */
  options: (string | SegmentOption)[];
  value: string;
  onChange?: (value: string) => void;
  style?: React.CSSProperties;
}

export function SegmentedControl(
  props: SegmentedControlProps,
): React.JSX.Element;
