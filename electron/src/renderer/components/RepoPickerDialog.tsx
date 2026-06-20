// Slice 6+7 — RepoPickerDialog
//
// Repo-picker as a Radix Dialog, open = activeOverlay==='repoPicker'.
// Preserves existing visuals/positioning; Radix manages focus-trap, Escape,
// and focus-return automatically.
//
// NOTE: Do NOT use components/ui/dialog.tsx — it adds centered/black defaults
// that conflict with this overlay's positioning.

import * as Dialog from '@radix-ui/react-dialog';
import type React from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { RepoPicker } from './RepoPicker';

export function RepoPickerDialog(): React.ReactElement {
  const { activeOverlay, closeOverlay } = useOverlays();
  const isOpen = activeOverlay === 'repoPicker';

  return (
    <Dialog.Root
      open={isOpen}
      onOpenChange={(open) => {
        if (!open) closeOverlay();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-transparent backdrop-blur-[20px]" />
        <Dialog.Content
          className="fixed inset-0 z-40 bg-transparent flex items-start justify-center pt-[153px]"
          aria-label="Repository picker"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeOverlay();
          }}
        >
          <div className="w-full max-w-2xl mx-4 bg-surface-container-lowest border border-primary shadow-[0_10px_30px_rgba(25,28,29,0.04)] overflow-hidden p-4">
            <RepoPicker onClose={closeOverlay} />
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
