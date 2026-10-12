import { appStore } from '../../../store';
import type { WSInboundMessage } from '../../../types/wsSchemas';
import type { WsEventContext } from './context';

export function handleCommentPosted(): void {
  appStore.setState((state) => ({
    ...state,
    lastSyncAt: new Date().toISOString(),
    commentsRevision: state.commentsRevision + 1,
    threadsRevision: state.threadsRevision + 1,
  }));
}

export function handleCommentsReady(
  message: Readonly<WSInboundMessage<'COMMENTS_READY'>>,
  context: WsEventContext,
): void {
  if (context.resolveResponse(message.id, message.data)) return;
  // Async classification pushes have no pending request; open views must refetch.
  appStore.setState((state) => ({
    ...state,
    commentsRevision: state.commentsRevision + 1,
  }));
}
