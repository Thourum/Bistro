import { Fault } from '@thourum/fault';
import { useEffect, useState } from 'react';
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

  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    setLoading(true);
    setDiffError(null);
    void wsClient
      .request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .mapErr(requestFault)
      .andThen((data) => parseResponse(diffResponseSchema, data))
      .match(
        (payload) => {
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
          setDiffError(error.message);
          setLoading(false);
        },
      );
  }, [isAwaitingHydration, pr]);

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
