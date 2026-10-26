import { draftIdFromKey } from '../components/diff/diffAnnotations';
import { addToast } from '../components/ToastContainer';
import { submitReview } from '../services/prActions';
import { deleteDrafts } from '../services/reviewDrafts';
import { settingsStore } from '../store/settings';
import type { KeyActionContext } from './keybindingActions';
import { getEffectiveBindings } from './keybindings';
import { findActionKeys } from './legendPresets';

function deleteActiveDraft(context: KeyActionContext): void {
  const draftId = draftIdFromKey(
    context.store.state.activeInlineCommentThreadId,
  );
  if (draftId === null || context.prId === undefined) return;
  void deleteDrafts(context.prId, [draftId]).match(
    () =>
      addToast({
        type: 'success',
        message: 'Draft deleted',
        dismissible: false,
      }),
    (fault) =>
      addToast({
        type: 'error',
        message: `Draft not deleted: ${fault.message}`,
        dismissible: true,
      }),
  );
}

function toggleActiveThreadResolve(context: KeyActionContext): void {
  const active = context.store.state.activeInlineCommentThreadId;
  if (
    context.routeBase !== 'diff' ||
    active === null ||
    draftIdFromKey(active) !== null
  ) {
    const { navigation, keybindings } = settingsStore.state;
    const bindings = getEffectiveBindings(navigation.style, keybindings);
    const keys = ['diff.nextComment', 'diff.prevComment'].flatMap(
      (actionId) => {
        const hint = findActionKeys(bindings, [actionId]);
        return hint === undefined ? [] : [hint];
      },
    );
    const hint = keys.length === 0 ? '' : ` (${keys.join(' / ')})`;
    addToast({
      type: 'warning',
      message: `Select a review thread in the diff${hint} to resolve it`,
      dismissible: true,
    });
    return;
  }
  document.dispatchEvent(new CustomEvent('bistro:diff:toggle-resolve'));
}

function submitDraftsReview(context: KeyActionContext): void {
  if (context.prId === undefined) return;
  if (context.store.state.reviewDrafts.length === 0) {
    addToast({
      type: 'warning',
      message: 'No drafts to submit',
      dismissible: true,
    });
    return;
  }
  submitReview(context.prId, 'COMMENT');
}

function discardAllDrafts(context: KeyActionContext): void {
  const ids = context.store.state.reviewDrafts.map((draft) => draft.id);
  if (context.prId === undefined || ids.length === 0) return;
  void deleteDrafts(context.prId, ids).match(
    () =>
      addToast({
        type: 'success',
        message: `Discarded ${ids.length} drafts`,
        dismissible: false,
      }),
    (fault) =>
      addToast({
        type: 'error',
        message: `Drafts not discarded: ${fault.message}`,
        dismissible: true,
      }),
  );
}

export const REVIEW_ACTIONS: Readonly<
  Record<string, (context: KeyActionContext) => void>
> = {
  'diff.deleteDraft': deleteActiveDraft,
  'review.toggleResolve': toggleActiveThreadResolve,
  'review.submit': submitDraftsReview,
  'review.discardDrafts': discardAllDrafts,
};
