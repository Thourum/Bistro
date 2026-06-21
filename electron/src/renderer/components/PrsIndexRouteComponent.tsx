// PrsIndexRouteComponent
//
// Rendered by the /prs/ index route. Bridges route search params ↔ UIState,
// then renders PRListPage with the route-authoritative filter/sort/q values.

import { useSearch } from '@tanstack/react-router';
import type React from 'react';
import { usePrsSearchStoreBridge } from '../hooks/usePrsSearchStoreBridge';
import { PRListPage } from '../pages/PRListPage';

export function PrsIndexRouteComponent(): React.ReactElement {
  const search = useSearch({ strict: false }) as {
    filter: 'open' | 'draft' | 'all' | 'needs_action';
    sort: 'updated' | 'created';
    q: string;
  };

  usePrsSearchStoreBridge({
    filter: search.filter,
    sort: search.sort,
    q: search.q,
  });

  return (
    <PRListPage
      routeSearch={{ filter: search.filter, sort: search.sort, q: search.q }}
    />
  );
}
