import { useQuery } from '@tanstack/react-query';
import { useSQLiteQuery } from '../hooks/useSQLiteQuery';
import type { PR } from '../types';

export function usePRList(filterState: 'open' | 'draft' | 'all' = 'open') {
  const { query } = useSQLiteQuery();

  return useQuery({
    queryKey: ['prs', filterState],
    queryFn: async () => {
      let sql = 'SELECT * FROM prs WHERE 1=1';

      if (filterState === 'open') {
        sql += " AND state = 'open' AND draft = 0";
      } else if (filterState === 'draft') {
        sql += " AND draft = 1";
      }

      sql += ' ORDER BY updated_at DESC LIMIT 100';

      const result = await query<PR & Record<string, unknown>>(sql);

      if (result.isErr()) {
        throw new Error(result.error);
      }

      return result.value as PR[];
    },
    staleTime: 30 * 1000, // 30s
    gcTime: 5 * 60 * 1000, // 5m
    retry: 3,
    retryDelay: (attemptIndex: number) => Math.min(1000 * Math.pow(2, attemptIndex), 30000),
  });
}
