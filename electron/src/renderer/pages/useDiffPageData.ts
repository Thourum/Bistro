import { Fault } from '@thourum/fault';
import { useEffect, useRef, useState } from 'react';
import { wsClient } from '../services/ws/connection';
import {
  type Comment,
  commentsResponseSchema,
  type DiffFile,
  diffResponseSchema,
  type PR,
} from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('DiffPage');

interface DiffPageData {
  diff: string;
  files: DiffFile[];
  oversizedPatchBytes: ReadonlyMap<string, number>;
  setFiles: React.Dispatch<React.SetStateAction<DiffFile[]>>;
  loading: boolean;
  diffError: string | null;
  comments: Comment[];
}

interface DiffPageDataOptions {
  pr: PR | undefined;
  isAwaitingHydration: boolean;
  commentsRevision: number;
}

function requestFault(error: unknown): Fault {
  return new Fault(error instanceof Error ? error : 'Unknown error')
    .withTag('CONNECTION_ERROR')
    .withCause(error);
}

/** Loads the full diff and inline annotations independently for the current PR. */
export function useDiffPageData({
  pr,
  isAwaitingHydration,
  commentsRevision,
}: DiffPageDataOptions): DiffPageData {
  const [diff, setDiff] = useState('');
  const [files, setFiles] = useState<DiffFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  // Viewed-only updates must not reparse patches or reset URL-driven selection.
  const [oversizedPatchBytes, setOversizedPatchBytes] = useState<
    ReadonlyMap<string, number>
  >(() => new Map());

  // The PR list re-polls every minute with fresh PR objects; key the fetch on
  // the PR revision so an unchanged PR isn't refetched (and its diff blanked).
  const prId = pr?.id;
  const repo = pr === undefined ? undefined : `${pr.owner}/${pr.repo}`;
  const baseSha = pr?.base_sha;
  const headSha = pr?.head_sha;
  const loadedPrIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (prId === undefined || repo === undefined || isAwaitingHydration) return;
    void baseSha;
    void headSha;
    // Show the loading state only for a new PR; a new head keeps the old diff
    // on screen until the refreshed one arrives.
    if (loadedPrIdRef.current !== prId) setLoading(true);
    setDiffError(null);
    void wsClient
      .request('REQUEST_DIFF', { pr_id: prId, repo })
      .mapErr(requestFault)
      .andThen((data) => parseResponse(diffResponseSchema, data))
      .match(
        (payload) => {
          loadedPrIdRef.current = prId;
          setDiff(payload.diff ?? '');
          setFiles(payload.files);
          // 'too_large' is a legacy status from before the per-PR budget removal.
          setOversizedPatchBytes(
            new Map(
              payload.files
                .filter((file) => !file.is_binary && file.diff_status !== 'ok')
                .map((file) => [file.path, file.patch_bytes]),
            ),
          );
          setLoading(false);
        },
        (error) => {
          log.error({ err: error.message }, 'failed to fetch diff');
          loadedPrIdRef.current = prId;
          setDiffError(error.message);
          setLoading(false);
        },
      );
  }, [isAwaitingHydration, prId, repo, baseSha, headSha]);

  useEffect(() => {
    if (pr === undefined) return;
    void commentsRevision;
    void wsClient
      .request('REQUEST_COMMENTS', { pr_id: pr.id })
      .mapErr(requestFault)
      .andThen((data) => parseResponse(commentsResponseSchema, data))
      .match(
        (payload) => {
          setComments(payload.comments);
        },
        (error) =>
          log.warn(
            { err: error.message },
            'failed to fetch comments for diff annotations',
          ),
      );
  }, [pr, commentsRevision]);

  return {
    diff,
    files,
    oversizedPatchBytes,
    setFiles,
    loading,
    diffError,
    comments,
  };
}
