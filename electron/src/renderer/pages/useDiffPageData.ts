import { ResultAsync } from 'neverthrow';
import { useEffect, useState } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

/** Loads the full diff and inline annotations independently for the current PR. */
export function useDiffPageData({
  pr,
  isAwaitingHydration,
  commentsRevision,
}: DiffPageDataOptions): DiffPageData {
  const { request } = useWebSocket();
  const [diff, setDiff] = useState('');
  const [files, setFiles] = useState<DiffFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);

  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    setLoading(true);
    setDiffError(null);
    void ResultAsync.fromPromise(
      request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` }),
      errorMessage,
    )
      .andThen((data) => parseResponse(diffResponseSchema, data))
      .match(
        (payload) => {
          setDiff(payload.diff ?? '');
          setFiles(payload.files);
          setLoading(false);
        },
        (message) => {
          log.error({ err: message }, 'failed to fetch diff');
          setDiffError(message);
          setLoading(false);
        },
      );
  }, [isAwaitingHydration, pr, request]);

  useEffect(() => {
    if (pr === undefined) return;
    void commentsRevision;
    void ResultAsync.fromPromise(
      request('REQUEST_COMMENTS', { pr_id: pr.id }),
      errorMessage,
    )
      .andThen((data) => parseResponse(commentsResponseSchema, data))
      .match(
        (payload) => {
          setComments(payload.comments);
        },
        (message) =>
          log.warn(
            { err: message },
            'failed to fetch comments for diff annotations',
          ),
      );
  }, [pr, request, commentsRevision]);

  return { diff, files, setFiles, loading, diffError, comments };
}
