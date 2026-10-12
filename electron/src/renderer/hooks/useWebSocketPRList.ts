import { useQuery } from '@tanstack/react-query';
import { err, Fault, ok, type Result, ServiceError } from '@thourum/fault';
import { z } from 'zod';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { PR } from '../types';
import { classifyPR } from '../utils/triage';
import { useStore } from './useStore';

type FilterState = UIState['filterState'];

interface RequestPRListData {
  filter: 'open' | 'draft' | 'all';
  limit: number;
  offset: number;
}

const PR_LIST_LIMIT = 100;
const PR_LIST_STALE_TIME_MS = 30 * 1000;
const PR_LIST_GC_TIME_MS = 5 * 60 * 1000;

const prSchema = z.object({
  id: z.string(),
  provider: z.string(),
  owner: z.string(),
  repo: z.string(),
  number: z.number(),
  title: z.string(),
  body: z.string().nullable(),
  author: z.string(),
  state: z.enum(['open', 'closed', 'merged']),
  created_at: z.string(),
  updated_at: z.string(),
  merged_at: z.string().nullable(),
  base_branch: z.string(),
  head_branch: z.string(),
  base_sha: z.string(),
  head_sha: z.string(),
  comment_count: z.number(),
  ci_status: z.string().nullable(),
  draft: z.number(),
  review_status: z.string().nullable(),
  approved_by: z.string().nullable(),
  changes_requested_by: z.string().nullable(),
  requested_reviewers: z.array(z.string()),
  touches_you: z.boolean(),
  mentioned: z.boolean(),
  viewer_commented: z.boolean(),
  unread: z.boolean(),
  reviewed_head_stale: z.boolean(),
  hydrated: z.number(),
}) satisfies z.ZodType<PR>;

const prListReadySchema = z.object({
  prs: z.array(prSchema),
  count: z.number(),
  has_more: z.boolean(),
  viewer_login: z.string().optional(),
  error: z.string().optional(),
});

function parsePRList(
  value: unknown,
): Result<z.infer<typeof prListReadySchema>, Fault> {
  const parsed = prListReadySchema.safeParse(value);
  if (!parsed.success) {
    return err(ServiceError('VALIDATION_ERROR', 'Invalid PR list response'));
  }
  if (parsed.data.error) {
    return err(ServiceError('EXTERNAL_ERROR', parsed.data.error));
  }
  return ok(parsed.data);
}

/**
 * Fetches PR list from sync engine via WebSocket.
 * 'needs_action' and 'touches_you' are client-side lanes narrowed locally.
 */
export function useWebSocketPRList(filterState: FilterState = 'open') {
  const prListRevision = useStore(appStore, (s) => s.prListRevision);
  const username = useStore(
    appStore,
    (s) => (s.connectedProviders ?? [])[0]?.username ?? '',
  );

  return useQuery({
    queryKey: ['prs-websocket', filterState, username, prListRevision],
    queryFn: async (): Promise<PR[]> => {
      const backendFilter: RequestPRListData['filter'] =
        filterState === 'needs_action' || filterState === 'touches_you'
          ? 'open'
          : filterState;
      const response = await wsClient
        .request('REQUEST_PR_LIST', {
          filter: backendFilter,
          limit: PR_LIST_LIMIT,
          offset: 0,
        } satisfies RequestPRListData)
        .mapErr((cause) =>
          new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
        )
        .andThen(parsePRList);
      if (response.isErr()) return Promise.reject(response.error);

      const payload = response.value;
      const viewerLogin = payload.viewer_login ?? '';
      if (appStore.state.viewerLogin !== viewerLogin) {
        appStore.setState((state) => ({ ...state, viewerLogin }));
      }
      if (filterState === 'needs_action') {
        return payload.prs.filter(
          (pr) => classifyPR(pr, username) === 'needs_action',
        );
      }
      if (filterState === 'touches_you') {
        return payload.prs.filter((pr) => pr.touches_you);
      }
      return payload.prs;
    },
    staleTime: PR_LIST_STALE_TIME_MS,
    gcTime: PR_LIST_GC_TIME_MS,
    retry: 1,
  });
}
