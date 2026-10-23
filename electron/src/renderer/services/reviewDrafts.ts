import { Fault, type ResultAsync } from '@thourum/fault';
import { z } from 'zod';
import { appStore } from '../store';
import { type ReviewDraft, reviewDraftSchema } from '../types';
import { parseResponse } from '../utils/error';
import { wsClient } from './ws/connection';

const draftsReadySchema = z.object({
  pr_id: z.string(),
  drafts: z
    .array(reviewDraftSchema)
    .nullish()
    .transform((drafts) => drafts ?? []),
});
const draftSavedSchema = z.object({ draft: reviewDraftSchema });
const draftsDeletedSchema = z.object({ deleted: z.number() });

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

function networkFault(cause: Fault): Fault {
  return new Fault(cause).withTag('NETWORK_ERROR').withCause(cause);
}

/** Loads the current user's drafts and stores them if `prId` is still shown. */
export function loadDrafts(
  prId: string,
): ResultAsync<readonly ReviewDraft[], Fault> {
  return wsClient
    .request('DRAFTS_LIST', { pr_id: prId })
    .mapErr(networkFault)
    .andThen((data) => parseResponse(draftsReadySchema, data))
    .map(({ drafts }) => {
      appStore.setState((state) =>
        state.reviewDraftsPrId === prId
          ? { ...state, reviewDrafts: drafts }
          : state,
      );
      return drafts;
    });
}

/** Creates or edits a draft; the DRAFTS_CHANGED push refreshes the list. */
export function saveDraft(
  input: Readonly<DraftInput>,
): ResultAsync<ReviewDraft, Fault> {
  return wsClient
    .request('DRAFT_SAVE', {
      pr_id: input.prId,
      id: input.id ?? '',
      path: input.path,
      side: input.side,
      start_line: input.startLine ?? 0,
      line: input.line,
      body: input.body,
      commit_sha: input.commitSha,
    })
    .mapErr(networkFault)
    .andThen((data) => parseResponse(draftSavedSchema, data))
    .map(({ draft }) => draft);
}

/** Deletes drafts; the DRAFTS_CHANGED push refreshes the list. */
export function deleteDrafts(
  prId: string,
  ids: readonly string[],
): ResultAsync<void, Fault> {
  return wsClient
    .request('DRAFT_DELETE', { pr_id: prId, draft_ids: ids })
    .mapErr(networkFault)
    .andThen((data) => parseResponse(draftsDeletedSchema, data))
    .map(() => undefined);
}

/** A draft written against an older head; GitHub still accepts its own commit. */
export function isDraftOutdated(draft: ReviewDraft, headSha: string): boolean {
  return headSha !== '' && draft.commit_sha !== headSha;
}
