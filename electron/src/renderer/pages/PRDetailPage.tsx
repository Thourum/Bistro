import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { MainLayout } from '../layouts/MainLayout';
import { DiffViewer } from '../components/DiffViewer';
import { CommentInput } from '../components/CommentInput';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { usePRList } from '../queries/usePRList';
import { useWebSocket } from '../hooks/useWebSocket';
import { parseMultiFileDiff } from '../utils/diff-parser';
import { useActionDispatch } from '../hooks/useActionDispatch';
import ActionFeedback from '../components/ActionFeedback';
import StatusSidebar from '../components/StatusSidebar';
import type { Comment } from '../types';

function extractFileName(filePath: string): string {
  return filePath.replace(/^[ab]\//, '');
}

export function PRDetailPage() {
  const selectedIndex = useStore(appStore, (s) => s.selectedPRIndex);
  const filterState = useStore(appStore, (s) => s.filterState);
  const [diff, setDiff] = useState<string>('');
  const [diffLoading, setDiffLoading] = useState(false);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  const [commentsError, setCommentsError] = useState<string | null>(null);
  const [commentTarget, setCommentTarget] = useState<{ fileIndex: number; lineNumber: number } | null>(null);

  const confirmingAction = useStore(appStore, (s) => s.confirmingAction);
  const { data: prs = [], isLoading, error } = usePRList(filterState);
  const pr = prs[selectedIndex];
  const { request, send } = useWebSocket();
  const pendingAction = useStore(appStore, (s) => s.pendingAction);
  const actionResult = useStore(appStore, (s) => s.actionResult);
  const { dispatchApprove, clearActionResult } = useActionDispatch(pr?.id);

  // Dispatch approve when keyboard flow signals approve_confirmed
  useEffect(() => {
    if (confirmingAction === 'approve_confirmed') {
      dispatchApprove();
    }
  }, [confirmingAction, dispatchApprove]);

  // Fetch diff when selected PR changes
  useEffect(() => {
    if (pr === undefined) {
      return;
    }

    setDiffLoading(true);
    setDiffError(null);

    request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .then((data) => {
        const payload = data as { diff?: string };
        setDiff(payload.diff ?? '');
        setDiffLoading(false);
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? err.message : 'Unknown error';
        console.error('Failed to fetch diff:', message);
        setDiffError(message);
        setDiffLoading(false);
      });
  }, [selectedIndex, request, pr]);

  // Parse diff sections so we can resolve fileIndex → filename
  const diffSections = useMemo(() => parseMultiFileDiff(diff), [diff]);

  // Fetch comments when selected PR changes
  useEffect(() => {
    if (pr === undefined) return;

    setCommentsError(null);
    request('REQUEST_COMMENTS', { pr_id: pr.id })
      .then((data) => {
        const payload = data as { comments?: Comment[] };
        setComments(payload.comments ?? []);
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? err.message : 'Unknown error';
        console.error('Failed to fetch comments:', message);
        setCommentsError(message);
      });
  }, [selectedIndex, request, pr]);

  const handleLineClick = useCallback((fileIndex: number, lineNumber: number) => {
    setCommentTarget({ fileIndex, lineNumber });
  }, []);

  const handleCommentSubmit = useCallback(
    (body: string) => {
      if (pr === undefined || commentTarget === null) return;

      // Resolve filename from fileIndex so inline comment anchors correctly
      const section = diffSections[commentTarget.fileIndex];
      const file = section != null
        ? extractFileName(section.newFile || section.oldFile)
        : undefined;

      // Optimistic update — add immediately, server confirms via COMMENT_POSTED event
      const optimisticComment: Comment = {
        id: `optimistic-${Date.now()}`,
        pr_id: pr.id,
        author: 'you',
        body,
        file,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        line: commentTarget.lineNumber,
      };
      setComments((prev) => [...prev, optimisticComment]);
      setCommentTarget(null);

      send('POST_COMMENT', {
        pr_id: pr.id,
        body,
        file,
        line: commentTarget.lineNumber,
      });
    },
    [pr, commentTarget, send, diffSections],
  );

  const handleCommentCancel = useCallback(() => {
    setCommentTarget(null);
  }, []);

  return (
    <MainLayout
      toolbar={
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm text-on-surface-variant">
            PR #{pr?.number ?? selectedIndex + 1}
          </span>
          {pr !== undefined && (
            <span className="text-sm text-on-surface truncate max-w-xs">
              {pr.title}
            </span>
          )}
          {pr !== undefined && (
            <span className="text-xs text-on-surface-variant font-mono ml-auto">
              {pr.author} · {pr.state}
              {pr.review_status != null && ` · ${pr.review_status}`}
            </span>
          )}
        </div>
      }
    >
      <div className="flex flex-col h-full">
        <div>
          <h2 className="text-lg font-bold text-on-surface mb-2 px-4 pt-4">PR Details</h2>
          {isLoading && (
            <p className="text-sm text-on-surface-variant px-4">Loading PR details...</p>
          )}
          {error !== null && (
            <p className="text-sm text-error px-4">
              Failed to load PR: {(error as Error).message}
            </p>
          )}
          {!isLoading && error === null && pr !== undefined && (
            <p className="text-sm text-on-surface-variant font-mono px-4">
              {pr.author} · {pr.state}
            </p>
          )}
          {!isLoading && error === null && pr === undefined && (
            <p className="text-sm text-on-surface-variant px-4">
              Open PR details sidebar to view review status, comments, and metadata.
            </p>
          )}
        </div>

        <ActionFeedback
          pendingAction={pendingAction}
          actionResult={actionResult}
          onDismiss={clearActionResult}
        />

        {diffLoading && (
          <p className="text-sm text-on-surface-variant px-4 py-2">Loading diff...</p>
        )}
        {diffError !== null && (
          <p className="text-sm text-error px-4 py-2">Failed to load diff: {diffError}</p>
        )}
        {commentsError !== null && (
          <p className="text-sm text-error px-4 py-2">Failed to load comments: {commentsError}</p>
        )}

        <div className="flex flex-1 min-h-0">
          <div className="flex-1 min-h-0">
            <DiffViewer
              diff={diff}
              className="h-full"
              comments={comments}
              onLineClick={handleLineClick}
            />
          </div>
          {pr !== undefined && (
            <StatusSidebar pr={pr} ciChecks={[]} />
          )}
        </div>

        {commentTarget !== null && (
          <CommentInput
            fileIndex={commentTarget.fileIndex}
            lineNumber={commentTarget.lineNumber}
            onSubmit={handleCommentSubmit}
            onCancel={handleCommentCancel}
          />
        )}
      </div>
    </MainLayout>
  );
}
