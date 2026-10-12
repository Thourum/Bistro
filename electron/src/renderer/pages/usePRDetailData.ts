import { ResultAsync } from 'neverthrow';
import { useEffect, useState } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import type { CICheck, Comment, PR } from '../types';
import { createLogger } from '../utils/logger';

const log = createLogger('PRDetailPage');

type DiffSummaryFile = { path: string; additions: number; deletions: number };
type DiffStats = { filesCount: number; additions: number; deletions: number };

interface PRDetailData {
  files: DiffSummaryFile[];
  comments: Comment[];
  ciChecks: CICheck[];
  stats: DiffStats;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

/** Loads the detail page's comments, diff summary, and CI checks independently. */
export function usePRDetailData(
  pr: PR | undefined,
  isAwaitingHydration: boolean,
  commentsRevision: number,
): PRDetailData {
  const { request } = useWebSocket();
  const [files, setFiles] = useState<DiffSummaryFile[]>([]);
  const [comments, setComments] = useState<Comment[]>([]);
  const [ciChecks, setCIChecks] = useState<CICheck[]>([]);
  const [stats, setStats] = useState<DiffStats>({
    filesCount: 0,
    additions: 0,
    deletions: 0,
  });

  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    log.debug({ commentsRevision }, 'fetching comments');
    void ResultAsync.fromPromise(
      request('REQUEST_COMMENTS', { pr_id: pr.id }),
      errorMessage,
    ).match(
      (data) => {
        const payload = data as { comments?: Comment[] };
        setComments(payload.comments ?? []);
      },
      (message) => log.warn({ err: message }, 'failed to fetch comments'),
    );
  }, [pr, request, commentsRevision, isAwaitingHydration]);

  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    void ResultAsync.fromPromise(
      request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` }),
      errorMessage,
    ).match(
      (data) => {
        const payload = data as {
          files?: DiffSummaryFile[];
          additions?: number;
          deletions?: number;
        };
        const parsed = payload.files ?? [];
        setFiles(parsed);
        setStats({
          filesCount: parsed.length,
          additions:
            payload.additions ??
            parsed.reduce((sum, file) => sum + file.additions, 0),
          deletions:
            payload.deletions ??
            parsed.reduce((sum, file) => sum + file.deletions, 0),
        });
      },
      (message) => log.warn({ err: message }, 'failed to fetch diff stats'),
    );

    void ResultAsync.fromPromise(
      request('REQUEST_CI_CHECKS', { pr_id: pr.id }),
      errorMessage,
    ).match(
      (data) => {
        const payload = data as { checks?: CICheck[] };
        setCIChecks(payload.checks ?? []);
      },
      (message) => log.warn({ err: message }, 'failed to fetch CI checks'),
    );
  }, [pr, request, isAwaitingHydration]);

  return { files, comments, ciChecks, stats };
}
