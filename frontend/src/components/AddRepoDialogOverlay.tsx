import type React from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { AddRepoDialog } from './AddRepoDialog';

export function AddRepoDialogOverlay(): React.ReactElement | null {
  const { activeOverlay, closeOverlay } = useOverlays();

  if (activeOverlay !== 'addRepo') {
    return null;
  }

  return <AddRepoDialog onClose={closeOverlay} />;
}
