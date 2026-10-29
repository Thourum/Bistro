import { Drafts } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { appStore } from '../store';
import type { ReviewDraft } from '../types';
import { call, callWrite } from './engine/call';

export interface DraftInput {
  prId: string;
  /** Non-null edits that draft's body; position fields are then ignored. */
  id: string | null;
  path: string;
  side: ReviewDraft['side'];
  startLine: number | null;
  line: number;
  body: string;
  commitSha: string;
}

/** Loads the current user's drafts and stores them if `prId` is still shown. */
export function loadDrafts(
  prId: string,
): ResultAsync<readonly ReviewDraft[], Fault> {
  return call(() => Drafts.List({ pr_id: prId })).map((response) => {
    const drafts = response.drafts ?? [];
    appStore.setState((state) =>
      state.reviewDraftsPrId === prId
        ? { ...state, reviewDrafts: drafts }
        : state,
    );
    return drafts;
  });
}

/** Creates or edits a draft; the drafts:changed event refreshes the list. */
export function saveDraft(
  input: Readonly<DraftInput>,
): ResultAsync<ReviewDraft, Fault> {
  return callWrite(() =>
    Drafts.Save({
      pr_id: input.prId,
      id: input.id ?? '',
      path: input.path,
      side: input.side,
      start_line: input.startLine ?? 0,
      line: input.line,
      body: input.body,
      commit_sha: input.commitSha,
    }),
  ).map(({ draft }) => draft);
}

/** Deletes drafts; the drafts:changed event refreshes the list. */
export function deleteDrafts(
  prId: string,
  ids: readonly string[],
): ResultAsync<void, Fault> {
  return callWrite(() =>
    Drafts.Delete({ pr_id: prId, draft_ids: [...ids] }),
  ).map(() => undefined);
}

/** A draft written against an older head; GitHub still accepts its own commit. */
export function isDraftOutdated(draft: ReviewDraft, headSha: string): boolean {
  return headSha !== '' && draft.commit_sha !== headSha;
}
