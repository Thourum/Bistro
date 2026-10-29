import type {
  CommentPosted,
  CommentsClassified,
} from '@bindings/services/models';
import { appStore } from '../../store';

export function handleCommentPosted(_data: Readonly<CommentPosted>): void {
  appStore.setState((state) => ({
    ...state,
    lastSyncAt: new Date().toISOString(),
    commentsRevision: state.commentsRevision + 1,
    threadsRevision: state.threadsRevision + 1,
  }));
}

export function handleCommentsClassified(
  _data: Readonly<CommentsClassified>,
): void {
  appStore.setState((state) => ({
    ...state,
    commentsRevision: state.commentsRevision + 1,
  }));
}
