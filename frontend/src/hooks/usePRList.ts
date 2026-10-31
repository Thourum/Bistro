import { type PRListResponse, PR as PRService } from '@bindings/services';
import { useQuery } from '@tanstack/react-query';
import { type Fault, okAsync, type ResultAsync } from '@thourum/fault';
import { call } from '../services/engine/call';
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

// Matches the engine's maxPRListLimit (internal/services/pr.go); the
// engine clamps larger pages back down to its default of 100.
const PR_LIST_PAGE_SIZE = 200;
const PR_LIST_STALE_TIME_MS = 30 * 1000;
const PR_LIST_GC_TIME_MS = 5 * 60 * 1000;

type PRListPage = Omit<PRListResponse, 'prs'> & { prs: PR[] };

function requestPRPage(
  backendFilter: RequestPRListData['filter'],
  offset: number,
): ResultAsync<PRListPage, Fault> {
  return call(() =>
    PRService.List({
      filter: backendFilter,
      limit: PR_LIST_PAGE_SIZE,
      offset,
    }),
  ).map((page) => ({ ...page, prs: page.prs ?? [] }));
}

/**
 * Walks engine pages until `has_more` is false so the list is never truncated.
 * A PR updated mid-walk shifts the offsets and can appear on two pages, so the
 * result keeps one entry per id. Skipped PRs heal on the next fetch,
 * since the update that moved them also bumps `prListRevision`.
 */
function requestAllPRPages(
  backendFilter: RequestPRListData['filter'],
  offset: number,
  collectedPRs: PR[],
): ResultAsync<PRListPage, Fault> {
  return requestPRPage(backendFilter, offset).andThen(
    (page): ResultAsync<PRListPage, Fault> => {
      const prs = [...collectedPRs, ...page.prs];
      if (!page.has_more)
        return okAsync({
          ...page,
          prs: [...new Map(prs.map((pr) => [pr.id, pr])).values()],
        });
      return requestAllPRPages(backendFilter, offset + PR_LIST_PAGE_SIZE, prs);
    },
  );
}

/**
 * Fetches the cached PR list through the engine bindings.
 * 'needs_action' and 'touches_you' are client-side lanes narrowed locally.
 */
export function usePRList(filterState: FilterState = 'open') {
  const prListRevision = useStore(appStore, (s) => s.prListRevision);
  const username = useStore(
    appStore,
    (s) => (s.connectedProviders ?? [])[0]?.username ?? '',
  );

  return useQuery({
    queryKey: ['prs', filterState, username, prListRevision],
    queryFn: async (): Promise<PR[]> => {
      const backendFilter: RequestPRListData['filter'] =
        filterState === 'needs_action' || filterState === 'touches_you'
          ? 'open'
          : filterState;
      const response = await requestAllPRPages(backendFilter, 0, []);
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
