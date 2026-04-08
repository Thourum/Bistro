import { useQuery } from '@tanstack/react-query';
import { useSQLiteQuery } from '../hooks/useSQLiteQuery';
import type { PR } from '../types';

export function usePRList(providerId: string, filterState: 'open' | 'draft' | 'all' = 'open') {
  const { query } = useSQLiteQuery();

  return useQuery({
    queryKey: ['prs', providerId, filterState],
    queryFn: async () => {
      let sql = 'SELECT * FROM pull_requests WHERE provider_id = ?';
      const params: unknown[] = [providerId];

      if (filterState === 'open') {
        sql += " AND state IN ('open')";
      } else if (filterState === 'draft') {
        sql += " AND state IN ('draft')";
      }

      sql += ' ORDER BY updated_at DESC LIMIT 100';

      const result = await query<PR & Record<string, unknown>>(sql, params);

      if (result.isErr()) {
        throw new Error(result.error);
      }

      return result.value as PR[];
    },
    staleTime: 30 * 1000, // 30s
    gcTime: 5 * 60 * 1000, // 5m
  });
}
