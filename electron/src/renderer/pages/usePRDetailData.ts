import { Fault, ResultAsync } from '@thourum/fault';
import { useEffect, useState } from 'react';
import { z } from 'zod';
import { useWebSocket } from '../hooks/useWebSocket';
import {
  type CICheck,
  type Comment,
  ciCheckSchema,
  commentsResponseSchema,
  diffResponseSchema,
  type PR,
} from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('PRDetailPage');
const ciChecksResponseSchema = z.object({
  checks: z
    .array(ciCheckSchema)
    .nullish()
    .transform((checks) => checks ?? []),
});

type DiffSummaryFile = { path: string; additions: number; deletions: number };
type DiffStats = { filesCount: number; additions: number; deletions: number };

interface PRDetailData {
  files: DiffSummaryFile[];
  comments: Comment[];
  ciChecks: CICheck[];
  stats: DiffStats;
}

function requestFault(error: unknown): Fault {
  return new Fault(error instanceof Error ? error : 'Unknown error')
    .withTag('CONNECTION_ERROR')
    .withCause(error);
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
    void commentsRevision;
    if (pr === undefined || isAwaitingHydration) return;
    void ResultAsync.fromPromise(
      request('REQUEST_COMMENTS', { pr_id: pr.id }),
      requestFault,
    )
      .andThen((data) => parseResponse(commentsResponseSchema, data))
      .match(
        (payload) => {
          setComments(payload.comments);
        },
        (error) => log.warn({ err: error.message }, 'failed to fetch comments'),
      );
  }, [pr, request, commentsRevision, isAwaitingHydration]);

  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    void ResultAsync.fromPromise(
      request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` }),
      requestFault,
    )
      .andThen((data) => parseResponse(diffResponseSchema, data))
      .match(
        (payload) => {
          const parsed = payload.files;
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
        (error) =>
          log.warn({ err: error.message }, 'failed to fetch diff stats'),
      );

    void ResultAsync.fromPromise(
      request('REQUEST_CI_CHECKS', { pr_id: pr.id }),
      requestFault,
    )
      .andThen((data) => parseResponse(ciChecksResponseSchema, data))
      .match(
        (payload) => {
          setCIChecks(payload.checks);
        },
        (error) =>
          log.warn({ err: error.message }, 'failed to fetch CI checks'),
      );
  }, [pr, request, isAwaitingHydration]);

  return { files, comments, ciChecks, stats };
}
