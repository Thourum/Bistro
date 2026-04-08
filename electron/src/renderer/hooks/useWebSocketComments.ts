import { useCallback, useState } from 'react';
import { useWebSocket } from './useWebSocket';
import type { Comment } from '../types';

interface RequestCommentsData {
  pr_id: string;
}

interface CommentsReadyData {
  pr_id: string;
  comments: Comment[];
}

interface PostCommentData {
  pr_id: string;
  body: string;
  line?: number;
}

interface CommentPostedData {
  comment_id: string;
  pr_id: string;
  author: string;
  body: string;
  line?: number;
  created_at: string;
}

/**
 * Fetch comments for a PR via WebSocket REQUEST_COMMENTS command
 */
export function useRequestComments() {
  const { request } = useWebSocket();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const requestComments = useCallback(
    async (prId: string): Promise<Comment[]> => {
      setLoading(true);
      setError(null);

      try {
        const data = await request('REQUEST_COMMENTS', { pr_id: prId } as RequestCommentsData);
        const payload = data as CommentsReadyData;
        return payload.comments;
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to fetch comments';
        setError(message);
        throw err;
      } finally {
        setLoading(false);
      }
    },
    [request]
  );

  return { requestComments, loading, error };
}

/**
 * Post a comment to a PR via WebSocket POST_COMMENT command
 */
export function usePostComment() {
  const { send } = useWebSocket();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const postComment = useCallback(
    async (prId: string, body: string, line?: number): Promise<void> => {
      setLoading(true);
      setError(null);

      try {
        send('POST_COMMENT', {
          pr_id: prId,
          body,
          line,
        } as PostCommentData);

        // Sending is fire-and-forget; listener will handle COMMENT_POSTED event
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to post comment';
        setError(message);
        throw err;
      } finally {
        setLoading(false);
      }
    },
    [send]
  );

  return { postComment, loading, error };
}
