import * as React from 'react';

export interface SystemBarLegendItem {
  key: string;
  label: string;
}

/**
 * Fixed bottom system bar — keyboard legend + status chrome.
 *
 * @startingPoint section="Layout" subtitle="The signature bottom status bar" viewport="900x40"
 */
export interface SystemBarProps extends React.HTMLAttributes<HTMLElement> {
  /** Left-aligned key/description legend. */
  legend?: SystemBarLegendItem[];
  /** Center slot content. */
  center?: React.ReactNode;
  /** Right status label. @default "System_Status: Operational" */
  status?: string;
  /** Version tag (e.g. "V.01"). */
  version?: string;
  /** Status dot color. @default "success" */
  statusTone?: 'success' | 'warning' | 'danger' | 'idle';
}

export function SystemBar(props: SystemBarProps): React.JSX.Element;
