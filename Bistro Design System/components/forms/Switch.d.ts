import * as React from 'react';

/** Square-track toggle switch for settings. */
export interface SwitchProps {
  checked?: boolean;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  label?: React.ReactNode;
  disabled?: boolean;
  id?: string;
  style?: React.CSSProperties;
}

export function Switch(props: SwitchProps): React.JSX.Element;
