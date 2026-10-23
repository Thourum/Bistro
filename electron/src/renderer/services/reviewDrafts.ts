import { err, Fault, type Result, type ResultAsync } from '@thourum/fault';
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
const engineErrorSchema = z.object({ error: z.string() });

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

/** Every drafts reply may carry `error` instead of (or beside) its payload. */
function parseDraftsResponse<T>(
  schema: z.ZodType<T>,
  data: unknown,
): Result<T, Fault> {
  const failure = engineErrorSchema.safeParse(data);
  if (failure.success) {
    return err(new Fault(failure.data.error).withTag('EXTERNAL_ERROR'));
  }
  return parseResponse(schema, data);
}

/** Loads the current user's drafts and stores them if `prId` is still shown. */
export function loadDrafts(
  prId: string,
): ResultAsync<readonly ReviewDraft[], Fault> {
  return wsClient
    .request('DRAFTS_LIST', { pr_id: prId })
    .mapErr(networkFault)
    .andThen((data) => parseDraftsResponse(draftsReadySchema, data))
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
    .andThen((data) => parseDraftsResponse(draftSavedSchema, data))
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
    .andThen((data) => parseDraftsResponse(draftsDeletedSchema, data))
    .map(() => undefined);
}

/** A draft written against an older head; GitHub still accepts its own commit. */
export function isDraftOutdated(draft: ReviewDraft, headSha: string): boolean {
  return headSha !== '' && draft.commit_sha !== headSha;
}
