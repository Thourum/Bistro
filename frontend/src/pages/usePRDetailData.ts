import { Comments, Diff } from '@bindings/services';
import { useEffect, useState } from 'react';
import { call } from '../services/engine/call';
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

/** Loads the detail page's comments and diff summary independently. */
export function usePRDetailData(
  pr: PR | undefined,
  isAwaitingHydration: boolean,
  commentsRevision: number,
): PRDetailData {
  const [files, setFiles] = useState<DiffSummaryFile[]>([]);
  const [comments, setComments] = useState<Comment[]>([]);

  const [stats, setStats] = useState<DiffStats>({
    filesCount: 0,
    additions: 0,
    deletions: 0,
  });

  useEffect(() => {
    void commentsRevision;
    if (pr === undefined || isAwaitingHydration) return;
    void call(() => Comments.List({ pr_id: pr.id })).match(
      (payload) => {
        setComments(payload.comments ?? []);
      },
      (error) => log.warn({ err: error.message }, 'failed to fetch comments'),
    );
  }, [pr, commentsRevision, isAwaitingHydration]);

  useEffect(() => {
    if (pr === undefined || isAwaitingHydration) return;
    void call(() => Diff.Get({ pr_id: pr.id })).match(
      (payload) => {
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
      (error) => log.warn({ err: error.message }, 'failed to fetch diff stats'),
    );
  }, [pr, isAwaitingHydration]);

  return { files, comments, ciChecks: [], stats };
}
