import type React from 'react';
import { useEffectiveBindings } from '../../hooks/useEffectiveBindings';
import { findActionKeys } from '../../utils/legendPresets';
import { Kbd } from './kbd';

interface ActionKeyHintProps {
  readonly actionId: string;
}

/** Effective key for `actionId`, or nothing when the user unbound it. */
export function ActionKeyHint({
  actionId,
}: ActionKeyHintProps): React.ReactElement | null {
  const keys = findActionKeys(useEffectiveBindings(), [actionId]);
  return keys === undefined ? null : <Kbd>{keys}</Kbd>;
}
