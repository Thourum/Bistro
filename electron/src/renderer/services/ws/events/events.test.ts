import { beforeEach, describe, expect, it } from 'vitest';
import { appStore } from '../../../store';
import { parseWsMessage } from '../../../types/wsSchemas';
import { dispatchWsEvent, type WsEventContext } from './index';

const initialState = appStore.state;

function createPendingResponse() {
  const responses: unknown[] = [];
  const pendingIds = new Set(['pending-1']);
  const context: WsEventContext = {
    resolveResponse: (id, data) => {
      if (id === undefined || !pendingIds.delete(id)) return false;
      responses.push(data);
      return true;
    },
  };
  return { context, responses, pendingIds };
}

function dispatch(type: string, data: unknown, context: WsEventContext): void {
  const message = parseWsMessage(
    JSON.stringify({ type, data, id: 'pending-1' }),
  )._unsafeUnwrap();
  dispatchWsEvent(message, context);
}

beforeEach(() => {
  appStore.setState(() => initialState);
});

describe('dispatchWsEvent', () => {
  it('does not resolve a response id carried by a push event', () => {
    const { context, responses, pendingIds } = createPendingResponse();
    dispatch('SYNC_COMPLETE', {}, context);
    expect(appStore.state.syncStatus).toBe('ready');
    expect(responses).toEqual([]);
    expect(pendingIds.has('pending-1')).toBe(true);
  });

  it('invalidates comments only when no pending response was handled', () => {
    const { context, responses } = createPendingResponse();
    const revision = appStore.state.commentsRevision;
    dispatch('COMMENTS_READY', { comments: [] }, context);
    expect(responses).toEqual([{ comments: [] }]);
    expect(appStore.state.commentsRevision).toBe(revision);
    dispatch('COMMENTS_READY', { comments: [] }, context);
    expect(appStore.state.commentsRevision).toBe(revision + 1);
  });

  it('maps expertise errors to the non-empty message or fallback code', () => {
    const withMessage = createPendingResponse();
    const withoutMessage = createPendingResponse();
    dispatch(
      'EXPERTISE_ERROR',
      {
        repo_id: 'repo-1',
        code: 'INDEX_ERROR',
        message: 'index failed',
      },
      withMessage.context,
    );
    dispatch(
      'EXPERTISE_ERROR',
      {
        repo_id: 'repo-1',
        code: 'INDEX_ERROR',
        message: '',
      },
      withoutMessage.context,
    );
    expect(withMessage.responses).toEqual([{ error: 'index failed' }]);
    expect(withoutMessage.responses).toEqual([{ error: 'INDEX_ERROR' }]);
    expect(appStore.state.expertiseRepoStatuses['repo-1']).toMatchObject({
      status: 'error',
      code: 'INDEX_ERROR',
      message: '',
    });
  });
});
