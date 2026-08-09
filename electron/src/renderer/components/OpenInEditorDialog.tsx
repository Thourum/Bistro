import type { EditorCheckoutDialog } from '../hooks/useOpenInEditor';
import { Button } from './ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/dialog';

interface OpenInEditorDialogProps {
  dialog: EditorCheckoutDialog | null;
  onCheckoutAndOpen: () => void;
  onOpenCurrentVersion: () => void;
  onCancel: () => void;
}

/**
 * Shown when the local checkout's HEAD doesn't match the PR's head SHA.
 * Spec: docs/superpowers/specs/2026-07-18-phase5-inbox-triage-polish-design.md §5.3.
 */
export function OpenInEditorDialog({
  dialog,
  onCheckoutAndOpen,
  onOpenCurrentVersion,
  onCancel,
}: OpenInEditorDialogProps) {
  return (
    <Dialog open={dialog !== null} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>PR revision not checked out</DialogTitle>
        </DialogHeader>
        <DialogDescription>
          Your local checkout isn't at this PR's head revision. Checking out
          moves your working copy to the PR head, detached from whatever branch
          you're currently on.
        </DialogDescription>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="outline" onClick={onOpenCurrentVersion}>
            Open current version
          </Button>
          <Button onClick={onCheckoutAndOpen}>Check out PR head</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
